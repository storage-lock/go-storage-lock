package storage_lock

import (
	"context"
	"fmt"
	"github.com/storage-lock/go-events"
	go_storage "github.com/storage-lock/go-storage"
	storage_events "github.com/storage-lock/go-storage-events"
	"math/rand"
	"sync"
	"time"
)

// StorageLock 基于存储介质的锁模型实现，底层存储介质Storage是可插拔的
type StorageLock struct {

	// 锁持久化存储到哪个存储介质上，go_storage.Storage是个接口，用来把锁进行持久化存储，这个接口有很多种不同的实现
	storage go_storage.Storage
	// 调用storage方法的时候不会直接调用，而是通过一层带事件监听和recover包着的执行器来调用，这样我们可以实现对锁的可观测性以及一些更高级的特性
	storageExecutor *storage_events.WithEventSafeExecutor

	// 锁的一些选项，可以高度定制化锁的行为
	options *StorageLockOptions

	// 负责为锁租约续期的看门狗，每个锁在被持有期间都会存在一个续租协程
	// 当锁被获取的时候看门狗启动，当锁被释放的时候看门狗停止
	storageLockWatchDog WatchDog

	// 保护 storageLockWatchDog 的读写，防止 Lock 和 UnLock 并发操作时产生数据竞争
	watchDogMu sync.Mutex

	// subscribeMu 保护 subscribers 的读写，防止并发操作时产生数据竞争
	subscribeMu sync.Mutex

	// subscribers 保存当前 StorageLock 持有的订阅者；按 lockId 分组。
	// 每次锁状态变化时，会向该 lockId 对应的所有 channel 推送最新的 LockInformation。
	subscribers map[string][]chan *go_storage.LockInformation

	// watchCancels 保存当前正在为某个 lockId 转发存储原生 Watch 通知的后台协程的取消函数。
	// 受 subscribeMu 保护，与 subscribers 生命周期一致：第一个订阅者到来时启动，最后一个
	// 订阅者取消订阅时停止，见 storage_lock_subscribe.go。
	watchCancels map[string]context.CancelFunc

	// 做一些ID自动生成的工作
	ownerIdGenerator *OwnerIdGenerator

	// ownershipLost 在看门狗确认持有权丢失时关闭，用来取消仍在运行的业务回调。
	// 正常解锁不会关闭它。每次成功获取独占锁时重新创建。
	ownershipLost chan struct{}
}

// LockIdPrefix 自动生成的锁的ID的前缀，但是不建议使用自动生成的锁ID
const LockIdPrefix = "storage-lock-id-"

// NewStorageLock 指定锁的ID创建锁，其它的选项都使用默认的
func NewStorageLock(storage go_storage.Storage, lockId string) (*StorageLock, error) {
	options := NewStorageLockOptionsWithLockId(lockId)
	return NewStorageLockWithOptions(storage, options)
}

// NewStorageLockWithOptions 创建一个基于存储介质的锁
// storage: 锁持久化的存储介质，不同的介质有不同的实现，比如基于Mysql、基于MongoDB
// options: 创建和维护锁时的相关配置项
func NewStorageLockWithOptions(storage go_storage.Storage, options *StorageLockOptions) (*StorageLock, error) {
	if storage == nil {
		return nil, ErrStorageNil
	}
	if options == nil {
		return nil, ErrInvalidOptions
	}
	// 保存独立配置，避免调用方复用 options 创建其他锁时更改正在运行的锁。
	optionsCopy := *options
	optionsCopy.EventListeners = append([]events.Listener(nil), options.EventListeners...)
	options = &optionsCopy

	// 参数检查
	if err := checkStorageLockOptions(options); err != nil {
		return nil, err
	}

	// 检查存储实现是否满足分布式锁的必要条件
	// 必要条件 1：CAS 原子性（CapabilityCAS）——硬性，不可降级，直接保护互斥性
	// 必要条件 2：可靠时间源——可由"Storage 声明 CapabilityReliableTime"或"外部注入 options.TimeProvider"满足其一
	//   这让对象存储、HTTP 存储等无服务端时钟的介质可通过注入 NTP 时间源接入
	if !options.SkipCapabilityCheck {
		missingCapabilities := go_storage.CheckCapabilities(storage)
		// 若仅缺 ReliableTime 且用户注入了外部 TimeProvider，则视为已满足
		if len(missingCapabilities) > 0 {
			filtered := make([]go_storage.StorageCapability, 0, len(missingCapabilities))
			for _, c := range missingCapabilities {
				if c == go_storage.CapabilityReliableTime && options.TimeProvider != nil {
					continue // 由外部 TimeProvider 替代满足
				}
				filtered = append(filtered, c)
			}
			if len(filtered) > 0 {
				return nil, fmt.Errorf("%w: storage %s missing capabilities: %v", ErrStorageCapabilityMissing, storage.GetName(), filtered)
			}
		}
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

// SetFair 打开或关闭跨进程的先来先服务排队。
// 测试框架要用它把工厂统一创建的锁切到公平模式；业务代码应在创建锁时通过选项设置。
func (x *StorageLock) SetFair(fair bool) {
	x.options.Fair = fair
}

// NewOwnerId 返回一个新的全局唯一持有者标识，供并发测试给每个竞争者分配身份。
func (x *StorageLock) NewOwnerId() string {
	return x.ownerIdGenerator.GenOwnerId()
}

// getTime 获取用于租约计算的时间
// 优先使用外部注入的 options.TimeProvider（用于对象存储、HTTP 存储等无服务端时钟的介质），
// 否则回退到 Storage 自身的时间。两者取其一，由能力校验保证至少有一个可用。
func (x *StorageLock) getTime(ctx context.Context, e *events.Event) (time.Time, error) {
	if x.options.TimeProvider != nil {
		return x.options.TimeProvider.GetTime(ctx)
	}
	return x.storageExecutor.GetTime(ctx, e)
}

// 获取租约下一次的过期时间
func (x *StorageLock) getLeaseExpireTime(ctx context.Context, e *events.Event) (time.Time, error) {
	storageTime, err := x.getTime(ctx, e)
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
func (x *StorageLock) getLockInformation(ctx context.Context, e *events.Event, lockId string) (*go_storage.LockInformation, error) {

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
	return go_storage.LockInformationFromJsonString(lockInformationJsonString)
}

// stopWatchDog 停止看门狗协程并清空引用，带互斥锁保护
func (x *StorageLock) stopWatchDog(ctx context.Context, e *events.Event, lockInformation *go_storage.LockInformation) {
	x.watchDogMu.Lock()
	defer x.watchDogMu.Unlock()

	if x.storageLockWatchDog != nil {
		stopLastWatchDogEvent := e.Fork().SetLockInformation(lockInformation).AddActionByName(ActionWatchDogStop).SetWatchDogId(x.storageLockWatchDog.GetID())
		err := x.storageLockWatchDog.Stop(ctx)
		x.storageLockWatchDog = nil
		if err != nil {
			stopLastWatchDogEvent.AddAction(events.NewAction(ActionWatchDogStopError).SetErr(err))
		} else {
			stopLastWatchDogEvent.AddAction(events.NewAction(ActionWatchDogStopSuccess))
		}
		stopLastWatchDogEvent.Publish(ctx)
	}
}

// setWatchDog 设置看门狗，带互斥锁保护
func (x *StorageLock) setWatchDog(watchDog WatchDog) {
	x.watchDogMu.Lock()
	defer x.watchDogMu.Unlock()
	x.storageLockWatchDog = watchDog
}

// ------------------------------------------------- --------------------------------------------------------------------

const (
	waiterModeRead  = "read"
	waiterModeWrite = "write"
)

// lostChannel 返回当前持有权丢失信号。通道关闭表示看门狗确认租约已经不属于持有者。
func (x *StorageLock) lostChannel() <-chan struct{} {
	x.watchDogMu.Lock()
	defer x.watchDogMu.Unlock()
	if x.ownershipLost == nil {
		x.ownershipLost = make(chan struct{})
	}
	return x.ownershipLost
}

// resetOwnershipLost 在新一次成功持有后重置丢失信号。
func (x *StorageLock) resetOwnershipLost() {
	x.watchDogMu.Lock()
	defer x.watchDogMu.Unlock()
	if x.ownershipLost == nil {
		x.ownershipLost = make(chan struct{})
		return
	}
	select {
	case <-x.ownershipLost:
		x.ownershipLost = make(chan struct{})
	default:
	}
}

// signalOwnershipLost 关闭丢失信号，幂等。
func (x *StorageLock) signalOwnershipLost() {
	x.watchDogMu.Lock()
	defer x.watchDogMu.Unlock()
	if x.ownershipLost == nil {
		return
	}
	select {
	case <-x.ownershipLost:
	default:
		close(x.ownershipLost)
	}
}
