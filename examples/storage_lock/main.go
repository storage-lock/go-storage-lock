package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/golang-infrastructure/go-iterator"
	"github.com/storage-lock/go-events"
	"github.com/storage-lock/go-storage"
	storage_lock "github.com/storage-lock/go-storage-lock"
	"sync"
	"time"
)

// ------------------------------------------------ ---------------------------------------------------------------------

// MemoryStorage 把锁存储在内存中，可以借助这个实现进程级别的锁，算是对内部的锁的一个扩展，但是似乎作用不是很大，仅仅是为了丰富实现...
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

func (x *MemoryStorage) CreateWithVersion(ctx context.Context, lockId string, version storage.Version, lockInformation *storage.LockInformation) error {
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

func main() {

	// 锁的id，表示一份临界资源
	lockId := "counter-lock-id"
	// 锁的持久化存储，这里使用基于内存的存储，可以替换为其它的实现，从项目README查看内置的开箱即用的Storage
	storage := NewMemoryStorage()
	// 创建锁的各种选项
	options := storage_lock.NewStorageLockOptionsWithLockId(lockId).AddEventListeners(events.NewListenerWrapper("print", func(ctx context.Context, e *events.Event) {
		//fmt.Println(e.ToJsonString())
	}))
	// 创建一把分布式锁
	lock, err := storage_lock.NewStorageLockWithOptions(storage, options)
	if err != nil {
		panic(err)
	}

	generator := storage_lock.NewOwnerIdGenerator()

	// 临界资源
	counter := 0

	// 第一个参与竞争锁的角色
	var wg sync.WaitGroup
	// 启动一万个协程，每个协程对count加一千次
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer func() {
				wg.Done()
			}()
			ownerId := generator.GenOwnerId()
			for i := 0; i < 1000; i++ {

				// 获取锁
				err := lock.Lock(context.Background(), ownerId)
				if err != nil {
					panic(err)
				}

				// 临界区，操作资源
				counter++
				fmt.Println(counter)

				// 释放锁
				err = lock.UnLock(context.Background(), ownerId)
				if err != nil {
					panic(err)
				}
			}
		}()
	}

	wg.Wait()
	fmt.Println(fmt.Sprintf("counter: %d", counter))

}
