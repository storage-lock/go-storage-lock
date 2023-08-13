package storage_lock

import (
	"context"
	"errors"
	"github.com/storage-lock/go-events"
	"github.com/storage-lock/go-storage"
	storage_events "github.com/storage-lock/go-storage-events"
	"time"
)

// Lock 尝试获取锁
// @params:
//
//	ctx: 用来控制超时，如果想永远不超时则传入context.Background()
//	ownerId: 是谁在尝试获取锁，如果不指定的话会为当前协程生成一个默认的ownerId
//
// @returns:
//
//	error: 当获取锁的时候发生错误的时候会中断竞争锁并返回错误
func (x *StorageLock) Lock(ctx context.Context, ownerId string) error {

	lockId := x.options.LockId

	// 触发一个获取锁的事件
	e := events.NewEvent(lockId).SetOwnerId(ownerId).SetType(events.EventTypeLock).SetListeners(x.options.EventListeners).SetStorageName(x.storage.GetName())

	// 先触发一个开始获取锁的事件
	e.AddActionByName(ActionLockBegin).Publish(ctx)

	// 记录操作时版本miss的次数
	versionMissCount := 0

	// 在方法退出的时候发送事件通知
	defer func() {
		e.Fork().AddAction(events.NewAction(ActionLockFinish).AddPayload(PayloadVersionMissCount, versionMissCount)).Publish(ctx)
	}()

	// 然后开始循环获取锁
	for {

		// 尝试获取锁
		err := x.tryLock(ctx, e.Fork(), lockId, ownerId)
		if err == nil {
			// 获取锁成功，退出
			e.Fork().AddAction(events.NewAction(ActionLockSuccess).AddPayload(PayloadVersionMissCount, versionMissCount)).Publish(ctx)
			return nil
		}

		// 只有在版本miss的情况下才会重试
		if !errors.Is(err, ErrVersionMiss) {
			e.Fork().AddAction(events.NewAction(ActionLockError).SetErr(err).AddPayload(PayloadVersionMissCount, versionMissCount)).Publish(ctx)
			return err
		}
		// 尝试获取锁的时候版本miss了，触发一个获取锁版本miss的事件让外部能够感知得到
		versionMissCount++
		e.Fork().AddAction(events.NewAction(ActionLockVersionMiss).AddPayload(PayloadVersionMissCount, versionMissCount)).Publish(ctx)

		// 然后休眠一下再开始重新抢占锁
		sleepDuration := x.options.VersionMissRetryInterval + x.retryIntervalRandomBase()
		e.Fork().AddAction(events.NewAction(ActionSleep).AddPayload(PayloadSleep, sleepDuration)).Publish(ctx)
		time.Sleep(sleepDuration)

		// 然后开始重试
		select {
		case <-ctx.Done():
			// 没有时间了，算球没获取成功
			e.Fork().AddAction(events.NewAction(ActionTimeout).AddPayload(PayloadVersionMissCount, versionMissCount)).Publish(ctx)
			return err
		default:
			// 还有时间，可以尝试重新获取
			e.Fork().AddAction(events.NewAction(ActionSleepRetry).AddPayload(PayloadVersionMissCount, versionMissCount)).Publish(ctx)
			continue
		}
	}
}

// tryLock 带重试次数的获取锁，因为乐观锁的失败率可能会比较高
func (x *StorageLock) tryLock(ctx context.Context, e *events.Event, lockId, ownerId string) error {

	// 触发开始获取锁的事件
	e.SetLockId(lockId).SetOwnerId(ownerId).AddAction(events.NewAction(ActionTryLockBegin)).Publish(ctx)

	// 先尝试从Storage中读取上次存储的锁的信息
	lockInformation, err := x.getLockInformation(ctx, e, lockId)
	// 如果读取锁的时候发生错误，除非是锁不存在的错误，否则都认为是中断执行
	if err != nil && !errors.Is(err, ErrLockNotFound) {
		e.Fork().AddAction(events.NewAction(ActionGetLockInformationError).SetErr(err)).Publish(ctx)
		return err
	}

	// 如果锁的信息存在，则说明之前锁就已经存在了
	if lockInformation != nil {
		return x.lockExists(ctx, e.Fork(), lockId, ownerId, lockInformation)
	} else {
		// 否则认为之前锁是不存在的
		return x.lockNotExists(ctx, e.Fork(), lockId, ownerId, lockInformation)
	}
}

// 尝试获取已经存在的锁
func (x *StorageLock) lockExists(ctx context.Context, e *events.Event, lockId, ownerId string, lockInformation *storage.LockInformation) error {

	e.SetLockId(lockId).SetOwnerId(ownerId).AddActionByName(ActionLockExists).Publish(ctx)

	storageTime, err := x.storageExecutor.GetTime(ctx, e.Fork())
	if err != nil {
		e.Fork().AddAction(events.NewAction(storage_events.ActionStorageGetTimeError).SetErr(err)).Publish(ctx)
		return err
	}

	// 看下锁是不是一个被释放的锁，如果是的话则尝试开始抢占锁
	if lockInformation.LockCount == 0 {
		return x.lockReleased(ctx, e.Fork(), lockId, ownerId, storageTime, lockInformation)
	}

	// 看下锁是否已经过期了，如果已经过期了的话，则直接开始尝试抢占锁
	if storageTime.After(lockInformation.LeaseExpireTime) {
		return x.lockExpired(ctx, e.Fork(), lockId, ownerId, storageTime, lockInformation)
	}

	// 锁没过期的话，又分为两种情况，一种是锁就是自己持有的，一种是锁被别人持有
	if lockInformation.OwnerId == ownerId {
		return x.lockReentry(ctx, e.Fork(), lockId, ownerId, lockInformation)
	} else {
		// 锁被其他人占用着，暂时不能尝试获取锁
		e.Fork().AddAction(events.NewAction(ActionLockBusy).AddPayload(storage_events.PayloadLockInformation, lockInformation)).Publish(ctx)
		return ErrLockBusy
	}
}

// 尝试抢占一个已经被释放了的锁
func (x *StorageLock) lockReleased(ctx context.Context, e *events.Event, lockId, ownerId string, storageTime time.Time, information *storage.LockInformation) error {

	// 创建锁的信息，除了lockId其它的都跟之前不一样了
	newLockInformation := &storage.LockInformation{
		LockId:          lockId,
		OwnerId:         ownerId,
		Version:         information.Version + 1,
		LockCount:       1,
		LockBeginTime:   storageTime,
		LeaseExpireTime: storageTime.Add(x.options.LeaseExpireAfter),
	}
	e.AddAction(events.NewAction(ActionLockReleased).AddPayload(storage_events.PayloadLockInformation, newLockInformation))

	// 开始抢占锁
	err := x.storageExecutor.UpdateWithVersion(ctx, e.Fork(), lockId, information.Version, newLockInformation.Version, newLockInformation)
	if err != nil {
		if errors.Is(err, ErrVersionMiss) {
			e.Fork().AddAction(events.NewAction(storage_events.ActionStorageUpdateWithVersionMiss)).Publish(ctx)
			return ErrVersionMiss
		} else {
			e.Fork().AddAction(events.NewAction(storage_events.ActionStorageUpdateWithVersionError).SetErr(err)).Publish(ctx)
			return err
		}
	} else {
		// 抢占成功，成功拿到了锁
		e.Fork().AddAction(events.NewAction(storage_events.ActionStorageUpdateWithVersionSuccess)).Publish(ctx)
		return nil
	}
}

// 尝试抢占已经过期的锁
func (x *StorageLock) lockExpired(ctx context.Context, e *events.Event, lockId, ownerId string, storageTime time.Time, information *storage.LockInformation) error {

	// 过期的锁认为是失效了，除了lockId其它都跟之前不一样了
	newLockInformation := &storage.LockInformation{
		LockId:          lockId,
		OwnerId:         ownerId,
		Version:         information.Version + 1,
		LockCount:       1,
		LockBeginTime:   storageTime,
		LeaseExpireTime: storageTime.Add(x.options.LeaseExpireAfter),
	}
	e.AddAction(events.NewAction(ActionLockExpired).AddPayload(storage_events.PayloadLockInformation, newLockInformation))

	// 抢占锁
	err := x.storageExecutor.UpdateWithVersion(ctx, e.Fork(), lockId, information.Version, newLockInformation.Version, newLockInformation)
	if err != nil {
		// 妈的，抢占失败
		if errors.Is(err, ErrVersionMiss) {
			e.Fork().AddAction(events.NewAction(storage_events.ActionStorageUpdateWithVersionMiss)).Publish(ctx)
			return ErrVersionMiss
		} else {
			e.Fork().AddAction(events.NewAction(storage_events.ActionStorageUpdateWithVersionError).SetErr(err)).Publish(ctx)
			return err
		}
	} else {
		// 抢占成功
		e.Fork().AddActionByName(storage_events.ActionStorageUpdateWithVersionSuccess).Publish(ctx)
		return nil
	}
}

// 进入重入锁的逻辑，尝试对可重入锁的层级加一
func (x *StorageLock) lockReentry(ctx context.Context, e *events.Event, lockId, ownerId string, lockInformation *storage.LockInformation) error {

	e.SetLockId(lockId).SetLockInformation(lockInformation).AddActionByName(ActionLockReentry).Publish(ctx)

	// 计算从当前时间开始计算的租约的过期时间
	expireTime, err := x.getLeaseExpireTime(ctx, e.Fork())
	if err != nil {
		e.Fork().AddAction(events.NewAction(ActionGetLeaseExpireTimeError).SetErr(err)).Publish(ctx)
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
	err = x.storageExecutor.UpdateWithVersion(ctx, e.Fork(), lockId, oldVersion, lockInformation.Version, lockInformation)
	// 更新成功，则本次获取锁成功，可重入锁的层级又深了一层
	if err != nil {
		if !errors.Is(err, ErrVersionMiss) {
			e.Fork().AddAction(events.NewAction(storage_events.ActionStorageUpdateWithVersionError).SetErr(err)).Publish(ctx)
			return err
		} else {
			e.Fork().AddActionByName(storage_events.ActionStorageUpdateWithVersionMiss).Publish(ctx)
			return ErrVersionMiss
		}
	} else {
		e.Fork().AddActionByName(storage_events.ActionStorageUpdateWithVersionSuccess).Publish(ctx)
		return nil
	}
}

// 尝试获取不存在的锁，这个是最爽的分支，能够直接获取到锁
func (x *StorageLock) lockNotExists(ctx context.Context, e *events.Event, lockId, ownerId string, lockInformation *storage.LockInformation) error {

	// 触发事件先
	e.SetLockId(lockId).SetOwnerId(ownerId).SetLockInformation(lockInformation).AddActionByName(ActionLockNotExists).Publish(ctx)

	// 获取Storage的时间
	storageTime, err := x.storageExecutor.GetTime(ctx, e.Fork())
	if err != nil {
		// 完蛋，出师未捷身先死，获取时间就没获取到
		e.Fork().AddAction(events.NewAction(storage_events.ActionStorageGetTimeError).SetErr(err)).Publish(ctx)
		return err
	}

	// 计算从Storage的当前时间开始计算的租约的过期时间
	expireTime := storageTime.Add(x.options.LeaseExpireAfter)

	// 锁还不存在，那尝试持有它
	lockInformation = &storage.LockInformation{
		LockId:  lockId,
		OwnerId: ownerId,
		// 锁的开始时间跟Storage的时间保持一致，这样才好有对比
		LockBeginTime: storageTime,
		// 因为这个锁之前还没存在过，所以这个版本号就从1开始
		Version:         1,
		LockCount:       1,
		LeaseExpireTime: expireTime,
	}
	e.SetLockInformation(lockInformation)

	// 尝试创建一条记录
	err = x.storageExecutor.CreateWithVersion(ctx, e.Fork(), lockId, lockInformation.Version, lockInformation)
	if err != nil {
		if errors.Is(err, ErrVersionMiss) {
			// 版本未命中，抢占失败
			e.Fork().AddAction(events.NewAction(storage_events.ActionStorageCreateWithVersionMiss).SetErr(err)).Publish(ctx)
			return ErrVersionMiss
		} else {
			// 发生了其它错误
			e.Fork().AddAction(events.NewAction(storage_events.ActionStorageCreateWithVersionError).SetErr(err)).Publish(ctx)
			return err
		}
	}
	// 锁抢占成功
	e.Fork().AddAction(events.NewAction(storage_events.ActionStorageCreateWithVersionSuccess)).Publish(ctx)

	// 插入成功，看下如果之前有续租协程的话就停掉，这一步是为了防止之前有资源未清理干净
	if x.storageLockWatchDog != nil {
		stopLastWatchDogEvent := e.Fork().AddActionByName(ActionWatchDogStop).SetWatchDogId(x.storageLockWatchDog.GetID())
		x.storageLockWatchDog = nil
		err := x.storageLockWatchDog.Stop(ctx)
		if err != nil {
			stopLastWatchDogEvent.AddAction(events.NewAction(ActionWatchDogStopError).SetErr(err))
		} else {
			stopLastWatchDogEvent.AddAction(events.NewAction(ActionWatchDogStopSuccess))
		}
		stopLastWatchDogEvent.Publish(ctx)
	}

	// 为自己创建一只新的看门狗
	x.storageLockWatchDog, err = x.options.WatchDogFactory.New(ctx, e.Fork(), x, ownerId)
	if err != nil {
		// 看门狗创建失败，尝试释放掉锁
		x.lockRollback(ctx, e.Fork(), lockId, ownerId, lockInformation)
		x.storageLockWatchDog = nil
		e.Fork().AddAction(events.NewAction(ActionWatchDogCreateError).SetErr(err)).Publish(ctx)
		return err
	}
	e.SetWatchDogId(x.storageLockWatchDog.GetID()).Fork().AddAction(events.NewAction(ActionWatchDogCreateSuccess)).Publish(ctx)

	// 启动这只看门狗
	err = x.storageLockWatchDog.Start(ctx)
	if err != nil {
		x.storageLockWatchDog = nil
		// 看门狗创建失败，尝试释放掉锁
		x.lockRollback(ctx, e.Fork(), lockId, ownerId, lockInformation)
		e.Fork().AddAction(events.NewAction(ActionWatchDogStartError).SetErr(err)).Publish(ctx)
		return err
	}
	e.Fork().AddAction(events.NewAction(ActionWatchDogStartSuccess)).Publish(ctx)

	return nil
}

// 获取到锁了，但是因为种种原因没办法真的获取成功，于是就尝试对齐进行回滚
func (x *StorageLock) lockRollback(ctx context.Context, e *events.Event, lockId, ownerId string, lockInformation *storage.LockInformation) {
	// 尽力而为释放锁，如果释放不掉也只能慢慢等它过期了

	e.AddAction(events.NewAction(ActionLockRollback)).Publish(ctx)

	lastVersion := lockInformation.Version
	lockInformation.Version++
	lockInformation.LockCount = 0
	err := x.unlockRelease(ctx, e.Fork(), lockId, ownerId, lockInformation, lastVersion)
	if err != nil {
		e.Fork().AddAction(events.NewAction(ActionLockRollbackError).SetErr(err)).Publish(ctx)
	} else {
		e.Fork().AddAction(events.NewAction(ActionLockRollbackSuccess)).Publish(ctx)
	}
}
