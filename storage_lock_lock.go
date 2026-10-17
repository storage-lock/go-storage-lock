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
//	ownerId: 是谁在尝试获取锁，必须是全局唯一的标识。
//	        ⚠️ 重要：Lock 和 UnLock 必须使用相同的 ownerId，否则无法释放锁！
//	        可以使用 OwnerIdGenerator.GenOwnerId() 生成，但请注意每次调用返回不同的 ID，
//	        因此需要先保存到变量中再使用。
//
// @returns:
//
//	error: 当获取锁的时候发生错误的时候会中断竞争锁并返回错误
func (x *StorageLock) Lock(ctx context.Context, ownerId string) error {
	if err := validateOwner(ctx, ownerId); err != nil {
		return err
	}

	lockId := x.options.LockId

	// 触发一个获取锁的事件
	e := events.NewEvent(lockId).SetOwnerId(ownerId).SetType(events.EventTypeLock).SetListeners(x.options.EventListeners).SetStorageName(x.storage.GetName())

	// 先触发一个开始获取锁的事件
	e.AddActionByName(ActionLockBegin).Publish(ctx)

	// 记录操作时版本miss的次数
	versionMissCount := 0
	lockBusyCount := 0

	// 在方法退出的时候发送事件通知
	defer func() {
		e.Fork().AddAction(events.NewAction(ActionLockFinish).AddPayload(PayloadVersionMissCount, versionMissCount)).Publish(ctx)
	}()

	// 公平模式下先把自己登记进存储里的队列，锁记录还不存在时也要登记。
	// 登记失败就没有排队，直接返回，否则调用方会在队列之外空转，把已经排队的人挤掉。
	if x.options.Fair {
		if err := x.ensureFairQueued(ctx, ownerId); err != nil {
			return err
		}
	}

	// 然后开始循环获取锁
	for {

		// 尝试获取锁
		err := x.tryLock(ctx, e.Fork(), lockId, ownerId)
		if err == nil {
			// 获取锁成功，退出
			e.Fork().AddAction(events.NewAction(ActionLockSuccess).AddPayload(PayloadVersionMissCount, versionMissCount).AddPayload(PayloadLockBusyCount, lockBusyCount)).Publish(ctx)
			return nil
		}

		// 只有在版本miss的情况下或者锁被其它人持有者的情况才会等待重试
		// 锁已经存在的错误被认为是版本miss的一种特殊情况
		if errors.Is(err, ErrVersionMiss) || errors.Is(err, ErrLockAlreadyExists) {
			// 尝试获取锁的时候版本miss了，触发一个获取锁版本miss的事件让外部能够感知得到
			versionMissCount++
			e.Fork().AddAction(events.NewAction(ActionLockVersionMiss).AddPayload(PayloadVersionMissCount, versionMissCount).AddPayload(PayloadLockBusyCount, lockBusyCount)).Publish(ctx)
		} else if errors.Is(err, ErrLockBusy) {
			// 锁被其它人持有着，勇敢牛牛，不怕困难，稍微一等，继续重试
			lockBusyCount++
			x.publishBusy(ctx, ownerId)
			e.Fork().AddAction(events.NewAction(ActionLockBusy).AddPayload(PayloadVersionMissCount, versionMissCount).AddPayload(PayloadLockBusyCount, lockBusyCount)).Publish(ctx)
		} else if errors.Is(err, ErrNotYourTurn) {
			// 队列里还有人排在前面。继续等，但不算作锁被占用。
			x.publishBusy(ctx, ownerId)
			e.Fork().AddAction(events.NewAction(ActionLockBusy).AddPayload(PayloadVersionMissCount, versionMissCount).AddPayload(PayloadLockBusyCount, lockBusyCount)).Publish(ctx)
		} else {
			// 其它类型的错误就不再管了，认为是获取锁失败
			if x.options.Fair {
				x.abandonWait(ownerId, waiterModeWrite)
			}
			e.Fork().AddAction(events.NewAction(ActionLockError).SetErr(err).AddPayload(PayloadVersionMissCount, versionMissCount).AddPayload(PayloadLockBusyCount, lockBusyCount)).Publish(ctx)
			return err
		}

		// 然后休眠一下再开始重新抢占锁
		sleepDuration := x.options.VersionMissRetryInterval + x.retryIntervalRandomBase()
		e.Fork().AddAction(events.NewAction(ActionSleep).AddPayload(PayloadSleep, sleepDuration)).Publish(ctx)
		if err := waitContext(ctx, sleepDuration); err != nil {
			if x.options.Fair {
				x.abandonWait(ownerId, waiterModeWrite)
			}
			e.Fork().AddAction(events.NewAction(ActionTimeout).AddPayload(PayloadVersionMissCount, versionMissCount).AddPayload(PayloadLockBusyCount, lockBusyCount)).Publish(ctx)
			return err
		}
		e.Fork().AddAction(events.NewAction(ActionSleepRetry).AddPayload(PayloadVersionMissCount, versionMissCount).AddPayload(PayloadLockBusyCount, lockBusyCount)).Publish(ctx)
	}
}

// TryLock 尝试一次获取独占锁，不等待持有者释放，也不等待 CAS 冲突重试。
// 竞争失败统一返回 ErrLockBusy；上下文取消和存储错误原样返回。
// 同 ownerId 的有效独占锁仍按 Lock 的规则增加重入深度。
// 本方法仍需执行存储 RPC，调用方应通过 ctx 限制 RPC 耗时。
func (x *StorageLock) TryLock(ctx context.Context, ownerId string) error {
	if err := validateOwner(ctx, ownerId); err != nil {
		return err
	}
	e := x.operationEvent(ownerId).SetType(events.EventTypeLock)
	e.AddActionByName(ActionLockBegin).Publish(ctx)
	defer e.Fork().AddActionByName(ActionLockFinish).Publish(ctx)
	err := x.tryLock(ctx, e.Fork(), x.options.LockId, ownerId)
	if errors.Is(err, ErrVersionMiss) || errors.Is(err, ErrLockAlreadyExists) || errors.Is(err, ErrNotYourTurn) {
		err = ErrLockBusy
	}
	if err == nil {
		e.Fork().AddActionByName(ActionLockSuccess).Publish(ctx)
	} else {
		if errors.Is(err, ErrLockBusy) {
			x.publishBusy(ctx, ownerId)
		}
		e.Fork().AddAction(events.NewAction(ActionLockError).SetErr(err)).Publish(ctx)
	}
	return err
}

// tryLock 带重试次数的获取锁，因为乐观锁的失败率可能会比较高
func (x *StorageLock) tryLock(ctx context.Context, e *events.Event, lockId, ownerId string) error {
	if err := validateOwner(ctx, ownerId); err != nil {
		return err
	}

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

	storageTime, err := x.getTime(ctx, e.Fork())
	if err != nil {
		e.Fork().AddAction(events.NewAction(storage_events.ActionStorageGetTimeError).SetErr(err)).Publish(ctx)
		return err
	}

	// 看下锁是否已经过期了，如果已经过期了的话，则直接开始尝试抢占锁
	if !storageTime.Before(lockInformation.LeaseExpireTime) {
		return x.lockExpired(ctx, e.Fork(), lockId, ownerId, storageTime, lockInformation)
	}

	// 释放后保留版本记录，保证下一次获取的栅栏令牌严格递增。
	if lockInformation.LockCount == 0 {
		return x.lockExpired(ctx, e.Fork(), lockId, ownerId, storageTime, lockInformation)
	}

	// 锁没过期的话，又分为两种情况，一种是锁就是自己持有的，一种是锁被别人持有
	if !lockInformation.ReadLock && lockInformation.OwnerId == ownerId {
		return x.lockReentry(ctx, e.Fork(), lockId, ownerId, lockInformation)
	} else {
		// 锁被其他人占用着，暂时不能尝试获取锁
		x.publishBusy(ctx, ownerId)
		e.Fork().AddAction(events.NewAction(ActionLockBusy).AddPayload(storage_events.PayloadLockInformation, lockInformation)).Publish(ctx)
		return ErrLockBusy
	}
}

// 尝试抢占已经过期的锁
func (x *StorageLock) lockExpired(ctx context.Context, e *events.Event, lockId, ownerId string, storageTime time.Time, information *storage.LockInformation) error {
	version, err := nextVersion(information.Version)
	if err != nil {
		return err
	}

	// 记录上有排队时，空闲或过期只说明锁可以换人，不说明谁都能拿。
	// 这条约束不看调用方自己开没开公平：没排队的客户端提交的 CAS 会把队首抹掉。
	// 队首离开后，其余等待者原样保留，下一次抢占继续由新的队首接手。
	carried := information.Waiters
	if head := firstWriteWaiter(information.Waiters); head != nil {
		if head.OwnerId != ownerId {
			return ErrNotYourTurn
		}
		carried = removeWaiter(information.Waiters, ownerId, waiterModeWrite)
	}

	// 过期的锁认为是失效了，除了lockId其它都跟之前不一样了
	newLockInformation := &storage.LockInformation{
		LockId:          lockId,
		OwnerId:         ownerId,
		Version:         version,
		LockCount:       1,
		LockBeginTime:   storageTime,
		LeaseExpireTime: storageTime.Add(x.options.LeaseExpireAfter),
		Waiters:         carried,
	}
	e.AddAction(events.NewAction(ActionLockExpired).AddPayload(storage_events.PayloadLockInformation, newLockInformation))

	// 抢占锁
	err = x.storageExecutor.UpdateWithVersion(ctx, e.Fork(), lockId, information.Version, newLockInformation.Version, newLockInformation)
	if err != nil {
		// 妈的，抢占失败
		if errors.Is(err, ErrVersionMiss) {
			e.Fork().AddAction(events.NewAction(storage_events.ActionStorageUpdateWithVersionMiss)).Publish(ctx)
			return ErrVersionMiss
		} else {
			e.Fork().AddAction(events.NewAction(storage_events.ActionStorageUpdateWithVersionError).SetErr(err)).Publish(ctx)
			return err
		}
	}

	// 抢占成功
	e.Fork().AddActionByName(storage_events.ActionStorageUpdateWithVersionSuccess).Publish(ctx)
	x.notifySubscribers(lockId, newLockInformation)
	x.publishExpired(ctx, ownerId)
	x.abandonWait(ownerId, waiterModeWrite)

	// 抢占过期锁相当于新持有，需要启动新的看门狗来续租
	// 之前可能有残留的看门狗协程（理论上不应该有，但防御性清理一下）
	x.stopWatchDog(ctx, e.Fork(), newLockInformation)
	return x.startWatchDog(ctx, e.Fork(), lockId, ownerId, newLockInformation)
}

// 进入重入锁的逻辑，尝试对可重入锁的层级加一
func (x *StorageLock) lockReentry(ctx context.Context, e *events.Event, lockId, ownerId string, lockInformation *storage.LockInformation) error {

	e.SetLockId(lockId).SetLockInformation(lockInformation).AddActionByName(ActionLockReentry).Publish(ctx)

	// 计算从当前时间开始计算的租约的过期时间
	expireTime, err := x.activeLeaseExpiry(ctx, e.Fork(), lockInformation)
	if err != nil {
		e.Fork().AddAction(events.NewAction(ActionGetLeaseExpireTimeError).SetErr(err)).Publish(ctx)
		return err
	}

	if x.options.MaxReentrantDepth > 0 && lockInformation.LockCount >= x.options.MaxReentrantDepth {
		return ErrMaxReentrantDepth
	}

	oldVersion := lockInformation.Version

	// 这个锁当前就是自己持有的，那进行了一次更改，版本增加
	lockInformation.Version, err = nextVersion(oldVersion)
	if err != nil {
		return err
	}
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
		x.notifySubscribers(lockId, lockInformation)
		x.abandonWait(ownerId, waiterModeWrite)
		return nil
	}
}

// 尝试获取不存在的锁，这个是最爽的分支，能够直接获取到锁
func (x *StorageLock) lockNotExists(ctx context.Context, e *events.Event, lockId, ownerId string, lockInformation *storage.LockInformation) error {

	// 触发事件先
	e.SetLockId(lockId).SetOwnerId(ownerId).SetLockInformation(lockInformation).AddActionByName(ActionLockNotExists).Publish(ctx)

	// 获取Storage的时间（或外部注入的时间源）
	storageTime, err := x.getTime(ctx, e.Fork())
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
	x.notifySubscribers(lockId, lockInformation)
	x.abandonWait(ownerId, waiterModeWrite)

	// 插入成功，看下如果之前有续租协程的话就停掉，这一步是为了防止之前有资源未清理干净
	x.stopWatchDog(ctx, e.Fork(), lockInformation)

	// 为自己创建并启动一只新的看门狗
	return x.startWatchDog(ctx, e.Fork(), lockId, ownerId, lockInformation)
}

// startWatchDog 创建并启动一只新的看门狗协程，用于在锁持有期间自动续租
// 如果看门狗创建或启动失败，会尝试回滚（释放）刚获取的锁
func (x *StorageLock) startWatchDog(ctx context.Context, e *events.Event, lockId, ownerId string, lockInformation *storage.LockInformation) error {
	x.resetOwnershipLost()
	if x.options.DisableWatchDog {
		return nil
	}

	// 为自己创建一只新的看门狗
	watchDog, err := x.options.WatchDogFactory.NewWatchDog(ctx, e.Fork(), x, ownerId)
	if err != nil {
		// 看门狗创建失败，尝试释放掉锁
		x.lockRollback(ctx, e.Fork(), lockId, ownerId, lockInformation)
		x.setWatchDog(nil)
		e.Fork().AddAction(events.NewAction(ActionWatchDogCreateError).SetErr(err)).Publish(ctx)
		return err
	}
	x.setWatchDog(watchDog)
	e.SetWatchDogId(watchDog.GetID()).Fork().AddAction(events.NewAction(ActionWatchDogCreateSuccess)).Publish(ctx)

	// 启动这只看门狗
	err = watchDog.Start(ctx)
	if err != nil {
		x.setWatchDog(nil)
		// 看门狗启动失败，尝试释放掉锁
		x.lockRollback(ctx, e.Fork(), lockId, ownerId, lockInformation)
		e.Fork().AddAction(events.NewAction(ActionWatchDogStartError).SetErr(err)).Publish(ctx)
		return err
	}
	e.Fork().AddAction(events.NewAction(ActionWatchDogStartSuccess)).Publish(ctx)

	return nil
}

// 获取到锁了，但是因为种种原因没办法真的获取成功，于是就尝试对其进行回滚
func (x *StorageLock) lockRollback(ctx context.Context, e *events.Event, lockId, ownerId string, lockInformation *storage.LockInformation) {
	ctx, cancel := x.cleanupContext(ctx)
	defer cancel()

	// 尽力而为回滚锁，如果释放不掉也只能慢慢等它过期了

	e.AddAction(events.NewAction(ActionLockRollback)).Publish(ctx)

	// 回滚也保留版本，避免删除再创建造成 ABA。
	version, err := nextVersion(lockInformation.Version)
	if err != nil {
		e.Fork().AddAction(events.NewAction(ActionLockRollbackError).SetErr(err)).Publish(ctx)
		return
	}

	// 墓碑回滚：版本号 +1，LockCount=0
	tombstone := &storage.LockInformation{
		LockId:          lockInformation.LockId,
		OwnerId:         lockInformation.OwnerId,
		Version:         version,
		LockCount:       0,
		LockBeginTime:   lockInformation.LockBeginTime,
		LeaseExpireTime: lockInformation.LeaseExpireTime,
		ReadLock:        lockInformation.ReadLock,
	}
	err = x.storageExecutor.UpdateWithVersion(ctx, e.Fork(), lockId, lockInformation.Version, tombstone.Version, tombstone)
	if err != nil {
		e.Fork().AddAction(events.NewAction(ActionLockRollbackError).SetErr(err)).Publish(ctx)
	} else {
		e.Fork().AddAction(events.NewAction(ActionLockRollbackSuccess)).Publish(ctx)
		x.notifySubscribers(lockId, tombstone)
	}
}
