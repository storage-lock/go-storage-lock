package storage_lock

import (
	"context"
	"errors"
)

// Unlock 是 UnLock 的惯用拼写别名。
func (x *StorageLock) Unlock(ctx context.Context, ownerId string) error {
	return x.UnLock(ctx, ownerId)
}

// TryWLock 尝试一次获取独占写锁。
func (x *StorageLock) TryWLock(ctx context.Context, ownerId string) error {
	return x.TryLock(ctx, ownerId)
}

// TryRLock 尝试一次获取共享读锁，不等待其他持有者释放。
func (x *StorageLock) TryRLock(ctx context.Context, ownerId string) error {
	if err := validateOwner(ctx, ownerId); err != nil {
		return err
	}
	e := x.operationEvent(ownerId)
	err := x.tryRLock(ctx, e, x.options.LockId, ownerId)
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrVersionMiss) || errors.Is(err, ErrLockAlreadyExists) {
		return ErrLockBusy
	}
	return err
}
