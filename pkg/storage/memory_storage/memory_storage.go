package memory_storage

import (
	"context"
	"encoding/json"
	"github.com/golang-infrastructure/go-iterator"
	"github.com/storage-lock/go-storage-lock/pkg/storage"
	"github.com/storage-lock/go-storage-lock/pkg/storage_lock"
	"sync"
	"time"
)

// ------------------------------------------------ ---------------------------------------------------------------------

// MemoryStorage 把锁存储在内存中，可以借助这个实现单机的锁，算是对内部的锁的一个扩展，但是似乎作用不是很大，仅仅是为了丰富实现...
// 也可以认为这个Storage是一个实现的样例，其它的存储引擎的实现可以参考此实现的逻辑
type MemoryStorage struct {

	// 实际存储锁的map
	storageMap map[string]*MemoryStorageValue

	// 用于线程安全的操作
	storageLock sync.RWMutex
}

var _ storage.Storage = &MemoryStorage{}

func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{
		storageMap:  make(map[string]*MemoryStorageValue),
		storageLock: sync.RWMutex{},
	}
}

func (x *MemoryStorage) GetName() string {
	return "memory-storage"
}

func (x *MemoryStorage) Init(ctx context.Context) error {
	// 没有要初始化的，在创建的时候就初始化了
	return nil
}

func (x *MemoryStorage) Get(ctx context.Context, lockId string) (string, error) {
	x.storageLock.RLock()
	defer x.storageLock.RUnlock()

	value, exists := x.storageMap[lockId]
	if !exists {
		return "", storage_lock.ErrLockNotFound
	} else {
		return value.LockInformationJsonString, nil
	}
}

func (x *MemoryStorage) UpdateWithVersion(ctx context.Context, lockId string, exceptedVersion, newVersion storage.Version, lockInformation *storage.LockInformation) error {
	x.storageLock.Lock()
	defer x.storageLock.Unlock()

	// 被更新的锁必须已经存在，否则无法更新
	oldValue, exists := x.storageMap[lockId]
	if !exists {
		return storage_lock.ErrLockNotFound
	}

	// 乐观锁的版本必须能够对应得上，否则拒绝更新
	if oldValue.Version != exceptedVersion {
		return storage_lock.ErrVersionMiss
	}

	// 开始更新锁的信息和版本
	oldValue.LockInformationJsonString = lockInformation.ToJsonString()
	oldValue.Version = newVersion
	return nil
}

func (x *MemoryStorage) InsertWithVersion(ctx context.Context, lockId string, version storage.Version, lockInformation *storage.LockInformation) error {
	x.storageLock.Lock()
	defer x.storageLock.Unlock()

	// 插入的时候之前的锁不能存在，否则认为是插入失败
	_, exists := x.storageMap[lockId]
	if exists {
		return storage_lock.ErrLockAlreadyExists
	}

	// 开始插入
	x.storageMap[lockId] = &MemoryStorageValue{
		LockId:                    lockId,
		Version:                   version,
		LockInformationJsonString: lockInformation.ToJsonString(),
	}
	return nil
}

func (x *MemoryStorage) DeleteWithVersion(ctx context.Context, lockId string, exceptedVersion storage.Version, lockInformation *storage.LockInformation) error {
	x.storageLock.Lock()
	defer x.storageLock.Unlock()

	// 被删除的锁必须已经存在，否则删除失败
	oldValue, exists := x.storageMap[lockId]
	if !exists {
		return storage_lock.ErrLockNotFound
	}

	// 期望的版本号必须相等，否则无法删除
	if oldValue.Version != exceptedVersion {
		return storage_lock.ErrVersionMiss
	}

	// 开始删除
	delete(x.storageMap, lockId)

	return nil
}

func (x *MemoryStorage) GetTime(ctx context.Context) (time.Time, error) {
	// 因为是单机的内存存储，所以直接返回当前机器的时间
	return time.Now(), nil
}

func (x *MemoryStorage) Close(ctx context.Context) error {
	return nil
}

func (x *MemoryStorage) List(ctx context.Context) (iterator.Iterator[*storage.LockInformation], error) {
	slice := make([]*storage.LockInformation, 0)
	for _, lock := range x.storageMap {
		info := &storage.LockInformation{}
		err := json.Unmarshal([]byte(lock.LockInformationJsonString), &info)
		if err != nil {
			return nil, err
		}
		slice = append(slice, info)
	}
	return iterator.FromSlice(slice), nil
}

// ------------------------------------------------ ---------------------------------------------------------------------

// MemoryStorageValue 锁在内存中的实际存储结构
type MemoryStorageValue struct {

	// 存储的是哪个锁的信息
	LockId string

	// 锁的版本号是多少
	Version storage.Version

	// 锁的信息序列化为JSON字符串存储在这个字段
	LockInformationJsonString string
}

// ------------------------------------------------ ---------------------------------------------------------------------
