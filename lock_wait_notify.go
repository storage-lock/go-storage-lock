package storage_lock

import (
	"context"
	"time"

	"github.com/storage-lock/go-storage"
)

// waitBeforeRetry 在下一轮获取前等待。公平排队时先把自己的登记续期，再等锁变化。
func (x *StorageLock) waitBeforeRetry(ctx context.Context, ownerId, mode string, interval time.Duration) error {
	if x.options.Fair {
		_ = x.renewWaiterRegistration(ctx, ownerId, mode)
	}
	return x.waitForLockChange(ctx, interval)
}

// waitForLockChange 等待锁记录变化、重试间隔结束，或调用方取消。
// 存储没有通知能力、订阅失败、或通知通道已经关闭时，退回按间隔等待，不空转。
// 收到通知只表示可以再试一次条件写，不表示锁已经拿到。
func (x *StorageLock) waitForLockChange(ctx context.Context, interval time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	watchable, ok := x.storage.(storage.LockWatchable)
	if !ok {
		return waitContext(ctx, interval)
	}
	watchCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	changes, err := watchable.Watch(watchCtx, x.options.LockId)
	if err != nil || changes == nil {
		return waitContext(ctx, interval)
	}
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return ctx.Err()
	case _, open := <-changes:
		if !open {
			return waitTimer(ctx, timer)
		}
		return finishWatchWake(ctx)
	}
}

// waitTimer 把已经开始的计时器走完。通道关闭时用它消化剩余间隔，避免马上再订一次。
func waitTimer(ctx context.Context, timer *time.Timer) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return ctx.Err()
	}
}

// finishWatchWake 在收到一条锁变化之后返回。调用方上下文已经取消时，取消优先于这次醒来。
func finishWatchWake(ctx context.Context) error {
	return ctx.Err()
}
