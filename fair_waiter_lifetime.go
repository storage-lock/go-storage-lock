package storage_lock

import (
	"context"
	"time"

	"github.com/storage-lock/go-events"
	"github.com/storage-lock/go-storage"
)

// waiterDeadlineWindow 是一次阻塞等待把排队登记往前续的窗口。
// 至少 1 秒，并且盖住四个重试间隔，避免一次重试还没回来登记就过期。
func waiterDeadlineWindow(retry time.Duration) time.Duration {
	window := retry * 4
	if window < time.Second {
		window = time.Second
	}
	return window
}

// waiterExpired 报告这条登记是否已过截止时间。零值截止时间永远视为还活着。
func waiterExpired(waiter storage.LockWaiter, now time.Time) bool {
	if waiter.Until.IsZero() {
		return false
	}
	return !now.Before(waiter.Until)
}

// pruneExpiredWaiters 按原顺序丢掉已经过期的登记。没有任何登记过期时 changed 为 false。
func pruneExpiredWaiters(waiters []storage.LockWaiter, now time.Time) (kept []storage.LockWaiter, changed bool) {
	kept = make([]storage.LockWaiter, 0, len(waiters))
	for _, waiter := range waiters {
		if waiterExpired(waiter, now) {
			changed = true
			continue
		}
		kept = append(kept, waiter)
	}
	if !changed {
		return waiters, false
	}
	return kept, true
}

// reconcileWriteIntent 在等待者被剔除之后，把写意图指到仍存活的下一位写者。
// 写意图本来就不指向被删掉的等待者时，保持原值。
func reconcileWriteIntent(before []storage.LockWaiter, intentOwner string, intentSince time.Time, kept []storage.LockWaiter) (string, time.Time) {
	if intentOwner == "" {
		return "", time.Time{}
	}
	listed := false
	for _, waiter := range before {
		if waiter.OwnerId == intentOwner {
			listed = true
			break
		}
	}
	if !listed {
		return intentOwner, intentSince
	}
	for _, waiter := range kept {
		if waiter.OwnerId == intentOwner {
			return intentOwner, intentSince
		}
	}
	for _, waiter := range kept {
		if waiter.Mode == waiterModeWrite {
			return waiter.OwnerId, waiter.Since
		}
	}
	return "", time.Time{}
}

// cloneLockInformation 复制一份记录再写入存储。
// 内存双端会保存指针；调用方随后还会改同一条记录，不能和存储里的那份共用。
func cloneLockInformation(info *storage.LockInformation) *storage.LockInformation {
	clone := *info
	if info.Waiters != nil {
		clone.Waiters = append([]storage.LockWaiter(nil), info.Waiters...)
	}
	if info.ReadOwners != nil {
		clone.ReadOwners = make(map[string]int, len(info.ReadOwners))
		for owner, depth := range info.ReadOwners {
			clone.ReadOwners[owner] = depth
		}
	}
	if info.Extra != nil {
		clone.Extra = make(map[string]string, len(info.Extra))
		for key, value := range info.Extra {
			clone.Extra[key] = value
		}
	}
	return &clone
}

// newWaiter 构造一条排队登记。只有公平获取才写截止时间，非公平登记保持旧记录的零值。
func (x *StorageLock) newWaiter(ownerId, mode string, now time.Time) storage.LockWaiter {
	waiter := storage.LockWaiter{OwnerId: ownerId, Mode: mode, Since: now}
	if x.options.Fair {
		waiter.Until = now.Add(waiterDeadlineWindow(x.options.VersionMissRetryInterval))
	}
	return waiter
}

// dropExpiredWaiters 在决定轮到谁之前，把过期登记从共享记录里删掉。
// now 为零时改读锁的时间源。没有过期登记时不写存储。
func (x *StorageLock) dropExpiredWaiters(ctx context.Context, e *events.Event, info *storage.LockInformation, now time.Time) error {
	if info == nil || len(info.Waiters) == 0 {
		return nil
	}
	var err error
	if now.IsZero() {
		now, err = x.getTime(ctx, e.Fork())
		if err != nil {
			return err
		}
	}
	kept, changed := pruneExpiredWaiters(info.Waiters, now)
	if !changed {
		return nil
	}
	previousWaiters := info.Waiters
	previousIntent := info.WriteIntentOwner
	previousSince := info.WriteIntentSince
	intent, since := reconcileWriteIntent(info.Waiters, info.WriteIntentOwner, info.WriteIntentSince, kept)
	oldVersion := info.Version
	next, err := nextVersion(oldVersion)
	if err != nil {
		return err
	}
	info.Version = next
	info.Waiters = kept
	info.WriteIntentOwner = intent
	info.WriteIntentSince = since
	stored := cloneLockInformation(info)
	if err := x.storageExecutor.UpdateWithVersion(ctx, e.Fork(), info.LockId, oldVersion, next, stored); err != nil {
		info.Version = oldVersion
		info.Waiters = previousWaiters
		info.WriteIntentOwner = previousIntent
		info.WriteIntentSince = previousSince
		return err
	}
	x.notifySubscribers(info.LockId, stored)
	x.publishWaitChanged(ctx, "")
	return nil
}

// renewWaiterRegistration 把调用方自己的公平排队登记续到当前时间源之后。
// 不在队列里、或截止时间已经不早于新的窗口时，不写存储，也不改变别人的顺序。
func (x *StorageLock) renewWaiterRegistration(ctx context.Context, ownerId, mode string) error {
	if !x.options.Fair {
		return nil
	}
	e := x.operationEvent(ownerId)
	now, err := x.getTime(ctx, e.Fork())
	if err != nil {
		return err
	}
	info, err := x.getLockInformation(ctx, e, x.options.LockId)
	if err != nil {
		return err
	}
	deadline := now.Add(waiterDeadlineWindow(x.options.VersionMissRetryInterval))
	index := -1
	for i := range info.Waiters {
		if info.Waiters[i].OwnerId == ownerId && info.Waiters[i].Mode == mode {
			index = i
			break
		}
	}
	if index < 0 {
		return nil
	}
	if !info.Waiters[index].Until.IsZero() && !info.Waiters[index].Until.Before(deadline) {
		return nil
	}
	info.Waiters[index].Until = deadline
	oldVersion := info.Version
	next, err := nextVersion(oldVersion)
	if err != nil {
		return err
	}
	info.Version = next
	stored := cloneLockInformation(info)
	if err := x.storageExecutor.UpdateWithVersion(ctx, e.Fork(), x.options.LockId, oldVersion, next, stored); err != nil {
		return err
	}
	x.notifySubscribers(x.options.LockId, stored)
	x.publishWaitChanged(ctx, ownerId)
	return nil
}
