package storage_lock

import (
	"context"
	"errors"
	"github.com/storage-lock/go-events"
	storage_events "github.com/storage-lock/go-storage-events"
	"github.com/storage-lock/go-utils"
	"sync/atomic"
	"time"
)

// WatchDogCommonsImpl 用于在锁存在期间为锁的租约续期的协程，当前实现是每个锁在持有期间都会配备一个刷新锁的租约时间的协程
type WatchDogCommonsImpl struct {

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

// WatchDogIDPrefix 看门狗协程分配的ID
const WatchDogIDPrefix = "storage-lock-watch-dog-"

var _ WatchDog = &WatchDogCommonsImpl{}

// NewWatchDogCommonsImpl 创建一只看门狗
func NewWatchDogCommonsImpl(ctx context.Context, e *events.Event, lock *StorageLock, ownerId string) *WatchDogCommonsImpl {

	// 为看门狗协程生成一个唯一ID
	id := utils.RandomID(WatchDogIDPrefix)

	// 设置一些通用的观测属性
	e.SetLockId(lock.options.LockId).
		SetOwnerId(ownerId).
		SetWatchDogId(id).
		SetStorageName(lock.storage.GetName())

	// 发送创建看门狗的事件
	e.Fork().AddActionByName(ActionWatchDogCreate).Publish(ctx)

	return &WatchDogCommonsImpl{
		id:          id,
		isRunning:   atomic.Bool{},
		storageLock: lock,
		ownerId:     ownerId,
	}
}

const WatchDogCommonsImplName = "watch-dog-commons-impl"

func (x *WatchDogCommonsImpl) Name() string {
	return WatchDogCommonsImplName
}

func (x *WatchDogCommonsImpl) GetID() string {
	return x.id
}

// Start 启动看门狗协程
func (x *WatchDogCommonsImpl) Start(ctx context.Context) error {

	// 发送开始的信号
	x.e.Fork().AddActionByName(ActionWatchDogStart).Publish(ctx)

	x.isRunning.Store(true)
	go func() {
		// 已经刷新成功多少次了
		refreshSuccessCount := 0
		// 统计连续多少次发生错误了
		continueErrorCount := 0
		for x.isRunning.Load() {

			// 发送一个租约刷新开始的事件，携带着当前的一些上下文
			refreshBeginAction := events.NewAction(ActionWatchDogRefreshBegin).
				AddPayload("continueErrorCount", continueErrorCount).
				AddPayload("refreshSuccessCount", refreshSuccessCount)
			x.e.Fork().AddAction(refreshBeginAction).Publish(context.Background())

			// 调用刷新的方法进行一次刷新
			refreshBeginTime := time.Now()
			err := x.refreshLeaseExpiredTime()
			if err != nil {
				continueErrorCount++
				// 连续失败次数太多把自己关掉
				// TODO 2023-8-12 20:46:01 cutoff提取为参数，由外部决定
				if continueErrorCount > 10 {
					ctx, cancelFunc := context.WithTimeout(context.Background(), time.Minute*5)
					err := x.Stop(ctx)
					cancelFunc()
					if err != nil {
						x.e.Fork().AddAction(events.NewAction(ActionWatchDogStopError).SetErr(err)).Publish(ctx)
					} else {
						x.e.Fork().AddAction(events.NewAction(ActionWatchDogStopError).SetErr(err)).Publish(ctx)
					}
					x.e.AddAction(events.NewAction(ActionWatchDogExitByTooManyError).
						AddPayload("continueErrorCount", continueErrorCount).
						AddPayload("refreshSuccessCount", refreshSuccessCount))
					break
				}
				x.e.Fork().
					AddAction(events.NewAction("watch-dog-refreshLeaseExpiredTime-error").AddPayload("continueErrorCount", continueErrorCount).SetErr(err)).
					Publish(context.Background())
			} else {

				// 记录当前的刷新成功
				refreshSuccessCount++

				// 把连续错误计数清零
				continueErrorCount = 0

				// 发送锁的租约刷新成功的事件
				refreshSuccessAction := events.NewAction(ActionWatchDogRefreshSuccess).
					AddPayload("continueErrorCount", continueErrorCount).
					AddPayload("refreshSuccessCount", refreshSuccessCount).
					SetErr(err)
				x.e.Fork().AddAction(refreshSuccessAction).Publish(context.Background())

			}

			// 休眠，避免刷新得太频繁导致乐观锁的版本miss率过高对底层存储系统产生负载
			time.Sleep(x.computeRefreshSleepDuration(refreshBeginTime))
		}

		// 给一个退出信号，注意这里是真正的
		x.e.AddActionByName(ActionWatchDogExit).Publish(context.Background())

	}()

	return nil
}

// 计算距离下次刷新应该休眠的时间
func (x *WatchDogCommonsImpl) computeRefreshSleepDuration(refreshBeginTime time.Time) time.Duration {
	cost := time.Now().Sub(refreshBeginTime)
	needSleepDuration := x.storageLock.options.LeaseRefreshInterval - cost
	return needSleepDuration
}

// 刷新锁的过期时间，为其续约
func (x *WatchDogCommonsImpl) refreshLeaseExpiredTime() error {

	e := x.e.Fork().AddActionByName(ActionWatchDogRefresh)

	// 计算操作超时时长，这里就简单的设置为不超过租约的间隔了
	ctx, cancelFunc := context.WithTimeout(context.Background(), x.storageLock.options.LeaseRefreshInterval)
	defer cancelFunc()

	// 查询锁的当前状态
	information, err := x.storageLock.getLockInformation(ctx, x.e, x.storageLock.options.LockId)
	if err != nil {

		// 如果是锁已经不存在了，则先将续租协程停掉，以免在短时间内进行大量获取释放操作时积压了太多无用的续租协程过慢的退出
		if errors.Is(err, ErrLockNotFound) {
			e.AddAction(events.NewAction(ActionLockNotFoundError).SetErr(err))
			err := x.Stop(ctx)
			if err != nil {
				e.AddAction(events.NewAction(ActionWatchDogStopError).SetErr(err))
			} else {
				e.AddAction(events.NewAction(ActionWatchDogStopSuccess).SetErr(err))
			}
		} else {
			e.AddAction(events.NewAction(ActionGetLockInformationError).SetErr(err))
		}

		// 当发生错误的时候只是补充一些上下文发送事件，之后就会退出
		e.Publish(ctx)
		return err
	}

	// 锁已经不是自己持有了，则直接退出，每个续租狗狗都是很忠贞的只为一个owner续租，并不会进行协程复用
	if information.OwnerId != x.ownerId {
		// 触发事件
		e.AddAction(events.NewAction(ActionNotLockOwner).AddPayload("lockInformation", information)).
			AddActionByName(ActionWatchDogStop).
			Publish(ctx)
		return ErrLockNotBelongYou
	}

	// 计算租约续租之后的过期时间，这里计算的时候需要使用到Storage中统一时间源
	expireTime, err := x.storageLock.getLeaseExpireTime(ctx, e.Fork())
	if err != nil {
		e.AddAction(events.NewAction(ActionGetLeaseExpireTimeError).SetErr(err)).Publish(ctx)
		return err
	}
	information.LeaseExpireTime = expireTime

	// 续租算作是一次修改，所以版本号要加一
	lastVersion := information.Version
	information.Version++

	// 尝试更新Storage中存储的锁的信息
	err = x.storageLock.storageExecutor.UpdateWithVersion(ctx, e.Fork(), x.storageLock.options.LockId, lastVersion, information.Version, information)
	if err != nil {
		e.AddAction(events.NewAction(storage_events.ActionStorageUpdateWithVersion + "-error").SetErr(err))
	} else {
		e.AddAction(events.NewAction(storage_events.ActionStorageUpdateWithVersion + "-success"))
	}

	e.Publish(ctx)
	return err
}

// TODO 操作的时候超时时间设置得更精准一些
//// 计算刷新操作允许的超时时间
//func (x *WatchDogCommonsImpl) computeRefreshTimeout() time.Duration {
//	t1 := (x.storageLock.options.LeaseExpireAfter - x.storageLock.options.LeaseRefreshInterval)
//	if timeout < time.Second*30 {
//		timeout = time.Second * 30
//	}
//}

// Stop 停止续租协程
func (x *WatchDogCommonsImpl) Stop(ctx context.Context) error {
	x.e.Fork().AddActionByName(ActionWatchDogStop).Publish(ctx)
	x.isRunning.Store(false)
	return nil
}

func (x *WatchDogCommonsImpl) SetEvent(e *events.Event) {
	x.e = e
}
