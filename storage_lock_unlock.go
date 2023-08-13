package storage_lock

import (
	"context"
	"errors"
	"github.com/storage-lock/go-events"
	"github.com/storage-lock/go-storage"
	storage_events "github.com/storage-lock/go-storage-events"
	"time"
)

// StorageLock中与释放锁相关的逻辑拆分到这个文件中，以防止逻辑都放在一个文件中内容太长不好管理

// UnLock 尝试释放锁，如果释放不成功的话则会返回error
// ownerId: 是谁在尝试释放锁，操作者应该有唯一的标识
func (x *StorageLock) UnLock(ctx context.Context, ownerId string) error {

	lockId := x.options.LockId
	e := events.NewEvent(lockId).SetType(events.EventTypeUnlock).SetStorageName(x.storage.GetName()).SetListeners(x.options.EventListeners).SetOwnerId(ownerId)

	versionMissCount := 0

	// 在方法退出的时候发送事件通知
	defer func() {
		e.Fork().AddAction(events.NewAction(ActionUnlockFinish).AddPayload(PayloadVersionMissCount, versionMissCount)).Publish(ctx)
	}()

	for {

		// 尝试释放锁
		err := x.tryUnlock(ctx, e.Fork(), lockId, ownerId)
		if err == nil {
			e.Fork().AddAction(events.NewAction(ActionUnlockSuccess).AddPayload(PayloadVersionMissCount, versionMissCount)).Publish(ctx)
			return nil
		}

		// 只有在版本miss的情况下才会重试，如果不是版本miss的错误的话就不再重试了
		if !errors.Is(err, ErrVersionMiss) {
			e.Fork().AddAction(events.NewAction(ActionUnlockError).SetErr(err).AddPayload(PayloadVersionMissCount, versionMissCount)).Publish(ctx)
			return err
		}
		versionMissCount++
		e.Fork().AddAction(events.NewAction(ActionUnlockVersionMiss).AddPayload(PayloadVersionMissCount, versionMissCount)).Publish(ctx)

		// 休眠一会儿再开始重试
		sleepDuration := x.options.VersionMissRetryInterval + x.retryIntervalRandomBase()
		e.Fork().AddAction(events.NewAction(ActionSleep).AddPayload(PayloadSleep, sleepDuration)).Publish(ctx)
		time.Sleep(sleepDuration)

		select {
		case <-ctx.Done():
			e.Fork().AddAction(events.NewAction(ActionTimeout).AddPayload(PayloadVersionMissCount, versionMissCount)).Publish(ctx)
			return err
		case <-time.After(time.Microsecond):
			e.Fork().AddAction(events.NewAction(ActionSleepRetry).AddPayload(PayloadVersionMissCount, versionMissCount)).Publish(ctx)
			continue
		}
	}
}

// tryUnlock 尝试释放掉锁
func (x *StorageLock) tryUnlock(ctx context.Context, e *events.Event, lockId, ownerId string) error {

	// 设置事件的上下文并发送开始事件
	e.SetLockId(lockId).SetOwnerId(ownerId).AddActionByName(ActionUnlock).Publish(ctx)

	// 尝试读取锁的信息
	lockInformation, err := x.getLockInformation(ctx, e.Fork(), lockId)
	e.SetLockInformation(lockInformation)

	// 如果锁的信息都读取失败了，则没必要继续下去，这里没必要区分是锁不存在的错误还是其它错误，反正只要是错误就直接中断返回
	if err != nil {
		e.Fork().AddAction(events.NewAction(ActionGetLockInformationError).SetErr(err)).Publish(ctx)
		return err
	}

	// 如果读取到的锁的信息为空，则说明锁不存在，一个不存在的锁自然也没有继续的必要
	if lockInformation == nil {
		e.Fork().AddActionByName("lock-information-not-exists").Publish(ctx)
		return ErrLockNotFound
	}

	// 如果锁的当前持有者的ID不是自己，则无权释放锁
	if lockInformation.OwnerId != ownerId {
		e.Fork().AddActionByName(ActionNotLockOwner).Publish(ctx)
		return ErrLockNotBelongYou
	}

	// 通过了前面的检查，确实锁是自己持有的，则开始对锁进行操作
	lastVersion := lockInformation.Version
	lockInformation.Version++
	lockInformation.LockCount--

	// 如果释放一次之后发现还没有释放干净，说明是重入锁，并且加锁次数还没有为0，则尝试更新锁的信息
	if lockInformation.LockCount > 0 {
		e.Fork().AddActionByName(ActionUnlockReentry).Publish(ctx)
		return x.unlockReentry(ctx, e.Fork(), lockId, ownerId, lockInformation, lastVersion)
	} else {
		// 如果经过这次操作之后锁的锁的锁定次数为0，说明应该彻底释放掉这个锁了，将其从Storage中清除
		e.Fork().AddActionByName(ActionUnlockRelease).Publish(ctx)
		return x.unlockRelease(ctx, e.Fork(), lockId, ownerId, lockInformation, lastVersion)
	}
}

// 可重入锁的层级减一，但是并没有彻底释放，更新数据库中的锁的信息
func (x *StorageLock) unlockReentry(ctx context.Context, e *events.Event, lockId, ownerId string, lockInformation *storage.LockInformation, lastVersion storage.Version) error {

	// 设置事件的上下文
	e.SetLockInformation(lockInformation).SetOwnerId(ownerId)

	// 先发送一个unlock reentry的事件
	unlockReentryAction := events.NewAction(ActionUnlockReentry).
		AddPayload(storage_events.PayloadLockId, lockId).
		AddPayload(storage_events.PayloadLockInformation, lockInformation).
		AddPayload(PayloadLastVersion, lastVersion)
	e.Fork().AddAction(unlockReentryAction).Publish(ctx)

	// 获取锁的过期时间
	expireTime, err := x.getLeaseExpireTime(ctx, e.Fork())
	if err != nil {
		e.Fork().AddAction(events.NewAction(ActionGetLeaseExpireTimeError).SetErr(err)).Publish(ctx)
		return err
	}
	lockInformation.LeaseExpireTime = expireTime

	err = x.storageExecutor.UpdateWithVersion(ctx, e.Fork(), lockId, lastVersion, lockInformation.Version, lockInformation)
	// 更新成功，直接返回，说明锁释放成功了
	if err == nil {
		e.Fork().AddActionByName(ActionUnlockSuccess).Publish(ctx)
		return nil
	}

	if err != nil && !errors.Is(err, ErrVersionMiss) {
		e.Fork().AddAction(events.NewAction(ActionUnlockError).SetErr(err)).Publish(ctx)
		return err
	}
	e.Fork().AddAction(events.NewAction(storage_events.ActionStorageUpdateWithVersionSuccess)).Publish(ctx)
	return nil
}

// 锁被彻底释放干净了，将其标记为已经释放，以方便下一个到来的人能够重新拿到它
// ctx: 可以用作一些超时控制之类的
// e: 事件流推送
// lockId: 解锁的是哪个锁
// ownerId: 当前是谁在尝试释放锁
// lockInformation: 注意这个参数传进来的时候已经被修改过了，所以这里只需要将其更新就可以了不用再操作版本号啥的
// lastVersion: CAS时期望的锁的最新版本，如果不是的话会修改失败
func (x *StorageLock) unlockRelease(ctx context.Context, e *events.Event, lockId, ownerId string, lockInformation *storage.LockInformation, lastVersion storage.Version) error {

	// 更新事件的上下文
	e.SetLockInformation(lockInformation).SetLockId(lockId).SetOwnerId(ownerId)

	// 触发一个unlock release的事件先
	unlockReleaseAction := events.NewAction(ActionUnlockRelease).
		AddPayload(PayloadLastVersion, lastVersion).
		AddPayload(storage_events.PayloadLockInformation, lockInformation)
	e.AddAction(unlockReleaseAction).Publish(ctx)

	err := x.storageExecutor.UpdateWithVersion(ctx, e, lockId, lastVersion, lockInformation.Version, lockInformation)
	if err != nil {
		if errors.Is(err, ErrVersionMiss) {
			e.Fork().AddAction(events.NewAction(storage_events.ActionStorageUpdateWithVersionMiss).SetErr(err)).Publish(ctx)
		} else {
			e.Fork().AddAction(events.NewAction(storage_events.ActionStorageUpdateWithVersionError).SetErr(err)).Publish(ctx)
		}
		return err
	}
	e.Fork().AddAction(events.NewAction(storage_events.ActionStorageUpdateWithVersionSuccess)).Publish(ctx)

	// 把看门狗协程也停止掉，不要再尝试续租了
	if x.storageLockWatchDog != nil {
		stopLastWatchDogEvent := e.Fork().SetLockInformation(lockInformation).AddActionByName(ActionWatchDogStop).SetWatchDogId(x.storageLockWatchDog.GetID())
		err := x.storageLockWatchDog.Stop(ctx)
		// 把指针清空，防止后续被重复设置为nil
		x.storageLockWatchDog = nil
		if err != nil {
			stopLastWatchDogEvent.AddAction(events.NewAction(ActionWatchDogStopError).SetErr(err))
		} else {
			stopLastWatchDogEvent.AddAction(events.NewAction(ActionWatchDogStopSuccess))
		}
		stopLastWatchDogEvent.Publish(ctx)
	}

	return nil
}
