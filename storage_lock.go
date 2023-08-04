package storage_lock

import (
	"context"
	"errors"
	variable_parameter "github.com/golang-infrastructure/go-variable-parameter"
	"github.com/storage-lock/go-events"
	"github.com/storage-lock/go-storage"
	"github.com/storage-lock/go-utils"
	"math/rand"
	"strings"
	"time"
)

// StorageLock 基于存储介质的锁模型实现，底层存储介质是可插拔的
type StorageLock struct {

	// 锁持久化存储到哪个存储介质上
	storage storage.Storage
	// 锁的一些选项，可以高度定制化锁的行为
	options *StorageLockOptions

	// 负责为锁租约续期的协程，每个锁在被持有期间都会存在一个续租协程
	storageLockWatchDog *LeaseRefreshGoroutine

	// 做一些ID自动生成的工作
	ownerIdGenerator *OwnerIdGenerator
}

const LockIdPrefix = "storage-lock-id-"

// NewStorageLock 创建一个基于存储介质的锁
// storage: 锁持久化的存储介质，不同的介质有不同的实现，比如基于Mysql、基于MongoDB
// options: 创建和维护锁时的相关配置项
func NewStorageLock(storage storage.Storage, options ...*StorageLockOptions) *StorageLock {

	// 如果没有设置锁的参数的话则使用默认选项
	option := variable_parameter.TakeFirstParamOrDefaultFunc[*StorageLockOptions](options, func() *StorageLockOptions {
		return NewStorageLockOptions()
	})

	// 触发创建锁的事件
	e := events.NewEvent(option.LockId).SetType(events.EventTypeCreateLock).SetStorageName(storage.GetName()).SetListeners(option.EventListeners)

	// 如果没有设置锁的ID的话，则为其生成一个随机的默认的ID，但是通常情况下不是最佳实践，应该避免这种用法
	if option.LockId == "" {
		option.LockId = utils.RandomID(LockIdPrefix)
		e.SetLockId(option.LockId).AddActionByName("random-lock-id")
	}

	lock := &StorageLock{
		storage:          storage,
		options:          option,
		ownerIdGenerator: NewOwnerIdGenerator(),
	}

	// 仅当锁被持有的时候才启动这个协程，否则的话可能会有协程残留
	//lock.storageLockWatchDog = NewStorageLockWatchDog(lock)

	e.Publish(context.Background())

	return lock
}

// Lock 尝试获取锁
// ctx: 用来控制超时，如果想永远不超时则传入context.Background，此时不获取到锁永不罢休，返回也永远为nil
// ownerId: 是谁在尝试获取锁，如果不指定的话会为当前协程生成一个默认的ownerId
func (x *StorageLock) Lock(ctx context.Context, ownerId ...string) error {

	// 触发一个获取锁的事件
	e := events.NewEvent(x.options.LockId).SetType(events.EventTypeLock).SetListeners(x.options.EventListeners).SetStorageName(x.storage.GetName())
	if len(ownerId) > 0 {
		e.SetOwnerId(ownerId[0])
	}

	for {

		err := x.lockWithRetry(ctx, e.Fork(), ownerId...)
		if err == nil {
			e.AddActionByName("success").Publish(ctx)
			return nil
		}

		// TODO 2023-6-21 00:33:21 错误处理
		e.Fork().AddActionByName("error").SetErr(err).Publish(ctx)

		select {
		case <-ctx.Done():
			e.AddActionByName("timeout").Publish(ctx)
			return err
		case <-time.After(time.Microsecond):
			e.Fork().AddActionByName("sleep-retry").Publish(ctx)
			time.Sleep(time.Second * time.Duration(rand.Intn(5)+1))
			continue
		}
	}
}

// lockWithRetry 带重试次数的获取锁，因为乐观锁的失败率可能会比较高
func (x *StorageLock) lockWithRetry(ctx context.Context, e *events.Event, ownerId ...string) error {

	e.AddActionByName("lock-with-retry")

	// 如果没有指定ownerId，则为其生成一个默认的ownerId
	if len(ownerId) == 0 {
		ownerId = append(ownerId, x.ownerIdGenerator.getDefaultOwnId())
		e.AddActionByName("use-default-owner")
	} else if len(ownerId) >= 2 {
		e.AddActionByName("owner-error").Publish(ctx)
		return ErrOwnerCanOnlyOne
	}

	// 先尝试从Storage中读取上次存储的锁的信息
	lockInformation, err := x.getLockInformation(ctx, e)
	// 如果读取锁的时候发生错误，除非是锁不存在的错误，否则都认为是中断执行
	if err != nil && !errors.Is(err, ErrLockNotFound) {
		e.AddActionByName("get-lock-information-error").SetErr(err).Publish(ctx)
		return err
	}

	// 如果锁的信息存在，则说明之前锁就已经存在了
	if lockInformation != nil {
		e.AddActionByName("to-lock-exists").Publish(ctx)
		return x.lockExists(ctx, e.Fork(), ownerId[0], lockInformation)
	} else {
		// 否则认为之前锁是不存在的
		e.AddActionByName("to-lock-not-exists").Publish(ctx)
		return x.lockNotExists(ctx, e.Fork(), ownerId[0], lockInformation)
	}
}

// 尝试获取已经存在的锁
func (x *StorageLock) lockExists(ctx context.Context, e *events.Event, ownerId string, lockInformation *storage.LockInformation) error {

	e.AddActionByName("lock-exists")

	// 给定的锁已经存在了，又分为两种情况，一种是锁就是自己持有的，一种是锁被别人持有
	if lockInformation.OwnerId == ownerId {
		e.AddActionByName("i-am-owner").Publish(ctx)
		return x.reentryLock(ctx, e.Fork(), ownerId, lockInformation)
	} else {
		e.AddActionByName("i-am-not-owner").Publish(ctx)
		return x.cleanExpiredLockAndRetry(ctx, e.Fork(), ownerId, lockInformation)
	}
}

// 进入重入锁的逻辑，尝试对可重入锁的层级加一
func (x *StorageLock) reentryLock(ctx context.Context, e *events.Event, ownerId string, lockInformation *storage.LockInformation) error {

	e.AddActionByName("reentry").SetLockInformation(lockInformation)

	// 计算从当前时间开始计算的租约的过期时间
	expireTime, err := x.getLeaseExpireTime(ctx, e)
	if err != nil {
		e.AddActionByName("get-lease-expire-time-error").SetErr(err).Publish(ctx)
		return err
	}

	oldVersion := lockInformation.Version

	// 这个锁当前就是自己持有的，那进行了一次更改，版本增加
	lockInformation.Version++
	// 锁的深度加1
	lockInformation.LockCount++
	// 同时租约过期时间也顺带跟着更新一下
	lockInformation.LeaseExpireTime = expireTime

	// 然后尝试把新的锁的信息更新回存储介质中
	err = x.storage.UpdateWithVersion(ctx, x.options.LockId, oldVersion, lockInformation.Version, lockInformation)
	// 更新成功，则本次获取锁成功，可重入锁的层级又深了一层
	if err == nil {
		e.AddActionByName("update-with-version-success").Publish(ctx)
		return nil
	}
	// 如果发生了错误，除非是版本未命中的错误，否则都不再重试了，直接认为是中断式的错误
	if !errors.Is(err, ErrVersionMiss) {
		e.AddActionByName("update-with-version-error").SetErr(err).Publish(ctx)
		return err
	}
	e.AddActionByName("update-with-version-miss")

	// 执行到这里，如果没更新成功，并且还有重试次数的话则重试
	select {
	case <-time.After(time.Microsecond):
		// 还有时间，再重试一次
		e.AddActionByName("to-lock-with-retry").Publish(ctx)
		return x.lockWithRetry(ctx, e.Fork(), ownerId)
	case <-ctx.Done():
		// 超时不再重试
		e.AddActionByName("timeout").Publish(ctx)
		return err
	}
}

// 获取锁的时候发现锁是被别人持有者的，但是不确定是不是有效的持有，于是就先尝试清除无效的持有，然后再重新竞争锁
func (x *StorageLock) cleanExpiredLockAndRetry(ctx context.Context, e *events.Event, ownerId string, lockInformation *storage.LockInformation) error {

	e.AddActionByName("clean-expired-lock-and-retry").SetLockInformation(lockInformation)

	// 另一种锁已经存在的情况是锁被别人持有者，这个时候又分为两种情况
	// 一种是持有锁的人在租约期内，那这个时候只能老老实实的等待
	// 还有一种情况是上次持有锁的人可能没有能正常退出释放锁，锁被残留在了数据库中，这个时候锁虽然存在，但是已经过了租约的有效期，
	// 因此这种情况是可以尝试清除掉无效的锁，然后大家再重新竞争锁的
	storageTime, err := x.storage.GetTime(ctx)
	if err != nil {
		e.AddActionByName("get-storage-time-error").SetErr(err).Publish(ctx)
		return err
	}

	// 看下是否过期了
	if lockInformation.LeaseExpireTime.After(storageTime) {
		// 锁被别人持有者，并且也没有过期，则只好放弃
		e.AddActionByName("lock-by-other-and-not-expired").Publish(ctx)
		return ErrLockFailed
	}
	e.AddActionByName("other-lock-expired")

	// 别人持有的锁过期了，啊哈哈，那我给它删掉清理一下吧
	// 这个返回的错误会被忽略，删除直接重试，这里的删除可能会失败，比如当出现并发情况时只有一个人能删除成功其它人都会失败
	// TODO 2023-1-27 18:53:41 思考这样搞会不会有什么问题
	err = x.storage.DeleteWithVersion(ctx, x.options.LockId, lockInformation.Version, lockInformation)
	if err != nil {
		e.Fork().SetLockInformation(lockInformation).AddActionByName("delete-with-version-error").SetErr(err).Publish(ctx)
	} else {
		e.Fork().SetLockInformation(lockInformation).AddActionByName("delete-with-version-success").Publish(ctx)
	}

	// 然后再尝试重新竞争锁，回归到一个普通的不存在的锁的竞争流程
	// 当然在分布式高竞争的情况下更有可能清除过期的锁是为他人做嫁衣，A刚删除锁还没来得及重新竞争就被B获取到了，A白忙活一场哈哈哈
	e.AddActionByName("to-lock-with-retry").Publish(ctx)
	return x.lockWithRetry(ctx, e.Fork(), ownerId)
}

// 尝试获取不存在的锁，这个是最爽的分支，能够直接获取到锁
func (x *StorageLock) lockNotExists(ctx context.Context, e *events.Event, ownerId string, lockInformation *storage.LockInformation) error {

	e.AddActionByName("lock-not-exists").SetLockInformation(lockInformation)

	// 获取Storage的时间
	storageTime, err := x.storage.GetTime(ctx)
	if err != nil {
		e.AddActionByName("get-storage-time-error").SetErr(err).Publish(ctx)
		return err
	}

	// 计算从Storage的当前时间开始计算的租约的过期时间
	expireTime := storageTime.Add(x.options.LeaseExpireAfter)

	// 锁还不存在，那尝试持有它
	lockInformation = &storage.LockInformation{
		LockId:  x.options.LockId,
		OwnerId: ownerId,
		// 锁的开始时间跟Storage的时间保持一致，这样才好有对比
		LockBeginTime:   storageTime,
		Version:         1,
		LockCount:       1,
		LeaseExpireTime: expireTime,
	}
	e.SetLockInformation(lockInformation)

	err = x.storage.InsertWithVersion(ctx, x.options.LockId, lockInformation.Version, lockInformation)
	if err != nil {

		e.Fork().SetLockInformation(lockInformation).AddActionByName("insert-with-version-error").SetErr(err).Publish(ctx)

		select {
		case <-ctx.Done():
			e.AddActionByName("timeout").Publish(ctx)
			return ErrLockFailed
		case <-time.After(time.Microsecond):
			e.AddActionByName("retry").Publish(ctx)
			return x.lockWithRetry(ctx, e.Fork(), ownerId)
		}

	}

	// 插入成功，看下如果之前有续租协程的话就停掉
	if x.storageLockWatchDog != nil {
		e.Fork().SetLockInformation(lockInformation).AddActionByName("stop-watch-dog").SetWatchDogId(x.storageLockWatchDog.GetID()).Publish(ctx)
		x.storageLockWatchDog.Stop()
	}

	// 启动一个新的租约续期协程
	x.storageLockWatchDog = NewStorageLockWatchDog(e, x, ownerId)
	x.storageLockWatchDog.Start()
	e.Fork().SetLockInformation(lockInformation).AddActionByName("start-watch-dog").SetWatchDogId(x.storageLockWatchDog.GetID()).Publish(ctx)

	e.Publish(ctx)
	return nil
}

// 获取租约下一次的过期时间
func (x *StorageLock) getLeaseExpireTime(ctx context.Context, e *events.Event) (time.Time, error) {

	getTimeAction := events.NewAction(ActionStorageGetTime)
	storageTime, err := x.storage.GetTime(ctx)
	e.Fork().AddAction(getTimeAction.End().SetErr(err).SetPayload(storageTime.String())).Publish(ctx)

	if err != nil {
		var zero time.Time
		return zero, err
	}
	return storageTime.Add(x.options.LeaseExpireAfter), nil
}

// ------------------------------------------------- --------------------------------------------------------------------

// UnLock 尝试释放锁，如果释放不成功的话则会返回error
// ownerId: 是谁在尝试释放锁，如果不指定的话会为当前协程生成一个默认的ownerId
func (x *StorageLock) UnLock(ctx context.Context, ownerId ...string) error {

	e := events.NewEvent(x.options.LockId).SetType(events.EventTypeUnlock).SetStorageName(x.storage.GetName()).SetListeners(x.options.EventListeners)

	for {

		err := x.unlockWithRetry(ctx, e, ownerId...)
		if err == nil {
			e.AddActionByName("unlock-success").Publish(ctx)
			return nil
		}

		// TODO 2023-6-21 00:33:21 错误处理
		e.Fork().AddActionByName("unlock-error").SetErr(err).Publish(ctx)

		select {
		case <-ctx.Done():
			e.AddActionByName("timeout").Publish(ctx)
			return err
		case <-time.After(time.Microsecond):
			e.Fork().AddActionByName("sleep-retry").Publish(ctx)
			time.Sleep(time.Second * time.Duration(rand.Intn(5)+1))
			continue
		}
	}
}

// unlockWithRetry 手动指定重试次数的释放锁，如果锁竞争较大的话应该适当提高乐观锁的失败重试次数
func (x *StorageLock) unlockWithRetry(ctx context.Context, e *events.Event, ownerId ...string) error {

	e.AddActionByName("unlock-with-retry")

	// 如果没有指定ownerId的话，则为其生成一个默认的ownerId
	if len(ownerId) == 0 {
		e.AddActionByName("with-default-owner-id")
		ownerId = append(ownerId, x.ownerIdGenerator.getDefaultOwnId())
	} else if len(ownerId) >= 2 {
		e.AddActionByName("owner-id-set-error").AddActionByName(strings.Join(ownerId, ", ")).Publish(ctx)
		return ErrOwnerCanOnlyOne
	}

	// 尝试读取锁的信息
	lockInformation, err := x.getLockInformation(ctx, e)

	// 如果锁的信息都读取失败了，则没必要继续下去，这里没必要区分是锁不存在的错误还是其它错误，反正只要是错误就直接中断返回
	if err != nil {
		e.AddActionByName("get-lock-information-error").SetErr(err).Publish(ctx)
		return err
	}

	// 如果读取到的锁的信息为空，则说明锁不存在，一个不存在的锁自然也没有继续的必要
	if lockInformation == nil {
		e.AddActionByName("lock-information-not-exists").Publish(ctx)
		return ErrLockNotFound
	}

	// 如果锁的当前持有者的ID不是自己，则无权释放锁
	if lockInformation.OwnerId != ownerId[0] {
		e.AddActionByName("not-lock-owner").Publish(ctx)
		return ErrLockNotBelongYou
	}

	// 通过了前面的检查，确实锁是自己持有的，则开始对锁进行操作
	lastVersion := lockInformation.Version
	lockInformation.Version++
	lockInformation.LockCount--

	// 如果释放一次之后发现还没有释放干净，说明是重入锁，并且加锁次数还没有为0，则尝试更新锁的信息
	if lockInformation.LockCount > 0 {
		e.AddActionByName("to-reentry-unlock").Publish(ctx)
		return x.reentryUnlock(ctx, e.Fork(), ownerId[0], lockInformation, lastVersion)
	} else {
		// 如果经过这次操作之后锁的锁的锁定次数为0，说明应该彻底释放掉这个锁了，将其从Storage中清除
		e.AddActionByName("to-unlock-with-clean").Publish(ctx)
		return x.unlockWithClean(ctx, e.Fork(), ownerId[0], lockInformation, lastVersion)
	}
}

// 可重入锁的层级减一，但是并没有彻底释放，更新数据库中的锁的信息
func (x *StorageLock) reentryUnlock(ctx context.Context, e *events.Event, ownerId string, lockInformation *storage.LockInformation, lastVersion storage.Version) error {

	e.AddActionByName("reentry-unlock").SetLockInformation(lockInformation)

	// 更新锁的过期时间
	expireTime, err := x.getLeaseExpireTime(ctx, e)
	if err != nil {
		e.AddActionByName("get-lease-expire-time-error").SetErr(err).Publish(ctx)
		return err
	}
	lockInformation.LeaseExpireTime = expireTime

	updateAction := events.NewAction(ActionStorageUpdateWithVersion)
	err = x.storage.UpdateWithVersion(ctx, x.options.LockId, lastVersion, lockInformation.Version, lockInformation)
	e.Fork().AddAction(updateAction.End().SetErr(err).SetPayload(lockInformation.ToJsonString())).Publish(ctx)

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
	select {
	case <-ctx.Done():
		// 更新失败，并且也没有重试次数了，则只好返回错误
		return ErrUnlockFailed
	case <-time.After(time.Microsecond):
		// 我还有重试次数，我要尝试重试
		return x.unlockWithRetry(ctx, e, ownerId)
	}
}

// 锁被彻底释放干净了，需要将其从Storage中清除
func (x *StorageLock) unlockWithClean(ctx context.Context, e *events.Event, ownerId string, lockInformation *storage.LockInformation, lastVersion storage.Version) error {

	e.AddActionByName("unlockWithClean").SetLockInformation(lockInformation)

	// 重入锁的次数已经被释放干净了，现在需要将其彻底删除
	deleteAction := events.NewAction(ActionStorageDeleteWithVersion)
	err := x.storage.DeleteWithVersion(ctx, x.options.LockId, lastVersion, lockInformation)
	e.Fork().AddAction(deleteAction.End().SetErr(err).SetPayload(lockInformation.ToJsonString())).Publish(ctx)

	// 如果删除的时候遇到错误，则直接认为锁释放失败
	if err != nil {

		e.AddActionByName("storage-delete-with-version-error")

		if errors.Is(err, ErrVersionMiss) {

			e.AddActionByName("is-version-miss-error")

			// 还有重试次数，则再次尝试删除锁
			select {
			case <-ctx.Done():
				// 没有重试次数了，则只好返回错误
				e.AddActionByName("timeout").Publish(ctx)
				return ErrLockFailed
			case <-time.After(time.Microsecond):
				e.AddActionByName("to-unlock-with-retry").Publish(ctx)
				return x.unlockWithRetry(ctx, e.Fork(), ownerId)
			}

		} else {
			e.AddActionByName("is-other-error").SetErr(err).Publish(ctx)
			return err
		}
	}

	// 执行到这里表示已经删除成功了，则将租约续期的协程停掉
	e.Fork().AddActionByName("stop-watch-dog").SetWatchDogId(x.storageLockWatchDog.GetID()).Publish(ctx)
	x.storageLockWatchDog.Stop()

	e.AddActionByName("done").Publish(ctx)
	return nil
}

//// UnLockUntilRelease 一直unlock直到释放掉锁，可能的场景是可重入锁重启之后清除之前可能存在的锁状态
//func (x *StorageLock) UnLockUntilRelease(ctx context.Context, ownerId ...string) error {
//	// TODO 递归可能会有溢出的风险，修改为迭代实现
//	err := x.UnLock(ctx, ownerId...)
//	if err != nil {
//		if errors.Is(err, ErrLockNotFound) {
//			return nil
//		} else {
//			return err
//		}
//	} else {
//		return x.UnLockUntilRelease(ctx, ownerId...)
//	}
//}

// 获取之前的锁保存的信息
func (x *StorageLock) getLockInformation(ctx context.Context, e *events.Event) (*storage.LockInformation, error) {

	getAction := events.NewAction(ActionStorageGet)
	lockInformationJsonString, err := x.storage.Get(ctx, x.options.LockId)
	e.Fork().AddAction(getAction.End().SetErr(err).SetPayload(lockInformationJsonString)).Publish(ctx)

	if err != nil {
		return nil, err
	}

	if lockInformationJsonString == "" {
		return nil, ErrLockNotFound
	}

	return storage.LockInformationFromJsonString(lockInformationJsonString)
}

// ------------------------------------------------- --------------------------------------------------------------------
