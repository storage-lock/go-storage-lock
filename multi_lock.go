package storage_lock

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// MultiLock 联锁，同时获取/释放多把锁。
// 要么全部成功，要么全部失败（失败时自动回滚已获取的锁）。
type MultiLock struct {
	locks []*StorageLock

	// 记录当前持有的锁及其 ownerId，用于回滚
	heldLocks map[*StorageLock]string

	mu sync.Mutex
}

// NewMultiLock 创建联锁实例。
// locks: 要同时获取的多把锁，锁的顺序决定获取顺序（可能影响死锁概率）
func NewMultiLock(locks ...*StorageLock) (*MultiLock, error) {
	if len(locks) == 0 {
		return nil, fmt.Errorf("MultiLock requires at least one lock")
	}

	// 检查重复
	seen := make(map[string]bool)
	for _, lock := range locks {
		if lock == nil {
			return nil, fmt.Errorf("MultiLock contains nil lock")
		}
		if seen[lock.options.LockId] {
			return nil, fmt.Errorf("MultiLock contains duplicate lock: %s", lock.options.LockId)
		}
		seen[lock.options.LockId] = true
	}

	return &MultiLock{
		locks:     locks,
		heldLocks: make(map[*StorageLock]string),
	}, nil
}

// Lock 同时获取所有锁，阻塞直到全部成功或 ctx 取消。
// 获取顺序按 NewMultiLock 传入的顺序，失败时自动回滚已获取的锁。
func (ml *MultiLock) Lock(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	ml.mu.Lock()
	defer ml.mu.Unlock()

	// 清空之前的持有记录（如果有的话）
	ml.heldLocks = make(map[*StorageLock]string)

	// 按顺序获取每把锁
	for i, lock := range ml.locks {
		ownerId := fmt.Sprintf("multilock-%d-%d", time.Now().UnixNano(), i)
		err := lock.Lock(ctx, ownerId)
		if err != nil {
			// 获取失败，回滚已获取的锁
			ml.rollback(ctx)
			return fmt.Errorf("failed to acquire lock %s: %w", lock.options.LockId, err)
		}

		// 记录已获取的锁
		ml.heldLocks[lock] = ownerId
	}

	return nil
}

// TryLock 非阻塞尝试同时获取所有锁。
// 任一锁获取失败则全部失败并回滚，返回 ErrLockBusy。
func (ml *MultiLock) TryLock(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	ml.mu.Lock()
	defer ml.mu.Unlock()

	ml.heldLocks = make(map[*StorageLock]string)

	for i, lock := range ml.locks {
		ownerId := fmt.Sprintf("multilock-%d-%d", time.Now().UnixNano(), i)
		err := lock.TryLock(ctx, ownerId)
		if err != nil {
			ml.rollback(ctx)
			if errors.Is(err, ErrLockBusy) {
				return ErrLockBusy
			}
			return fmt.Errorf("failed to try acquire lock %s: %w", lock.options.LockId, err)
		}

		ml.heldLocks[lock] = ownerId
	}

	return nil
}

// UnLock 释放所有已获取的锁。
// 即使部分释放失败也会尝试释放所有锁，错误会被合并返回。
func (ml *MultiLock) UnLock(ctx context.Context) error {
	ml.mu.Lock()
	defer ml.mu.Unlock()

	if len(ml.heldLocks) == 0 {
		return nil
	}

	var errs []error
	for lock, ownerId := range ml.heldLocks {
		if err := lock.UnLock(ctx, ownerId); err != nil {
			errs = append(errs, fmt.Errorf("failed to release lock %s: %w", lock.options.LockId, err))
		}
	}

	// 清空持有记录
	ml.heldLocks = make(map[*StorageLock]string)

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// rollback 回滚已获取的锁（内部使用，不加锁）。
func (ml *MultiLock) rollback(ctx context.Context) {
	for lock, ownerId := range ml.heldLocks {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), DefaultUnlockTimeout)
		_ = lock.UnLock(cleanupCtx, ownerId)
		cancel()
	}
	ml.heldLocks = make(map[*StorageLock]string)
}

// HeldCount 返回当前持有的锁数量。
func (ml *MultiLock) HeldCount() int {
	ml.mu.Lock()
	defer ml.mu.Unlock()
	return len(ml.heldLocks)
}

// LockIds 返回所有参与联锁的 LockId 列表。
func (ml *MultiLock) LockIds() []string {
	ids := make([]string, len(ml.locks))
	for i, lock := range ml.locks {
		ids[i] = lock.options.LockId
	}
	return ids
}
