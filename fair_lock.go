package storage_lock

import (
	"context"
	"fmt"

	go_storage "github.com/storage-lock/go-storage"
)

// FairLock 是跨进程公平锁的薄包装。
// 公平顺序写在锁记录的等待队列里，不使用进程内队列，也不让所有调用方共用一个 owner。
// 对象存储只要满足单 key 线性一致 CAS 就可以用；它不要求存储另有“顺序节点”。
type FairLock struct {
	lock *StorageLock
}

// NewFairLock 创建一把开启 Fair 的锁。owner 由每次 Lock/TryLock/UnLock 传入。
func NewFairLock(storage go_storage.Storage, lockId string) (*FairLock, error) {
	if storage == nil {
		return nil, ErrStorageNil
	}
	if lockId == "" {
		return nil, fmt.Errorf("lockId is empty")
	}
	options := NewStorageLockOptionsWithLockId(lockId)
	options.Fair = true
	return NewFairLockWithOptions(storage, options)
}

// NewFairLockWithOptions 使用调用方的配置创建公平锁，并强制打开 Fair。
func NewFairLockWithOptions(storage go_storage.Storage, options *StorageLockOptions) (*FairLock, error) {
	if options == nil {
		return nil, ErrInvalidOptions
	}
	copied := *options
	copied.Fair = true
	lock, err := NewStorageLockWithOptions(storage, &copied)
	if err != nil {
		return nil, err
	}
	return &FairLock{lock: lock}, nil
}

// Lock 按锁记录中的等待顺序获取。ownerId 必须是这个调用方自己的身份。
func (fl *FairLock) Lock(ctx context.Context, ownerId string) error {
	return fl.lock.Lock(ctx, ownerId)
}

// TryLock 只尝试一次。还没轮到自己时返回 ErrNotYourTurn，不会插队。
func (fl *FairLock) TryLock(ctx context.Context, ownerId string) error {
	return fl.lock.TryLock(ctx, ownerId)
}

// UnLock 释放当前 owner 持有的公平锁。
func (fl *FairLock) UnLock(ctx context.Context, ownerId string) error {
	return fl.lock.UnLock(ctx, ownerId)
}

// LockId 返回锁的唯一标识。
func (fl *FairLock) LockId() string {
	return fl.lock.options.LockId
}

// IsFair 返回 true。公平性来自存储记录里的队列，不是这个包装器自己的内存队列。
func (fl *FairLock) IsFair() bool {
	return fl.lock.options.Fair
}

// StorageLock 返回被包装的锁，供需要 Inspect、Waiters 或 fencing token 的调用方使用。
func (fl *FairLock) StorageLock() *StorageLock {
	return fl.lock
}
