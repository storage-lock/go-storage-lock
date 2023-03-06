package storage_lock

import (
	"context"
	"errors"
	variable_parameter "github.com/golang-infrastructure/go-variable-parameter"
	"github.com/google/uuid"
	"strings"
	"time"
)

// StorageLock 基于存储介质的锁
type StorageLock struct {

	// 锁持久化存储到哪个介质上
	storage Storage
	// 锁的一些选项
	options *StorageLockOptions

	// 负责为锁租约续期的协程
	storageLockWatchDog *LeaseRefreshGoroutine

	// 做一些ID自动生成的工作
	ownerIdGenerator *OwnerIdGenerator
}

// NewStorageLock 创建一个基于存储介质的锁
// storage: 锁持久化存储介质
// options: 创建和维护锁时的相关配置项
func NewStorageLock(storage Storage, options ...*StorageLockOptions) *StorageLock {

	// 设置默认选项
	option := variable_parameter.TakeFirstParamOrDefaultFunc[*StorageLockOptions](options, func() *StorageLockOptions {
		return NewStorageLockOptions()
	})

	// 如果没有设置锁的ID的话，则为其生成一个随机的默认的ID
	if option.LockId == "" {
		option.LockId = strings.ReplaceAll(uuid.New().String(), "-", "")
	}

	lock := &StorageLock{
		storage:          storage,
		options:          option,
		ownerIdGenerator: NewOwnerIdGenerator(),
	}

	// 仅当获取到锁的时候才启动这个协程，否则的话可能会有协程残留
	//lock.storageLockWatchDog = NewStorageLockWatchDog(lock)

	return lock
}

// Lock 尝试获取锁
func (x *StorageLock) Lock(ctx context.Context, ownerId ...string) error {
	return x.LockWithRetry(ctx, x.options.VersionMissRetryTimes, ownerId...)
}

// LockWithRetry 带重试次数的获取锁，因为乐观锁的失败率可能会比较高
func (x *StorageLock) LockWithRetry(ctx context.Context, leftTryTimes uint, ownerId ...string) error {

	// 如果没有指定ownerId，则为其生成一个默认的ownerId
	if len(ownerId) == 0 {
		ownerId = append(ownerId, x.ownerIdGenerator.getDefaultOwnId())
	} else if len(ownerId) >= 2 {
		return ErrOwnerCanOnlyOne
	}

	// 先尝试读取锁的信息
	lockInformation, err := x.getLockInformation(ctx)
	// 如果读取锁的时候发生错误，除非是锁不存在的错误，否则都认为是中断执行
	if err != nil && !errors.Is(err, ErrLockNotFound) {
		return err
	}

	// 计算从当前时间开始计算的租约的过期时间
	expireTime, err := x.getLeaseExpireTime(ctx)
	if err != nil {
		return err
	}

	// 如果锁的信息存在，则说明之前锁就已经存在了
	if lockInformation != nil {

		// 给定的锁已经存在了，又分为两种情况，一种是锁就是自己持有的，一种是锁被别人持有
		oldVersion := lockInformation.Version

		if lockInformation.OwnerId == ownerId[0] {

			// 这个锁当前就是自己持有的，那进行了一次更改，版本增加
			lockInformation.Version++
			// 锁的深度加1
			lockInformation.LockCount++
			// 同时租约过期时间也顺带跟着更新一下
			lockInformation.LeaseExpireTime = expireTime

			// 然后尝试把新的锁的信息更新回存储介质中
			err = x.storage.UpdateWithVersion(ctx, x.options.LockId, oldVersion, lockInformation.Version, lockInformation.ToJsonString())
			// 更新成功，则本次释放锁成功
			if err == nil {
				return nil
			}
			// 如果发生了错误，除非是版本未命中的错误，否则都不再重试了，直接认为是中断式的错误
			if !errors.Is(err, ErrVersionMiss) {
				return err
			}
			// 执行到这里，要么是没更新成功，是还有重试次数的话则重试
			if leftTryTimes > 0 {
				return x.LockWithRetry(ctx, leftTryTimes-1, ownerId...)
			} else {
				return err
			}
		} else {
			storageTime, err := x.storage.GetTime(ctx)
			if err != nil {
				return err
			}
			// 如果持有锁的不是自己，则看下是否过期了
			if lockInformation.LeaseExpireTime.After(storageTime) {
				// 锁被别人持有者，并且也没有过期，则只好放弃
				return ErrLockFailed
			}
			// 别人持有的锁过期了，啊哈哈，那我给它删掉清理一下吧
			// 这个返回的错误会被忽略，删除直接重试
			// TODO 2023-1-27 18:53:41 思考这样搞会不会有什么问题
			_ = x.storage.DeleteWithVersion(ctx, x.options.LockId, oldVersion)
			return x.LockWithRetry(ctx, leftTryTimes-1, ownerId...)
		}
	}

	// 锁还不存在，那尝试持有它
	lockInformation = &LockInformation{
		OwnerId:         ownerId[0],
		LockBeginTime:   time.Now(),
		Version:         1,
		LockCount:       1,
		LeaseExpireTime: expireTime,
	}
	err = x.storage.InsertWithVersion(ctx, x.options.LockId, lockInformation.Version, lockInformation.ToJsonString())
	if err != nil {
		if leftTryTimes > 0 {
			return x.LockWithRetry(ctx, leftTryTimes-1, ownerId...)
		} else {
			return ErrLockFailed
		}
	}

	// 插入成功，看下如果之前有的话停掉
	if x.storageLockWatchDog != nil {
		x.storageLockWatchDog.Stop()
	}

	// 启动一个新的租约续期协程
	x.storageLockWatchDog = NewStorageLockWatchDog(x, ownerId[0])
	x.storageLockWatchDog.Start()
	return nil
}

// 获取租约下一次的过期时间
func (x *StorageLock) getLeaseExpireTime(ctx context.Context) (time.Time, error) {
	var zero time.Time
	storageTime, err := x.storage.GetTime(ctx)
	if err != nil {
		return zero, err
	}
	return storageTime.Add(x.options.LeaseExpireAfter), nil
}

// UnLock 尝试释放锁，如果释放不成功的话则会返回error
func (x *StorageLock) UnLock(ctx context.Context, ownerId ...string) error {
	return x.UnLockWithRetry(ctx, x.options.VersionMissRetryTimes, ownerId...)
}

// UnLockWithRetry 手动指定重试次数的释放锁，如果锁竞争较大的话应该适当提高乐观锁的失败重试次数
func (x *StorageLock) UnLockWithRetry(ctx context.Context, leftTryTimes uint, ownerId ...string) error {

	// 如果没有指定ownerId的话，则为其生成一个默认的ownerId
	if len(ownerId) == 0 {
		ownerId = append(ownerId, x.ownerIdGenerator.getDefaultOwnId())
	} else if len(ownerId) >= 2 {
		return ErrOwnerCanOnlyOne
	}

	// 尝试读取锁的信息
	lockInformation, err := x.getLockInformation(ctx)

	// 如果锁的信息都读取失败了，则没必要继续下去，这里没必要区分是锁不存在的错误还是其它错误，反正只要是错误就直接中断返回
	if err != nil {
		return err
	}

	// 如果读取到的锁的信息为空，则说明锁不存在
	if lockInformation == nil {
		return ErrLockNotFound
	}

	// 如果锁的当前持有者的ID不是自己，则无权释放锁
	if lockInformation.OwnerId != ownerId[0] {
		return ErrLockNotBelongYou
	}

	// 锁是自己持有的，则尝试释放锁
	expireTime, err := x.getLeaseExpireTime(ctx)
	if err != nil {
		return err
	}
	lastVersion := lockInformation.Version
	lockInformation.Version++
	lockInformation.LockCount--
	lockInformation.LeaseExpireTime = expireTime
	// 如果释放一次之后发现还没有释放干净，说明是重入锁，并且加锁次数还没有为0，则尝试更新锁的信息
	if lockInformation.LockCount > 0 {
		err := x.storage.UpdateWithVersion(ctx, x.options.LockId, lastVersion, lockInformation.Version, lockInformation.ToJsonString())
		// 更新成功，直接返回，说明锁释放成功了
		if err == nil {
			return nil
		}
		// 如果是发生了错误，只要不是版本未命中的错误则都不再重试
		// 这里仅认为版本未命中的错误才是可以恢复的错误，其他类型的错误都是不可以恢复的错误，就不再重试了
		if err != nil && !errors.Is(err, ErrVersionMiss) {
			return err
		}
		// 更新未成功，看下是否还有重试次数
		if leftTryTimes > 0 {
			// 我还有重试次数，我要尝试重试
			return x.UnLockWithRetry(ctx, leftTryTimes-1, ownerId...)
		} else {
			// 更新失败，并且也没有重试次数了，则只好返回错误
			return ErrUnlockFailed
		}
	} else {
		// 重入锁的次数已经被释放干净了，现在需要将其彻底删除
		err := x.storage.DeleteWithVersion(ctx, x.options.LockId, lastVersion)
		// 如果删除的时候遇到错误，则直接认为锁释放失败
		if err != nil {
			if errors.Is(err, ErrVersionMiss) {
				// 还有重试次数，则再次尝试删除锁
				if leftTryTimes > 0 {
					return x.UnLockWithRetry(ctx, leftTryTimes-1, ownerId...)
				} else {
					// 没有重试次数了，则只好返回错误
					return ErrLockFailed
				}
			} else {
				return err
			}
		}

		// 执行到这里表示已经删除成功了，则将租约续期的协程停掉
		x.storageLockWatchDog.Stop()
		return nil
	}
}

// UnLockUntilRelease 一直unlock直到释放掉锁，可能的场景是可重入锁重启之后清除之前可能存在的锁状态
func (x *StorageLock) UnLockUntilRelease(ctx context.Context, ownerId ...string) error {
	// TODO 递归可能会有溢出的风险，修改为迭代实现
	err := x.UnLock(ctx, ownerId...)
	if err != nil {
		if errors.Is(err, ErrLockNotFound) {
			return nil
		} else {
			return err
		}
	} else {
		return x.UnLockUntilRelease(ctx, ownerId...)
	}
}

// 获取之前的锁保存的信息
func (x *StorageLock) getLockInformation(ctx context.Context) (*LockInformation, error) {
	lockInformationJsonString, err := x.storage.Get(ctx, x.options.LockId)
	if err != nil {
		return nil, err
	}
	if lockInformationJsonString == "" {
		return nil, ErrLockNotFound
	}
	return FromJsonString(lockInformationJsonString)
}

// ------------------------------------------------- --------------------------------------------------------------------
