package storage_lock

import (
	"context"
	"errors"
	"github.com/storage-lock/go-events"
	"github.com/storage-lock/go-utils"
	"sync/atomic"
	"time"
)

// LeaseRefreshGoroutine 用于在锁存在期间为锁的租约续期的协程，当前实现是每个锁在持有期间都会配备一个刷新锁的租约时间的协程
type LeaseRefreshGoroutine struct {

	// 租约续费协程有唯一ID标识，用于区分方便观测
	id string

	// 当前协程运行期间产生的事件都是这个事件的子事件
	e *events.Event

	// 协程是否处在运行状态的标志位，为true表示处在运行状态，为false表示未处在运行状态
	isRunning atomic.Bool

	// 要续哪个锁的租约，当前每个续租的协程只能为一个锁续约，并且每次重新获取锁都会启动一个新的续租协程
	storageLock *StorageLock

	// 是为谁而续这个租约，即锁的持有者
	ownerId string
}

const WatchDogIDPrefix = "storage-lock-watch-dog-"

// NewStorageLockWatchDog 创建一只看门狗
func NewStorageLockWatchDog(e *events.Event, lock *StorageLock, ownerId string) *LeaseRefreshGoroutine {

	// 为看门狗协程生成一个唯一ID
	id := utils.RandomID(WatchDogIDPrefix)

	// 设置一些通用的观测属性
	e.SetLockId(lock.options.LockId).
		SetOwnerId(ownerId).
		SetWatchDogId(id).
		SetStorageName(lock.storage.GetName())

	// 发送创建看门狗的事件
	e.Fork().AppendActionByName("create-watch-dog").Publish(context.Background())

	return &LeaseRefreshGoroutine{
		id:          id,
		isRunning:   atomic.Bool{},
		storageLock: lock,
		ownerId:     ownerId,
	}
}

func (x *LeaseRefreshGoroutine) GetID() string {
	return x.id
}

// Start 启动看门狗协程
func (x *LeaseRefreshGoroutine) Start() {

	x.e.Fork().AppendActionByName("start-watch-dog").Publish(context.Background())

	x.isRunning.Store(true)
	go func() {
		// 统计连续多少次发生错误了
		continueErrorCount := 0
		for x.isRunning.Load() {

			// 超时时间设置为租约有效期的一半，这样子相当于有两次机会，但是超时最短不能小于半分钟，这是为了兼容有些存储介质可能会比较慢的情况
			timeout := (x.storageLock.options.LeaseExpireAfter - x.storageLock.options.LeaseRefreshInterval) / 2
			if timeout < time.Second*30 {
				timeout = time.Second * 30
			}

			ctx, cancelFunc := context.WithTimeout(context.Background(), timeout)
			err := x.refreshLeaseExpiredTime(ctx)
			cancelFunc()
			if err != nil {
				continueErrorCount++
				// 连续失败次数太多把自己关掉
				if continueErrorCount > 10 {
					x.Stop()
				}
			} else {
				continueErrorCount = 0
			}

			// 休眠，避免刷新得太频繁导致乐观锁的版本miss率过高
			time.Sleep(x.storageLock.options.LeaseRefreshInterval)
		}
	}()
}

// Stop 停止续租协程
func (x *LeaseRefreshGoroutine) Stop() {
	x.isRunning.Store(false)
}

// 刷新锁的过期时间，为其续约
func (x *LeaseRefreshGoroutine) refreshLeaseExpiredTime(ctx context.Context) error {

	e := x.e.Fork().AppendActionByName(ActionWatchDogRefreshLease)

	information, err := x.storageLock.getLockInformation(ctx, x.e)
	if err != nil {

		// 如果是锁已经不存在了，则先将续租协程停掉，以免在短时间内进行大量获取释放操作时挤压了太多无用的续租协程过慢的退出
		if errors.Is(err, ErrLockNotFound) {
			e.AppendAction(events.NewAction(ActionLockNotFoundError).SetErr(err))
			x.Stop()
			e.AppendActionByName(ActionWatchDogStop)
		} else {
			e.AppendAction(events.NewAction(ActionGetLockInformationError).SetErr(err))
		}

		e.Publish(ctx)
		return err
	}

	// 锁已经不是自己持有了，则直接退出，每个续租协程都是很忠贞的只为一个owner续租
	if information.OwnerId != x.ownerId {
		e.AppendAction(events.NewAction(ActionNotLockOwner).SetPayload(information.ToJsonString()))
		x.Stop()
		e.AppendActionByName(ActionWatchDogStop).Publish(ctx)
		return ErrLockNotBelongYou
	}

	lastVersion := information.Version
	information.Version++

	expireTime, err := x.storageLock.getLeaseExpireTime(ctx, e)
	if err != nil {
		e.AppendAction(events.NewAction("getLeaseExpireTime-error").SetErr(err)).Publish(ctx)
		return err
	}

	information.LeaseExpireTime = expireTime

	updateAction := events.NewAction(ActionStorageUpdateWithVersion)
	err = x.storageLock.storage.UpdateWithVersion(ctx, x.storageLock.options.LockId, lastVersion, information.Version, information)
	e.Fork().AppendAction(updateAction.End().SetErr(err)).SetLockInformation(information).Publish(ctx)

	return err
}
