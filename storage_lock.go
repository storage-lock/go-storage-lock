package storage_lock

import (
	"context"
	"github.com/storage-lock/go-events"
	"github.com/storage-lock/go-storage"
	storage_events "github.com/storage-lock/go-storage-events"
	"math/rand"
	"time"
)

// StorageLock 基于存储介质的锁模型实现，底层存储介质Storage是可插拔的
type StorageLock struct {

	// 锁持久化存储到哪个存储介质上，storage.Storage是个接口，用来把锁进行持久化存储，这个接口有很多种不同的实现
	storage storage.Storage
	// 调用storage方法的时候不会直接调用，而是通过一层带事件监听和recover包着的执行器来调用，这样我们可以实现对锁的可观测性以及一些更高级的特性
	storageExecutor *storage_events.WithEventSafeExecutor

	// 锁的一些选项，可以高度定制化锁的行为
	options *StorageLockOptions

	// 负责为锁租约续期的看门狗，每个锁在被持有期间都会存在一个续租协程
	// 当锁被获取的时候看门狗启动，当锁被释放的时候看门狗停止
	storageLockWatchDog WatchDog

	// 做一些ID自动生成的工作
	ownerIdGenerator *OwnerIdGenerator
}

// LockIdPrefix 自动生成的锁的ID的前缀，但是不建议使用自动生成的锁ID
const LockIdPrefix = "storage-lock-id-"

// NewStorageLock 指定锁的ID创建锁，其它的选项都使用默认的
func NewStorageLock(storage storage.Storage, lockId string) (*StorageLock, error) {
	options := NewStorageLockOptionsWithLockId(lockId)
	return NewStorageLockWithOptions(storage, options)
}

// NewStorageLockWithOptions 创建一个基于存储介质的锁
// storage: 锁持久化的存储介质，不同的介质有不同的实现，比如基于Mysql、基于MongoDB
// options: 创建和维护锁时的相关配置项
func NewStorageLockWithOptions(storage storage.Storage, options *StorageLockOptions) (*StorageLock, error) {

	// 参数检查
	if err := checkStorageLockOptions(options); err != nil {
		return nil, err
	}

	// 触发创建锁的事件
	e := events.NewEvent(options.LockId).SetType(events.EventTypeCreateLock).SetStorageName(storage.GetName()).SetListeners(options.EventListeners)
	lock := &StorageLock{
		storage:          storage,
		storageExecutor:  storage_events.NewWithEventSafeExecutor(storage),
		options:          options,
		ownerIdGenerator: NewOwnerIdGenerator(),
	}

	ctx, cancelFunc := context.WithTimeout(context.Background(), time.Minute*5)
	defer cancelFunc()
	e.Publish(ctx)

	return lock, nil
}

// 获取租约下一次的过期时间
func (x *StorageLock) getLeaseExpireTime(ctx context.Context, e *events.Event) (time.Time, error) {
	storageTime, err := x.storageExecutor.GetTime(ctx, e)
	if err != nil {
		var zero time.Time
		return zero, err
	}
	return storageTime.Add(x.options.LeaseExpireAfter), nil
}

// 重试随机间隔基础值，防止惊群效应
func (x *StorageLock) retryIntervalRandomBase() time.Duration {
	return time.Duration(rand.Intn(950)+50) * time.Microsecond
}

// 获取之前的锁保存的信息
// ctx:
// e: 事件流推送
// lockId: 要获取的锁的信息
func (x *StorageLock) getLockInformation(ctx context.Context, e *events.Event, lockId string) (*storage.LockInformation, error) {

	e.AddActionByName("StorageLock.getLockInformation.Begin").Publish(ctx)

	lockInformationJsonString, err := x.storageExecutor.Get(ctx, e.Fork(), lockId)
	if err != nil {
		e.Fork().AddAction(events.NewAction(storage_events.ActionStorageGetError).SetErr(err).AddPayload(storage_events.PayloadLockId, lockId)).Publish(ctx)
		return nil, err
	}

	// 查询不存在的锁
	if lockInformationJsonString == "" {
		e.Fork().AddAction(events.NewAction(ActionLockNotFoundError).AddPayload(storage_events.PayloadLockId, lockId)).Publish(ctx)
		return nil, ErrLockNotFound
	}

	// 触发查询成功的事件
	action := events.NewAction(storage_events.ActionStorageGetSuccess).
		AddPayload(storage_events.PayloadLockId, lockId).
		AddPayload(storage_events.PayloadLockInformationJsonString, lockInformationJsonString)
	e.Fork().AddAction(action).Publish(ctx)
	return storage.LockInformationFromJsonString(lockInformationJsonString)
}

// ------------------------------------------------- --------------------------------------------------------------------
