package storage_lock

import (
	"context"
	"sync"
	"time"
)

// ------------------------------------------------ ---------------------------------------------------------------------

// MemoryStorage 把锁存储在内存中，可以借助这个实现单机的锁，算是对内部的锁的一个扩展，但是似乎作用不是很大，仅仅是为了丰富实现...
type MemoryStorage struct {

	// 实际存储锁的map
	storageMap map[string]*MemoryStorageValue

	// 用于线程安全的操作
	storageLock sync.RWMutex
}

var _ Storage = &MemoryStorage{}

func NewStorageLockUseMemory() Storage {
	return NewMemoryStorage()
}

func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{
		storageMap:  make(map[string]*MemoryStorageValue),
		storageLock: sync.RWMutex{},
	}
}

func (x *MemoryStorage) Init() error {
	// 没有要初始化的，在创建的时候就初始化了
	return nil
}

func (x *MemoryStorage) Get(ctx context.Context, lockId string) (string, error) {
	x.storageLock.RLock()
	defer x.storageLock.RUnlock()

	value, exists := x.storageMap[lockId]
	if !exists {
		return "", ErrLockNotFound
	} else {
		return value.LockInformationJsonString, nil
	}
}

func (x *MemoryStorage) UpdateWithVersion(ctx context.Context, lockId string, exceptedVersion, newVersion Version, lockInformationJsonString string) error {
	x.storageLock.Lock()
	defer x.storageLock.Unlock()

	oldValue, exists := x.storageMap[lockId]
	if !exists {
		return ErrLockNotFound
	}
	if oldValue.Version != exceptedVersion {
		return ErrVersionMiss
	}
	oldValue.LockInformationJsonString = lockInformationJsonString
	oldValue.Version = newVersion
	return nil
}

func (x *MemoryStorage) InsertWithVersion(ctx context.Context, lockId string, version Version, lockInformationJsonString string) error {
	x.storageLock.Lock()
	defer x.storageLock.Unlock()

	_, exists := x.storageMap[lockId]
	if exists {
		return ErrLockAlreadyExists
	}
	x.storageMap[lockId] = &MemoryStorageValue{
		LockId:                    lockId,
		Version:                   version,
		LockInformationJsonString: lockInformationJsonString,
	}
	return nil
}

func (x *MemoryStorage) DeleteWithVersion(ctx context.Context, lockId string, exceptedVersion Version) error {
	x.storageLock.Lock()
	defer x.storageLock.Unlock()

	oldValue, exists := x.storageMap[lockId]
	if !exists {
		return ErrLockNotFound
	}
	if oldValue.Version != exceptedVersion {
		return ErrVersionMiss
	}
	delete(x.storageMap, lockId)
	return nil
}

func (x *MemoryStorage) GetTime(ctx context.Context) (time.Time, error) {
	return time.Now(), nil
}

// ------------------------------------------------ ---------------------------------------------------------------------

type MemoryStorageValue struct {
	LockId                    string
	Version                   Version
	LockInformationJsonString string
}

// ------------------------------------------------ ---------------------------------------------------------------------
