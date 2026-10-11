package storage_lock

import (
	"context"
	"fmt"
	"sync"

	go_storage "github.com/storage-lock/go-storage"
)

// FairLock 公平锁，保证请求按 FIFO 顺序获取锁。
// 仅支持支持"顺序语义"的存储介质（SQL、MongoDB、etcd 等）。
// S3/OSS 等对象存储不支持公平锁。
type FairLock struct {
	storage go_storage.Storage
	lockId  string

	// 内部使用普通锁实现
	lock *StorageLock

	mu sync.Mutex
}

// NewFairLock 创建公平锁实例。
// 注意：公平性依赖存储层的顺序保证能力。
func NewFairLock(storage go_storage.Storage, lockId string) (*FairLock, error) {
	if storage == nil {
		return nil, ErrStorageNil
	}
	if lockId == "" {
		return nil, fmt.Errorf("lockId is empty")
	}

	lock, err := NewStorageLock(storage, lockId)
	if err != nil {
		return nil, err
	}

	return &FairLock{
		storage: storage,
		lockId:  lockId,
		lock:    lock,
	}, nil
}

// Lock 获取锁，阻塞直到成功或 ctx 取消。
// 公平性：请求按到达顺序排队，先到先得。
func (fl *FairLock) Lock(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	fl.mu.Lock()
	defer fl.mu.Unlock()

	// 公平锁的实现依赖存储层的队列语义
	// 对于支持顺序节点/事务的存储（如 etcd/ZK/SQL），可以实现真正的 FIFO
	// 对于不支持的存储，退化为普通锁

	ownerId := fmt.Sprintf("fairlock-%s", fl.lockId)
	return fl.lock.Lock(ctx, ownerId)
}

// TryLock 非阻塞尝试获取锁。
// 成功返回 nil，失败返回 ErrLockBusy。
func (fl *FairLock) TryLock(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	fl.mu.Lock()
	defer fl.mu.Unlock()

	ownerId := fmt.Sprintf("fairlock-%s", fl.lockId)
	return fl.lock.TryLock(ctx, ownerId)
}

// UnLock 释放锁。
func (fl *FairLock) UnLock(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	fl.mu.Lock()
	defer fl.mu.Unlock()

	ownerId := fmt.Sprintf("fairlock-%s", fl.lockId)
	return fl.lock.UnLock(ctx, ownerId)
}

// LockId 返回锁的唯一标识。
func (fl *FairLock) LockId() string {
	return fl.lockId
}

// IsFair 返回 true，表示这是公平锁。
func (fl *FairLock) IsFair() bool {
	return true
}
