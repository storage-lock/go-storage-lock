package storage_lock

import (
	"context"
	"fmt"
	go_storage "github.com/storage-lock/go-storage"
	"sync"
	"time"
)

// LocalOrderedLock 只给同一个进程里的 goroutine 排队。
// 队列在内存里，别的进程看不到，也不能代替 StorageLockOptions.Fair。
// 跨进程的先来先服务用 FairLock，或创建锁时把 Fair 设为 true。
type LocalOrderedLock struct {
	storageLock *StorageLock
	mu          sync.Mutex
	owner       string
	holding     bool
	queue       []chan struct{}
	closed      bool
}

func NewLocalOrderedLock(storage go_storage.Storage, lockId string) (*LocalOrderedLock, error) {
	storageLock, err := NewStorageLock(storage, lockId)
	if err != nil {
		return nil, err
	}
	return &LocalOrderedLock{storageLock: storageLock}, nil
}

func NewLocalOrderedLockWithOptions(storage go_storage.Storage, options *StorageLockOptions) (*LocalOrderedLock, error) {
	storageLock, err := NewStorageLockWithOptions(storage, options)
	if err != nil {
		return nil, err
	}
	return &LocalOrderedLock{storageLock: storageLock}, nil
}

func (x *LocalOrderedLock) Lock(ctx context.Context, ownerId string) error {
	x.mu.Lock()
	var ticket chan struct{}
	for {
		if x.closed {
			x.abandonTicketLocked(ticket)
			x.mu.Unlock()
			return fmt.Errorf("fair lock: closed")
		}
		if err := ctx.Err(); err != nil {
			x.abandonTicketLocked(ticket)
			if x.owner == "" {
				x.signalHeadLocked()
			}
			x.mu.Unlock()
			return err
		}

		// 已经持有才允许重入。预约阶段的 owner 还没拿到存储锁，后来者必须排队。
		if x.holding && x.owner == ownerId && ticket == nil {
			x.mu.Unlock()
			return x.storageLock.Lock(ctx, ownerId)
		}

		// 只有队首、且当前没人持有时才能碰存储锁。后面的票不能靠定时器插队。
		if x.owner == "" && x.isHeadLocked(ticket) {
			if ticket != nil {
				x.abandonTicketLocked(ticket)
				ticket = nil
			}
			x.owner = ownerId
			x.holding = false
			x.mu.Unlock()
			return x.finishAcquire(ctx, ownerId)
		}

		if ticket == nil {
			ticket = make(chan struct{}, 1)
			x.queue = append(x.queue, ticket)
		}
		x.mu.Unlock()
		select {
		case <-ctx.Done():
			x.mu.Lock()
			x.abandonTicketLocked(ticket)
			if x.owner == "" {
				x.signalHeadLocked()
			}
			x.mu.Unlock()
			return ctx.Err()
		case <-ticket:
			x.mu.Lock()
		case <-time.After(x.storageLock.options.VersionMissRetryInterval):
			// 只是重新检查队首，不在这里获取存储锁。
			x.mu.Lock()
		}
	}
}

// isHeadLocked 报告 ticket 是否可以获取。ticket 为空且队列为空，表示没有人排在前面。
func (x *LocalOrderedLock) isHeadLocked(ticket chan struct{}) bool {
	if len(x.queue) == 0 {
		return ticket == nil
	}
	return ticket != nil && x.queue[0] == ticket
}

func (x *LocalOrderedLock) finishAcquire(ctx context.Context, ownerId string) error {
	for {
		if err := ctx.Err(); err != nil {
			x.releaseReservation(ownerId)
			return err
		}
		err := x.storageLock.Lock(ctx, ownerId)
		x.mu.Lock()
		closed := x.closed
		x.mu.Unlock()
		if closed {
			x.releaseReservation(ownerId)
			if err == nil {
				_ = x.storageLock.UnLock(context.Background(), ownerId)
			}
			return fmt.Errorf("fair lock: closed")
		}
		if err == nil {
			x.mu.Lock()
			if x.owner == ownerId {
				x.holding = true
			}
			x.mu.Unlock()
			return nil
		}
		select {
		case <-ctx.Done():
			x.releaseReservation(ownerId)
			return ctx.Err()
		case <-time.After(x.storageLock.options.VersionMissRetryInterval):
		}
	}
}

func (x *LocalOrderedLock) releaseReservation(ownerId string) {
	x.mu.Lock()
	defer x.mu.Unlock()
	if x.owner == ownerId && !x.holding {
		x.owner = ""
		x.signalHeadLocked()
	}
}

func (x *LocalOrderedLock) abandonTicketLocked(ticket chan struct{}) {
	if ticket == nil {
		return
	}
	for i, queued := range x.queue {
		if queued == ticket {
			x.queue = append(x.queue[:i], x.queue[i+1:]...)
			return
		}
	}
}

func (x *LocalOrderedLock) signalHeadLocked() {
	if len(x.queue) == 0 {
		return
	}
	select {
	case x.queue[0] <- struct{}{}:
	default:
	}
}

func (x *LocalOrderedLock) UnLock(ctx context.Context, ownerId string) error {
	err := x.storageLock.UnLock(ctx, ownerId)
	if err != nil {
		return err
	}
	status, statusErr := x.storageLock.Inspect(ctx)

	x.mu.Lock()
	defer x.mu.Unlock()
	if x.owner != ownerId {
		return nil
	}
	// 重入还没放完时，队列继续等真正的释放。
	if statusErr == nil && status.Held && !status.Information.ReadLock && status.Information.OwnerId == ownerId && status.Information.LockCount > 0 {
		x.holding = true
		return nil
	}
	x.owner = ""
	x.holding = false
	x.signalHeadLocked()
	return nil
}

func (x *LocalOrderedLock) GetStorageLock() *StorageLock {
	return x.storageLock
}

// QueueLength 返回当前进程内还在排队的等待者数量。
func (x *LocalOrderedLock) QueueLength() int {
	x.mu.Lock()
	defer x.mu.Unlock()
	return len(x.queue)
}

func (x *LocalOrderedLock) Close(ctx context.Context) error {
	x.mu.Lock()
	defer x.mu.Unlock()
	x.closed = true
	for _, ticket := range x.queue {
		select {
		case ticket <- struct{}{}:
		default:
		}
	}
	x.queue = nil
	return nil
}

var ErrLocalOrderedLockNotOwner = fmt.Errorf("fair lock: not the lock owner")

// FairStorageLock 是旧名字。它只在本进程内排队，不提供跨进程公平。
// 新代码使用 LocalOrderedLock。
type FairStorageLock = LocalOrderedLock

// NewFairStorageLock 保留旧构造函数，行为与 NewLocalOrderedLock 相同。
func NewFairStorageLock(storage go_storage.Storage, lockId string) (*LocalOrderedLock, error) {
	return NewLocalOrderedLock(storage, lockId)
}

// NewFairStorageLockWithOptions 保留旧构造函数，行为与 NewLocalOrderedLockWithOptions 相同。
func NewFairStorageLockWithOptions(storage go_storage.Storage, options *StorageLockOptions) (*LocalOrderedLock, error) {
	return NewLocalOrderedLockWithOptions(storage, options)
}

// ErrFairLockNotOwner 保留旧错误名。
var ErrFairLockNotOwner = ErrLocalOrderedLockNotOwner
