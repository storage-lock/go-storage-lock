package storage_lock

import (
	"context"
	"errors"
	"time"

	"github.com/storage-lock/go-events"
	"github.com/storage-lock/go-storage"
	storage_events "github.com/storage-lock/go-storage-events"
)

// RLock 以共享读锁模式获取锁。
// 多个不同的 ownerId 可以同时持有同一把读锁；写锁与读锁互斥。
func (x *StorageLock) RLock(ctx context.Context, ownerId string) error {
	if err := validateOwner(ctx, ownerId); err != nil {
		return err
	}
	lockId := x.options.LockId
	e := events.NewEvent(lockId).SetOwnerId(ownerId).SetType(events.EventTypeLock).SetListeners(x.options.EventListeners).SetStorageName(x.storage.GetName())
	e.AddActionByName(ActionRLockBegin).Publish(ctx)

	for {
		err := x.tryRLock(ctx, e.Fork(), lockId, ownerId)
		if err == nil {
			e.Fork().AddActionByName(ActionRLockSuccess).Publish(ctx)
			return nil
		}
		if errors.Is(err, ErrVersionMiss) || errors.Is(err, ErrLockAlreadyExists) {
			e.Fork().AddAction(events.NewAction(ActionRLockVersionMiss)).Publish(ctx)
		} else if errors.Is(err, ErrLockBusy) {
			e.Fork().AddAction(events.NewAction(ActionRLockBusy)).Publish(ctx)
		} else {
			e.Fork().AddAction(events.NewAction(ActionRLockError).SetErr(err)).Publish(ctx)
			return err
		}
		sleepDuration := x.options.VersionMissRetryInterval + x.retryIntervalRandomBase()
		e.Fork().AddAction(events.NewAction(ActionSleep).AddPayload(PayloadSleep, sleepDuration)).Publish(ctx)
		if err := waitContext(ctx, sleepDuration); err != nil {
			e.Fork().AddAction(events.NewAction(ActionTimeout)).Publish(ctx)
			return err
		}
		e.Fork().AddAction(events.NewAction(ActionSleepRetry)).Publish(ctx)
	}
}

func (x *StorageLock) tryRLock(ctx context.Context, e *events.Event, lockId, ownerId string) error {
	lockInformation, err := x.getLockInformation(ctx, e, lockId)
	if err != nil && !errors.Is(err, ErrLockNotFound) {
		e.Fork().AddAction(events.NewAction(ActionGetLockInformationError).SetErr(err)).Publish(ctx)
		return err
	}

	if lockInformation == nil {
		return x.rlockCreate(ctx, e, lockId, ownerId)
	}

	if storageTime, err := x.getTime(ctx, e.Fork()); err != nil {
		return err
	} else if storageTime.After(lockInformation.LeaseExpireTime) || lockInformation.LockCount == 0 {
		// 记录本身还存在（过期或者墓碑），只是逻辑上已失效，必须用 UpdateWithVersion 抢占，
		// 不能走 rlockCreate 的 CreateWithVersion：对不支持原子删除的存储（如 MongoDB/S3
		// 等墓碑释放路径），记录一直都在，CreateWithVersion 只会不断返回
		// ErrLockAlreadyExists，而 rlockCreate 对该错误的处理是回退到 tryRLock 重新走一遍
		// 同样的判断，形成死循环（RLock 永远卡在 sleep-retry，参照写锁 lockExpired 的处理方式）。
		return x.rlockReclaim(ctx, e, lockId, ownerId, storageTime, lockInformation)
	}

	if lockInformation.ReadLock {
		// 已有写者排队时，新读者不再进入，避免写饥饿。已经在读的 owner 仍可重入。
		if lockInformation.WriteIntentOwner != "" && lockInformation.ReadDepth(ownerId) == 0 {
			return ErrUpgradeConflict
		}
		return x.rlockReentry(ctx, e, lockId, ownerId, lockInformation)
	}

	return ErrLockBusy
}

// rlockReclaim 抢占一个已经存在但逻辑上已失效的记录（租约过期，或墓碑 LockCount==0）。
// 与写锁的 lockExpired 对称：记录还在存储里，必须用 UpdateWithVersion 而不是
// CreateWithVersion 去覆盖它。
func (x *StorageLock) rlockReclaim(ctx context.Context, e *events.Event, lockId, ownerId string, storageTime time.Time, oldLockInformation *storage.LockInformation) error {
	// 已有写者排队时，读者不能抢先把队列清掉。
	if firstWriteWaiter(oldLockInformation.Waiters) != nil {
		return ErrNotYourTurn
	}
	newLockInformation := &storage.LockInformation{
		LockId:          lockId,
		OwnerId:         ownerId,
		Version:         0,
		LockCount:       1,
		LockBeginTime:   storageTime,
		LeaseExpireTime: storageTime.Add(x.options.LeaseExpireAfter),
		ReadLock:        true,
		ReadOwners:      map[string]int{ownerId: 1},
	}
	var err error
	newLockInformation.Version, err = nextVersion(oldLockInformation.Version)
	if err != nil {
		return err
	}
	err = x.storageExecutor.UpdateWithVersion(ctx, e.Fork(), lockId, oldLockInformation.Version, newLockInformation.Version, newLockInformation)
	if err != nil {
		if errors.Is(err, ErrVersionMiss) {
			e.Fork().AddActionByName(storage_events.ActionStorageUpdateWithVersionMiss).Publish(ctx)
			return ErrVersionMiss
		}
		e.Fork().AddAction(events.NewAction(storage_events.ActionStorageUpdateWithVersionError).SetErr(err)).Publish(ctx)
		return err
	}
	e.Fork().AddActionByName(storage_events.ActionStorageUpdateWithVersionSuccess).Publish(ctx)
	x.notifySubscribers(lockId, newLockInformation)

	// 抢占相当于新持有，防御性地清理一下可能残留的看门狗后再启动新的
	x.stopWatchDog(ctx, e.Fork(), newLockInformation)
	return x.startWatchDog(ctx, e.Fork(), lockId, ownerId, newLockInformation)
}

func (x *StorageLock) rlockCreate(ctx context.Context, e *events.Event, lockId, ownerId string) error {
	storageTime, err := x.getTime(ctx, e.Fork())
	if err != nil {
		e.Fork().AddAction(events.NewAction(storage_events.ActionStorageGetTimeError).SetErr(err)).Publish(ctx)
		return err
	}
	expireTime := storageTime.Add(x.options.LeaseExpireAfter)

	newLockInformation := &storage.LockInformation{
		LockId:          lockId,
		OwnerId:         ownerId,
		Version:         1,
		LockCount:       1,
		LockBeginTime:   storageTime,
		LeaseExpireTime: expireTime,
		ReadLock:        true,
		ReadOwners:      map[string]int{ownerId: 1},
	}
	err = x.storageExecutor.CreateWithVersion(ctx, e.Fork(), lockId, newLockInformation.Version, newLockInformation)
	if err != nil {
		if errors.Is(err, ErrVersionMiss) {
			e.Fork().AddAction(events.NewAction(storage_events.ActionStorageCreateWithVersionMiss)).Publish(ctx)
			return ErrVersionMiss
		}
		if errors.Is(err, ErrLockAlreadyExists) {
			// 并发创建读锁时，另一个 reader 已先创建；回退到尝试加入已有读锁
			return x.tryRLock(ctx, e, lockId, ownerId)
		}
		e.Fork().AddAction(events.NewAction(storage_events.ActionStorageCreateWithVersionError).SetErr(err)).Publish(ctx)
		return err
	}
	e.Fork().AddActionByName(storage_events.ActionStorageCreateWithVersionSuccess).Publish(ctx)
	x.notifySubscribers(lockId, newLockInformation)
	return x.startWatchDog(ctx, e.Fork(), lockId, ownerId, newLockInformation)
}

func (x *StorageLock) rlockReentry(ctx context.Context, e *events.Event, lockId, ownerId string, lockInformation *storage.LockInformation) error {
	if x.options.MaxReentrantDepth > 0 && lockInformation.ReadDepth(ownerId) >= x.options.MaxReentrantDepth {
		return ErrMaxReentrantDepth
	}
	oldVersion := lockInformation.Version
	newVersion, err := nextVersion(oldVersion)
	if err != nil {
		return err
	}
	lockInformation.Version = newVersion
	lockInformation.AddReadOwner(ownerId)
	lockInformation.Waiters = removeWaiter(lockInformation.Waiters, ownerId, waiterModeRead)

	expireTime, err := x.getLeaseExpireTime(ctx, e.Fork())
	if err != nil {
		e.Fork().AddAction(events.NewAction(ActionGetLeaseExpireTimeError).SetErr(err)).Publish(ctx)
		return err
	}
	lockInformation.LeaseExpireTime = expireTime

	err = x.storageExecutor.UpdateWithVersion(ctx, e.Fork(), lockId, oldVersion, lockInformation.Version, lockInformation)
	if err != nil {
		if errors.Is(err, ErrVersionMiss) {
			e.Fork().AddActionByName(storage_events.ActionStorageUpdateWithVersionMiss).Publish(ctx)
			return ErrVersionMiss
		}
		e.Fork().AddAction(events.NewAction(storage_events.ActionStorageUpdateWithVersionError).SetErr(err)).Publish(ctx)
		return err
	}
	e.Fork().AddActionByName(storage_events.ActionStorageUpdateWithVersionSuccess).Publish(ctx)
	x.notifySubscribers(lockId, lockInformation)
	return nil
}

// RUnLock 释放共享读锁。
func (x *StorageLock) RUnLock(ctx context.Context, ownerId string) error {
	lockId := x.options.LockId
	e := events.NewEvent(lockId).SetOwnerId(ownerId).SetType(events.EventTypeUnlock).SetListeners(x.options.EventListeners).SetStorageName(x.storage.GetName())
	e.AddActionByName(ActionRUnlockBegin).Publish(ctx)

	for {
		err := x.tryRUnlock(ctx, e.Fork(), lockId, ownerId)
		if err == nil {
			e.Fork().AddActionByName(ActionRUnlockSuccess).Publish(ctx)
			return nil
		}
		if errors.Is(err, ErrVersionMiss) {
			e.Fork().AddAction(events.NewAction(ActionRUnlockVersionMiss)).Publish(ctx)
		} else {
			e.Fork().AddAction(events.NewAction(ActionRUnlockError).SetErr(err)).Publish(ctx)
			return err
		}
		sleepDuration := x.options.VersionMissRetryInterval + x.retryIntervalRandomBase()
		e.Fork().AddAction(events.NewAction(ActionSleep).AddPayload(PayloadSleep, sleepDuration)).Publish(ctx)
		if err := waitContext(ctx, sleepDuration); err != nil {
			e.Fork().AddAction(events.NewAction(ActionTimeout)).Publish(ctx)
			return err
		}
		e.Fork().AddAction(events.NewAction(ActionSleepRetry)).Publish(ctx)
	}
}

func (x *StorageLock) tryRUnlock(ctx context.Context, e *events.Event, lockId, ownerId string) error {
	lockInformation, err := x.getLockInformation(ctx, e, lockId)
	if err != nil {
		e.Fork().AddAction(events.NewAction(ActionGetLockInformationError).SetErr(err)).Publish(ctx)
		return err
	}
	if lockInformation == nil {
		e.Fork().AddActionByName(ActionLockNotExists).Publish(ctx)
		return ErrLockNotFound
	}
	if !lockInformation.ReadLock {
		e.Fork().AddActionByName(ActionNotLockOwner).Publish(ctx)
		return ErrLockNotBelongYou
	}
	if ownerId == "" {
		return ErrOwnerIdEmpty
	}
	if !lockInformation.RemoveReadOwner(ownerId) {
		e.Fork().AddActionByName(ActionNotLockOwner).Publish(ctx)
		return ErrLockNotBelongYou
	}
	lastVersion := lockInformation.Version
	newVersion, err := nextVersion(lastVersion)
	if err != nil {
		return err
	}
	lockInformation.Version = newVersion

	if lockInformation.LockCount > 0 {
		return x.runlockReentry(ctx, e.Fork(), lockId, ownerId, lockInformation, lastVersion)
	}
	return x.runlockRelease(ctx, e.Fork(), lockId, ownerId, lockInformation, lastVersion)
}

func (x *StorageLock) runlockReentry(ctx context.Context, e *events.Event, lockId, ownerId string, lockInformation *storage.LockInformation, lastVersion storage.Version) error {
	expireTime, err := x.getLeaseExpireTime(ctx, e.Fork())
	if err != nil {
		e.Fork().AddAction(events.NewAction(ActionGetLeaseExpireTimeError).SetErr(err)).Publish(ctx)
		return err
	}
	lockInformation.LeaseExpireTime = expireTime
	err = x.storageExecutor.UpdateWithVersion(ctx, e.Fork(), lockId, lastVersion, lockInformation.Version, lockInformation)
	if err != nil {
		if errors.Is(err, ErrVersionMiss) {
			e.Fork().AddActionByName(storage_events.ActionStorageUpdateWithVersionMiss).Publish(ctx)
			return ErrVersionMiss
		}
		e.Fork().AddAction(events.NewAction(storage_events.ActionStorageUpdateWithVersionError).SetErr(err)).Publish(ctx)
		return err
	}
	e.Fork().AddActionByName(storage_events.ActionStorageUpdateWithVersionSuccess).Publish(ctx)
	x.notifySubscribers(lockId, lockInformation)
	return nil
}

func (x *StorageLock) runlockRelease(ctx context.Context, e *events.Event, lockId, ownerId string, lockInformation *storage.LockInformation, lastVersion storage.Version) error {
	e.SetLockInformation(lockInformation).AddActionByName(ActionRUnlockRelease).Publish(ctx)
	x.stopWatchDog(ctx, e, lockInformation)

	tombstone := &storage.LockInformation{
		LockId:          lockInformation.LockId,
		Version:         0,
		LockCount:       0,
		LockBeginTime:   lockInformation.LockBeginTime,
		LeaseExpireTime: lockInformation.LeaseExpireTime,
		ReadLock:        lockInformation.ReadLock,
	}
	var err error
	tombstone.Version, err = nextVersion(lastVersion)
	if err != nil {
		return err
	}
	err = x.storageExecutor.UpdateWithVersion(ctx, e.Fork(), lockId, lastVersion, tombstone.Version, tombstone)
	if err != nil {
		if errors.Is(err, ErrVersionMiss) {
			e.Fork().AddActionByName(storage_events.ActionStorageUpdateWithVersionMiss).Publish(ctx)
			return ErrVersionMiss
		}
		e.Fork().AddAction(events.NewAction(storage_events.ActionStorageUpdateWithVersionError).SetErr(err)).Publish(ctx)
		return err
	}
	e.Fork().AddActionByName(storage_events.ActionStorageUpdateWithVersionSuccess).Publish(ctx)
	x.notifySubscribers(lockId, tombstone)
	return nil
}

// WLock 以独占写锁模式获取锁。
// 写锁与任何其他写锁/读锁互斥。已有读者时先登记写意图，阻止新读者进入。
func (x *StorageLock) WLock(ctx context.Context, ownerId string) error {
	if err := validateOwner(ctx, ownerId); err != nil {
		return err
	}
	if err := x.prepareWrite(ctx, ownerId); err != nil {
		return err
	}
	err := x.Lock(ctx, ownerId)
	if err != nil {
		x.abandonWait(ownerId, waiterModeWrite)
	}
	return err
}

// WUnLock 释放独占写锁。
func (x *StorageLock) WUnLock(ctx context.Context, ownerId string) error {
	return x.UnLock(ctx, ownerId)
}

// TryUpgrade 把 owner 持有的一层读锁升级为独占写锁。
// 只有该 owner 是唯一读者时才能成功；还有其他读者时返回 ErrLockBusy，读锁保持原样。
// 升级只尝试一次，不会等待，因此不会把自己等待自己的读锁释放而卡死。
func (x *StorageLock) TryUpgrade(ctx context.Context, ownerId string) error {
	if err := validateOwner(ctx, ownerId); err != nil {
		return err
	}
	lockId := x.options.LockId
	e := x.operationEvent(ownerId).SetType(events.EventTypeLock)
	e.AddActionByName(ActionUpgradeBegin).Publish(ctx)
	err := x.tryUpgrade(ctx, e.Fork(), lockId, ownerId)
	if errors.Is(err, ErrVersionMiss) {
		err = ErrLockBusy
	}
	if err != nil {
		e.Fork().AddAction(events.NewAction(ActionUpgradeError).SetErr(err)).Publish(ctx)
		return err
	}
	e.Fork().AddActionByName(ActionUpgradeSuccess).Publish(ctx)
	return nil
}

func (x *StorageLock) tryUpgrade(ctx context.Context, e *events.Event, lockId, ownerId string) error {
	info, err := x.getLockInformation(ctx, e, lockId)
	if err != nil {
		return err
	}
	now, err := x.getTime(ctx, e.Fork())
	if err != nil {
		return err
	}
	if info.LockCount <= 0 || !now.Before(info.LeaseExpireTime) {
		return ErrLeaseExpired
	}
	if !info.ReadLock || info.ReadDepth(ownerId) <= 0 {
		return ErrLockNotBelongYou
	}
	// 还有别的读者，升级会剥夺他们的持有权，必须失败并保持读锁。
	if info.ReaderCount() != 1 || info.ReadDepth(ownerId) != info.LockCount {
		return ErrUpgradeConflict
	}
	oldVersion := info.Version
	next, err := nextVersion(oldVersion)
	if err != nil {
		return err
	}
	upgraded := &storage.LockInformation{
		LockId:          lockId,
		OwnerId:         ownerId,
		Version:         next,
		LockCount:       1,
		LockBeginTime:   info.LockBeginTime,
		LeaseExpireTime: now.Add(x.options.LeaseExpireAfter),
	}
	if err := x.storageExecutor.UpdateWithVersion(ctx, e.Fork(), lockId, oldVersion, next, upgraded); err != nil {
		return err
	}
	x.notifySubscribers(lockId, upgraded)
	return nil
}

// Downgrade 把 owner 持有的一层独占写锁降为共享读锁，让其他读者可以进入。
// 写锁重入深度大于 1 时只减少一层，仍保持独占。不是当前写者时返回 ErrLockNotBelongYou。
func (x *StorageLock) Downgrade(ctx context.Context, ownerId string) error {
	if err := validateOwner(ctx, ownerId); err != nil {
		return err
	}
	e := x.operationEvent(ownerId).SetType(events.EventTypeUnlock)
	for {
		err := x.downgradeOnce(ctx, e.Fork(), ownerId)
		if !errors.Is(err, ErrVersionMiss) {
			return err
		}
		if err := waitContext(ctx, x.options.VersionMissRetryInterval+x.retryIntervalRandomBase()); err != nil {
			return err
		}
	}
}

func (x *StorageLock) downgradeOnce(ctx context.Context, e *events.Event, ownerId string) error {
	info, err := x.getLockInformation(ctx, e, x.options.LockId)
	if err != nil {
		return err
	}
	expires, err := x.activeLeaseExpiry(ctx, e.Fork(), info)
	if err != nil {
		return err
	}
	if info.ReadLock || info.OwnerId != ownerId || info.LockCount <= 0 {
		return ErrLockNotBelongYou
	}
	oldVersion := info.Version
	next, err := nextVersion(oldVersion)
	if err != nil {
		return err
	}
	info.Version = next
	info.LeaseExpireTime = expires
	info.LockCount--
	if info.LockCount == 0 {
		info.ReadLock = true
		info.LockCount = 1
		info.ReadOwners = map[string]int{ownerId: 1}
		info.WriteIntentOwner = ""
		info.WriteIntentSince = time.Time{}
	}
	if err := x.storageExecutor.UpdateWithVersion(ctx, e.Fork(), x.options.LockId, oldVersion, next, info); err != nil {
		return err
	}
	x.notifySubscribers(x.options.LockId, info)
	return nil
}

// prepareWrite 在独占获取前登记写意图。锁被读者持有时，这个登记阻止后续新读者进入。
func (x *StorageLock) prepareWrite(ctx context.Context, ownerId string) error {
	e := x.operationEvent(ownerId)
	for {
		err := x.prepareWriteOnce(ctx, e.Fork(), ownerId)
		if errors.Is(err, ErrVersionMiss) {
			if waitErr := waitContext(ctx, x.options.VersionMissRetryInterval+x.retryIntervalRandomBase()); waitErr != nil {
				return waitErr
			}
			continue
		}
		return err
	}
}

func (x *StorageLock) prepareWriteOnce(ctx context.Context, e *events.Event, ownerId string) error {
	info, err := x.getLockInformation(ctx, e, x.options.LockId)
	if errors.Is(err, ErrLockNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	now, err := x.getTime(ctx, e.Fork())
	if err != nil {
		return err
	}
	active := info.LockCount > 0 && now.Before(info.LeaseExpireTime)
	// 不排队时，空闲或过期的记录没有人需要让位，登记没有意义。
	// 排队时必须在空闲记录上也留下自己的位置，否则释放和接手之间谁都能插进来。
	if !active && !x.options.Fair {
		return nil
	}
	// 自己持有的独占锁直接重入，不需要排队。
	if active && !info.ReadLock && info.OwnerId == ownerId {
		return nil
	}
	changed := false
	if !info.HasWaiter(ownerId, waiterModeWrite) {
		info.Waiters = append(info.Waiters, storage.LockWaiter{OwnerId: ownerId, Mode: waiterModeWrite, Since: now})
		changed = true
	}
	if info.ReadLock && info.WriteIntentOwner == "" {
		info.WriteIntentOwner = ownerId
		info.WriteIntentSince = now
		changed = true
	}
	if !changed {
		return nil
	}
	oldVersion := info.Version
	next, err := nextVersion(oldVersion)
	if err != nil {
		return err
	}
	info.Version = next
	return x.storageExecutor.UpdateWithVersion(ctx, e.Fork(), x.options.LockId, oldVersion, next, info)
}

// abandonWait 在获取失败或取消时移除自己的等待登记和写意图。清理失败不影响调用方拿到的错误。
func (x *StorageLock) abandonWait(ownerId, mode string) {
	ctx, cancel := x.cleanupContext(context.Background())
	defer cancel()
	e := x.operationEvent(ownerId)
	for attempt := 0; attempt < 3; attempt++ {
		info, err := x.getLockInformation(ctx, e, x.options.LockId)
		if err != nil {
			return
		}
		waiters := removeWaiter(info.Waiters, ownerId, mode)
		intentOwner := info.WriteIntentOwner
		intentSince := info.WriteIntentSince
		if mode == waiterModeWrite && info.WriteIntentOwner == ownerId {
			intentOwner, intentSince = "", time.Time{}
			for _, waiter := range waiters {
				if waiter.Mode == waiterModeWrite {
					intentOwner = waiter.OwnerId
					intentSince = waiter.Since
					break
				}
			}
		}
		if len(waiters) == len(info.Waiters) && intentOwner == info.WriteIntentOwner {
			return
		}
		oldVersion := info.Version
		next, err := nextVersion(oldVersion)
		if err != nil {
			return
		}
		info.Version = next
		info.Waiters = waiters
		info.WriteIntentOwner = intentOwner
		info.WriteIntentSince = intentSince
		if err := x.storageExecutor.UpdateWithVersion(ctx, e.Fork(), x.options.LockId, oldVersion, next, info); err == nil {
			x.notifySubscribers(x.options.LockId, info)
			x.publishWaitChanged(ctx, ownerId)
			return
		}
	}
}

// ensureFairQueued 保证 owner 已经写进存储里的等待队列，然后才返回。
// 记录不存在时先创建一条只含队列的记录；别人抢先创建时重试并排到那条记录后面。
// 自己已经持有有效的独占锁时不用排队，直接返回。
func (x *StorageLock) ensureFairQueued(ctx context.Context, ownerId string) error {
	e := x.operationEvent(ownerId)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		info, err := x.getLockInformation(ctx, e.Fork(), x.options.LockId)
		if errors.Is(err, ErrLockNotFound) {
			now, timeErr := x.getTime(ctx, e.Fork())
			if timeErr != nil {
				return timeErr
			}
			placeholder := &storage.LockInformation{
				LockId:  x.options.LockId,
				Version: 1,
				Waiters: []storage.LockWaiter{{OwnerId: ownerId, Mode: waiterModeWrite, Since: now}},
			}
			err = x.storageExecutor.CreateWithVersion(ctx, e.Fork(), x.options.LockId, placeholder.Version, placeholder)
			if err == nil {
				x.notifySubscribers(x.options.LockId, placeholder)
				x.publishWaitChanged(ctx, ownerId)
				return nil
			}
			// 记录已经被别人创建出来了。LevelDB 这类存储把"已存在"报成版本冲突，
			// 和真正的版本不匹配是同一件事：重试，读到那条记录后排到队尾。
			if errors.Is(err, ErrLockAlreadyExists) || errors.Is(err, ErrVersionMiss) {
				continue
			}
			return err
		}
		if err != nil {
			return err
		}
		now, timeErr := x.getTime(ctx, e.Fork())
		if timeErr != nil {
			return timeErr
		}
		active := info.LockCount > 0 && now.Before(info.LeaseExpireTime)
		if active && !info.ReadLock && info.OwnerId == ownerId {
			return nil
		}
		if info.HasWaiter(ownerId, waiterModeWrite) {
			return nil
		}
		oldVersion := info.Version
		next, versionErr := nextVersion(oldVersion)
		if versionErr != nil {
			return versionErr
		}
		info.Version = next
		info.Waiters = append(info.Waiters, storage.LockWaiter{OwnerId: ownerId, Mode: waiterModeWrite, Since: now})
		err = x.storageExecutor.UpdateWithVersion(ctx, e.Fork(), x.options.LockId, oldVersion, next, info)
		if err == nil {
			x.notifySubscribers(x.options.LockId, info)
			x.publishWaitChanged(ctx, ownerId)
			return nil
		}
		if errors.Is(err, ErrVersionMiss) {
			if waitErr := waitContext(ctx, x.options.VersionMissRetryInterval+x.retryIntervalRandomBase()); waitErr != nil {
				return waitErr
			}
			continue
		}
		return err
	}
}

// firstWriteWaiter 返回队列里排在最前的写者。没有写者时返回 nil。
func firstWriteWaiter(waiters []storage.LockWaiter) *storage.LockWaiter {
	for i := range waiters {
		if waiters[i].Mode == waiterModeWrite {
			return &waiters[i]
		}
	}
	return nil
}

func removeWaiter(waiters []storage.LockWaiter, ownerId, mode string) []storage.LockWaiter {
	filtered := waiters[:0:0]
	for _, waiter := range waiters {
		if waiter.OwnerId == ownerId && waiter.Mode == mode {
			continue
		}
		filtered = append(filtered, waiter)
	}
	return filtered
}
