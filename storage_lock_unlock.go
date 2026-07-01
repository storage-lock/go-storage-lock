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
// ownerId: 是谁在尝试释放锁，必须与 Lock 时使用的 ownerId 相同，否则会返回 ErrLockNotBelongYou
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
		default:
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
		e.Fork().AddActionByName(ActionLockNotExists).Publish(ctx)
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
		return x.unlockReentry(ctx, e.Fork(), lockId, ownerId, lockInformation, lastVersion)
	} else {
		// 如果经过这次操作之后锁的锁的锁定次数为0，说明应该彻底释放掉这个锁了，将其从Storage中清除
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
	if err != nil {
		if errors.Is(err, ErrVersionMiss) {
			e.Fork().AddAction(events.NewAction(storage_events.ActionStorageUpdateWithVersionMiss).SetErr(err)).Publish(ctx)
			return ErrVersionMiss
		} else {
			e.Fork().AddAction(events.NewAction(storage_events.ActionStorageUpdateWithVersionError).SetErr(err)).Publish(ctx)
			return err
		}
	} else {
		e.Fork().AddActionByName(storage_events.ActionStorageUpdateWithVersionSuccess).Publish(ctx)
		return nil
	}
}

// 锁被彻底释放干净了，将其从Storage中删除，这样下一个获取锁的人就能走 CreateWithVersion 路径
// ctx: 可以用作一些超时控制之类的
// e: 事件流推送
// lockId: 解锁的是哪个锁
// ownerId: 当前是谁在尝试释放锁
// lockInformation: 锁的信息，用于传递给 DeleteWithVersion 作为条件校验
// lastVersion: CAS时期望的锁的最新版本，如果不是的话会删除失败
func (x *StorageLock) unlockRelease(ctx context.Context, e *events.Event, lockId, ownerId string, lockInformation *storage.LockInformation, lastVersion storage.Version) error {

	// 更新事件的上下文
	e.SetLockInformation(lockInformation).SetLockId(lockId).SetOwnerId(ownerId)

	// 触发一个unlock release的事件先
	unlockReleaseAction := events.NewAction(ActionUnlockRelease).
		AddPayload(PayloadLastVersion, lastVersion).
		AddPayload(storage_events.PayloadLockInformation, lockInformation)
	e.AddAction(unlockReleaseAction).Publish(ctx)

	// 先停止看门狗，再删除锁记录
	// 这非常重要：如果先 DeleteWithVersion 再 stopWatchDog，那么在 DeleteWithVersion 因版本 miss 重试期间，
	// 看门狗仍在持续续租推进版本号，与 UnLock 形成竞争，导致 UnLock 无限重试（活锁）。
	// 先停看门狗可让版本号稳定下来，DeleteWithVersion 才能成功。
	x.stopWatchDog(ctx, e, lockInformation)

	// 释放路径分两种：
	//   - 存储支持原子条件删除（CapabilityAtomicDelete）：用 DeleteWithVersion 真正删除记录，
	//     下次获取锁走 lockNotExists → CreateWithVersion，语义最清晰
	//   - 存储不支持原子条件删除（如对象存储只有条件 PUT 没有条件 DELETE）：
	//     降级为 UpdateWithVersion 写入"墓碑"标记（LockCount=0），记录保留但逻辑上已释放，
	//     下次获取锁走 lockExists → 识别 LockCount==0 → lockExpired 抢占路径。
	//     互斥性不受影响——墓碑写入本身就是一次 UpdateWithVersion 的原子 CAS。
	if storage.SupportsAtomicDelete(x.storage) {
		return x.unlockReleaseByDelete(ctx, e, lockId, ownerId, lockInformation, lastVersion)
	}
	return x.unlockReleaseByTombstone(ctx, e, lockId, ownerId, lockInformation, lastVersion)
}

// unlockReleaseByDelete 通过 DeleteWithVersion 真正删除锁记录（存储支持原子条件删除时）
func (x *StorageLock) unlockReleaseByDelete(ctx context.Context, e *events.Event, lockId, ownerId string, lockInformation *storage.LockInformation, lastVersion storage.Version) error {
	err := x.storageExecutor.DeleteWithVersion(ctx, e, lockId, lastVersion, lockInformation)
	if err != nil {
		if errors.Is(err, ErrVersionMiss) {
			e.Fork().AddAction(events.NewAction(storage_events.ActionStorageDeleteWithVersionMiss).SetErr(err)).Publish(ctx)
			return ErrVersionMiss
		} else {
			e.Fork().AddAction(events.NewAction(storage_events.ActionStorageDeleteWithVersionError).SetErr(err)).Publish(ctx)
			return err
		}
	}
	e.Fork().AddAction(events.NewAction(storage_events.ActionStorageDeleteWithVersionSuccess)).Publish(ctx)
	return nil
}

// unlockReleaseByTombstone 通过 UpdateWithVersion 写入墓碑标记释放锁（存储不支持原子条件删除时）
// 墓碑 = LockCount 置 0、LeaseExpireTime 设为当前时间（立即失效），记录保留但逻辑已释放。
// 下次获取锁时 lockExists 识别 LockCount==0 走抢占路径。
func (x *StorageLock) unlockReleaseByTombstone(ctx context.Context, e *events.Event, lockId, ownerId string, lockInformation *storage.LockInformation, lastVersion storage.Version) error {
	// 构造墓碑：版本号 +1，LockCount=0，租约立即过期
	lockInformation.Version = lastVersion + 1
	lockInformation.LockCount = 0
	now, err := x.getTime(ctx, e.Fork())
	if err != nil {
		e.Fork().AddAction(events.NewAction(storage_events.ActionStorageGetTimeError).SetErr(err)).Publish(ctx)
		return err
	}
	lockInformation.LeaseExpireTime = now // 立即过期，双保险（主要靠 LockCount==0 识别）

	err = x.storageExecutor.UpdateWithVersion(ctx, e.Fork(), lockId, lastVersion, lockInformation.Version, lockInformation)
	if err != nil {
		if errors.Is(err, ErrVersionMiss) {
			e.Fork().AddAction(events.NewAction(storage_events.ActionStorageUpdateWithVersionMiss).SetErr(err)).Publish(ctx)
			return ErrVersionMiss
		} else {
			e.Fork().AddAction(events.NewAction(storage_events.ActionStorageUpdateWithVersionError).SetErr(err)).Publish(ctx)
			return err
		}
	}
	e.Fork().AddActionByName(storage_events.ActionStorageUpdateWithVersionSuccess).Publish(ctx)
	return nil
}
