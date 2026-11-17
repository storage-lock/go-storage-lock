package storage_lock

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/storage-lock/go-events"
	storage "github.com/storage-lock/go-storage"
)

// LockStatus 是一次存储读取的快照，不构成后续业务操作的持锁保证。
type LockStatus struct {
	Information    storage.LockInformation
	ObservedAt     time.Time
	Held           bool
	RemainingLease time.Duration
	// Waiters 是观测到的等待队列，按登记顺序排列。
	Waiters []storage.LockWaiter
	// WriteIntentOwner 是当前阻止新读者进入的写者，空表示没有。
	WriteIntentOwner string
	// WriteIntentAge 是写意图已经等待的时长。
	WriteIntentAge time.Duration
}

// Inspect 返回锁记录及基于统一时间源计算的剩余租期。
// 不存在的记录返回 ErrLockNotFound；已释放的版本记录返回 Held=false、RemainingLease=0。
func (x *StorageLock) Inspect(ctx context.Context) (*LockStatus, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	e := x.operationEvent("")
	info, err := x.getLockInformation(ctx, e, x.options.LockId)
	if err != nil {
		return nil, err
	}
	now, err := x.getTime(ctx, e.Fork())
	if err != nil {
		return nil, err
	}
	status := &LockStatus{Information: *info, ObservedAt: now, Waiters: append([]storage.LockWaiter(nil), info.Waiters...), WriteIntentOwner: info.WriteIntentOwner}
	status.Held = info.LockCount > 0 && now.Before(info.LeaseExpireTime)
	if info.WriteIntentOwner != "" && !info.WriteIntentSince.IsZero() && now.After(info.WriteIntentSince) {
		status.WriteIntentAge = now.Sub(info.WriteIntentSince)
	}
	if status.Held {
		status.RemainingLease = info.LeaseExpireTime.Sub(now)
	}
	return status, nil
}

// IsLocked 报告锁当前是否被有效持有。记录不存在时返回 false, nil。
func (x *StorageLock) IsLocked(ctx context.Context) (bool, error) {
	status, err := x.Inspect(ctx)
	if errors.Is(err, ErrLockNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return status.Held, nil
}

// GetOwner 返回当前有效的独占持有者。
// 未持有或记录不存在时返回 ErrLockNotFound。
// 有效共享读锁返回 ErrLockNotBelongYou，不把最后一次条件写留下的 OwnerId 当成独占持有者。
func (x *StorageLock) GetOwner(ctx context.Context) (string, error) {
	status, err := x.Inspect(ctx)
	if err != nil {
		return "", err
	}
	if !status.Held {
		return "", ErrLockNotFound
	}
	if status.Information.ReadLock {
		return "", ErrLockNotBelongYou
	}
	return status.Information.OwnerId, nil
}

// TTL 返回 ownerId 当前持有的独占锁的剩余租期。
// 已过期、已释放或属于其他持有者时返回错误，不会重新获取锁。
func (x *StorageLock) TTL(ctx context.Context, ownerId string) (time.Duration, error) {
	if err := validateOwner(ctx, ownerId); err != nil {
		return 0, err
	}
	status, err := x.Inspect(ctx)
	if err != nil {
		return 0, err
	}
	if err := checkExclusiveOwner(&status.Information, ownerId); err != nil {
		return 0, err
	}
	if !status.Held {
		return 0, ErrLeaseExpired
	}
	return status.RemainingLease, nil
}

// Renew 延长当前持有者的租约，不改变重入深度，也不改变锁的种类。
// 独占锁只认 OwnerId。共享读锁认的是仍在持有者集合里的读者，不是最后一次写下的 OwnerId。
// 已过期的租约必须重新获取。CAS 冲突会等待后重试，等待可通过 ctx 取消。
func (x *StorageLock) Renew(ctx context.Context, ownerId string) error {
	if err := validateOwner(ctx, ownerId); err != nil {
		return err
	}
	e := x.operationEvent(ownerId)
	for {
		err := x.renewOnce(ctx, e.Fork(), ownerId, false)
		if !errors.Is(err, ErrVersionMiss) {
			return err
		}
		if err := waitContext(ctx, x.options.VersionMissRetryInterval+x.retryIntervalRandomBase()); err != nil {
			return err
		}
	}
}

// allowRead 为真时，OwnerId 仍匹配的旧读记录也可以续租。
// 公开 Renew 传入 false，共享读锁只认持有者集合里的读者。
func (x *StorageLock) renewOnce(ctx context.Context, e *events.Event, ownerId string, allowRead bool) error {
	if err := validateOwner(ctx, ownerId); err != nil {
		return err
	}
	info, err := x.getLockInformation(ctx, e, x.options.LockId)
	if err != nil {
		return err
	}
	if info.LockCount <= 0 {
		return ErrLockNotFound
	}
	if info.ReadLock {
		if info.ReadDepth(ownerId) <= 0 && !(allowRead && info.OwnerId == ownerId) {
			return ErrLockNotBelongYou
		}
	} else if info.OwnerId != ownerId {
		return ErrLockNotBelongYou
	}
	expires, err := x.activeLeaseExpiry(ctx, e.Fork(), info)
	if err != nil {
		return err
	}
	next, err := nextVersion(info.Version)
	if err != nil {
		return err
	}
	previous := info.Version
	info.Version = next
	info.LeaseExpireTime = expires
	if err := x.storageExecutor.UpdateWithVersion(ctx, e.Fork(), x.options.LockId, previous, next, info); err != nil {
		return err
	}
	x.notifySubscribers(x.options.LockId, info)
	x.publishRenewed(ctx, ownerId)
	return nil
}

func (x *StorageLock) activeLeaseExpiry(ctx context.Context, e *events.Event, info *storage.LockInformation) (time.Time, error) {
	now, err := x.getTime(ctx, e)
	if err != nil {
		return time.Time{}, err
	}
	if !now.Before(info.LeaseExpireTime) {
		return time.Time{}, ErrLeaseExpired
	}
	expires := now.Add(x.options.LeaseExpireAfter)
	if expires.Before(info.LeaseExpireTime) {
		expires = info.LeaseExpireTime
	}
	return expires, nil
}

func checkExclusiveOwner(info *storage.LockInformation, ownerId string) error {
	if info.LockCount <= 0 {
		return ErrLockNotFound
	}
	if info.OwnerId != ownerId || info.ReadLock {
		return ErrLockNotBelongYou
	}
	return nil
}

func validateOwner(ctx context.Context, ownerId string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if ownerId == "" {
		return ErrOwnerIdEmpty
	}
	return nil
}

func nextVersion(version storage.Version) (storage.Version, error) {
	if version == math.MaxUint64 {
		return 0, ErrVersionExhausted
	}
	return version + 1, nil
}

func waitContext(ctx context.Context, interval time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return ctx.Err()
	}
}

func (x *StorageLock) operationEvent(ownerId string) *events.Event {
	return events.NewEvent(x.options.LockId).SetOwnerId(ownerId).
		SetStorageName(x.storage.GetName()).SetListeners(x.options.EventListeners)
}

func (x *StorageLock) cleanupContext(ctx context.Context) (context.Context, context.CancelFunc) {
	timeout := x.options.UnlockTimeout
	if timeout == 0 {
		timeout = DefaultUnlockTimeout
	}
	return context.WithTimeout(context.WithoutCancel(ctx), timeout)
}

// Waiters 返回当前有效独占锁的等待队列副本，不延长租约。
// 记录不存在或已释放返回 ErrLockNotFound，共享读锁返回 ErrLockNotBelongYou，
// 租约已到点返回 ErrLeaseExpired。
func (x *StorageLock) Waiters(ctx context.Context) ([]storage.LockWaiter, error) {
	status, err := x.Inspect(ctx)
	if err != nil {
		return nil, err
	}
	info := &status.Information
	if info.LockCount <= 0 {
		return nil, ErrLockNotFound
	}
	if info.ReadLock {
		return nil, ErrLockNotBelongYou
	}
	if !status.Held {
		return nil, ErrLeaseExpired
	}
	return status.Waiters, nil
}

// BeginLease 显式获取独占锁并返回业务上下文。
// 返回的 cancel 只停止丢失监听，不释放锁；调用方仍要 UnLock。
// 看门狗确认持有权丢失时，ctx 以 ErrLockLost 取消。关闭看门狗时没有丢失检测，ctx 只跟随调用方。
func (x *StorageLock) BeginLease(ctx context.Context, ownerId string) (context.Context, context.CancelFunc, error) {
	if err := x.Lock(ctx, ownerId); err != nil {
		return nil, nil, err
	}
	owned, cancel := x.OwnedContext(ctx)
	return owned, cancel, nil
}
