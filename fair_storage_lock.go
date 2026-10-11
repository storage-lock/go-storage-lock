package storage_lock

import (
	"context"
	"fmt"
	go_storage "github.com/storage-lock/go-storage"
	"sync"
	"time"
)

// FairStorageLock 公平锁包装器
// 在单进程内保证多个 goroutine 按 FIFO 顺序获取锁，防止饥饿。
type FairStorageLock struct {
	storageLock *StorageLock
	mu          sync.Mutex
	owner       string
	queue       []chan struct{}
	closed      bool
}

func NewFairStorageLock(storage go_storage.Storage, lockId string) (*FairStorageLock, error) {
	storageLock, err := NewStorageLock(storage, lockId)
	if err != nil {
		return nil, err
	}
	return &FairStorageLock{storageLock: storageLock}, nil
}

func NewFairStorageLockWithOptions(storage go_storage.Storage, options *StorageLockOptions) (*FairStorageLock, error) {
	storageLock, err := NewStorageLockWithOptions(storage, options)
	if err != nil {
		return nil, err
	}
	return &FairStorageLock{storageLock: storageLock}, nil
}

func (x *FairStorageLock) Lock(ctx context.Context, ownerId string) error {
	for {
		x.mu.Lock()
		if x.closed {
			x.mu.Unlock()
			return fmt.Errorf("fair lock: closed")
		}

		// 自己已持有，允许重入
		if x.owner == ownerId {
			x.mu.Unlock()
			return x.storageLock.Lock(ctx, ownerId)
		}

		// 无人持有，尝试获取。失败时继续持有互斥锁，只等待后重试，不能把调用方加入队列后再解锁。
		if x.owner == "" {
			x.mu.Unlock()
			err := x.storageLock.Lock(ctx, ownerId)
			x.mu.Lock()
			if x.closed {
				x.mu.Unlock()
				if err == nil {
					_ = x.storageLock.UnLock(context.Background(), ownerId)
				}
				return fmt.Errorf("fair lock: closed")
			}
			if err == nil {
				x.owner = ownerId
				x.mu.Unlock()
				return nil
			}
			x.mu.Unlock()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(x.storageLock.options.VersionMissRetryInterval):
				continue
			}
		}

		// 其他人持有，加入等待队列
		ch := make(chan struct{}, 1)
		x.queue = append(x.queue, ch)
		x.mu.Unlock()

		select {
		case <-ctx.Done():
			x.mu.Lock()
			for i, q := range x.queue {
				if q == ch {
					x.queue = append(x.queue[:i], x.queue[i+1:]...)
					break
				}
			}
			x.mu.Unlock()
			return ctx.Err()
		case <-time.After(x.storageLock.options.VersionMissRetryInterval):
			continue
		}
	}
}

func (x *FairStorageLock) UnLock(ctx context.Context, ownerId string) error {
	err := x.storageLock.UnLock(ctx, ownerId)
	if err != nil {
		return err
	}

	x.mu.Lock()
	if x.owner == ownerId {
		x.owner = ""
		if len(x.queue) > 0 {
			next := x.queue[0]
			x.queue = x.queue[1:]
			select {
			case next <- struct{}{}:
			default:
			}
		}
	}
	x.mu.Unlock()

	return nil
}

func (x *FairStorageLock) GetStorageLock() *StorageLock {
	return x.storageLock
}

// QueueLength 返回当前进程内还在排队的等待者数量。
func (x *FairStorageLock) QueueLength() int {
	x.mu.Lock()
	defer x.mu.Unlock()
	return len(x.queue)
}

func (x *FairStorageLock) Close(ctx context.Context) error {
	x.mu.Lock()
	defer x.mu.Unlock()
	x.closed = true
	for _, ch := range x.queue {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
	x.queue = nil
	return nil
}

var ErrFairLockNotOwner = fmt.Errorf("fair lock: not the lock owner")
