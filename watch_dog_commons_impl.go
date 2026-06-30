package storage_lock

import (
	"context"
	"errors"
	"github.com/storage-lock/go-events"
	storage_events "github.com/storage-lock/go-storage-events"
	"github.com/storage-lock/go-utils"
	"sync"
	"sync/atomic"
	"time"
)

// WatchDogCommonsImpl 用于在锁存在期间为锁的租约续期的协程，当前实现是每个锁在持有期间都会配备一个刷新锁的租约时间的协程
type WatchDogCommonsImpl struct {

	// 租约续费协程有唯一ID标识，用于区分方便观测
	id string
	// 创建看门狗的时候就把锁的id给固定住，防止options被瞎几把改导致流程出错
	lockId string

	// 当前协程运行期间产生的事件都是这个事件的子事件
	e *events.Event

	// 协程是否处在运行状态的标志位，为true表示处在运行状态，为false表示未处在运行状态
	isRunning atomic.Bool

	// 要续哪个锁的租约，当前每个续租的协程只能为一个锁续约，并且每次重新获取锁都会启动一个新的续租协程
	storageLock *StorageLock

	// 是为谁而续这个租约，即锁的持有者
	ownerId string

	// stop: Stop 时 close，用于通知 goroutine 立即退出（可中断 sleep）
	stop chan struct{}
	// done: goroutine 真正退出后 close，Stop 等待此信号确认 goroutine 已结束
	done chan struct{}
	// 确保 stop/done 各只被 close 一次
	stopOnce sync.Once
	doneOnce sync.Once
}

// WatchDogIDPrefix 看门狗协程分配的ID
const WatchDogIDPrefix = "storage-lock-watch-dog-"

var _ WatchDog = &WatchDogCommonsImpl{}

// NewWatchDogCommonsImpl 创建一只看门狗
func NewWatchDogCommonsImpl(ctx context.Context, e *events.Event, lock *StorageLock, ownerId string) *WatchDogCommonsImpl {

	// 为看门狗协程生成一个唯一ID
	lockId := lock.options.LockId
	id := utils.RandomID(WatchDogIDPrefix)

	// 设置一些通用的观测属性
	e.SetLockId(lockId).
		SetOwnerId(ownerId).
		SetWatchDogId(id).
		SetStorageName(lock.storage.GetName())

	// 发送创建看门狗的事件
	e.AddActionByName(ActionWatchDogCreate).Publish(ctx)

	return &WatchDogCommonsImpl{
		id:          id,
		lockId:      lockId,
		isRunning:   atomic.Bool{},
		storageLock: lock,
		ownerId:     ownerId,
		e:           e,
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

	// 初始化 stop/done channel
	// stop: Stop 时 close，goroutine 监听它来立即中断 sleep 退出
	// done: goroutine 退出时 close，Stop 等待它确认 goroutine 已结束
	x.stop = make(chan struct{})
	x.done = make(chan struct{})
	x.isRunning.Store(true)
	go func() {

		// goroutine 退出时关闭 done channel，通知 Stop 已结束
		defer x.doneOnce.Do(func() { close(x.done) })

		// 已经刷新成功多少次了
		refreshSuccessCount := 0
		// 统计连续多少次发生错误了
		continueErrorCount := 0

		// 退出的时候给一个信号，使用 defer 确保在 goroutine 退出时才触发，且能捕获到最终的计数值
		defer func() {
			exitAction := events.NewAction(ActionWatchDogExit).
				AddPayload(PayloadRefreshSuccessCount, refreshSuccessCount).
				AddPayload(PayloadContinueErrorCount, continueErrorCount)
			x.e.Fork().AddAction(exitAction).Publish(context.Background())
		}()

		// 先休眠一下，再死循环刷新
		// 这是针对锁定时间比较短的锁的一个优化，当狗狗休眠结束锁已经被释放掉了，而狗狗也已经被标记为退出状态
		// 能够避免一次无效的刷新，也能够避免因为自身续租而导致的miss率
		// 而对于持有时间比较长的锁来说，也不差这么点时间
		// 时间不要太长，避免协程泄露，1秒封顶
		needSleep := x.storageLock.options.LeaseRefreshInterval
		if needSleep > time.Second {
			needSleep = time.Second
		}
		// 使用 select 监听 stop channel，使 Stop 能立即唤醒此 sleep，避免 UnLock 阻塞
		select {
		case <-x.stop:
			// Stop 已经被调用，直接退出
			return
		case <-time.After(needSleep):
			// 正常唤醒，继续刷新
		}

		for x.isRunning.Load() {

			// 发送一个租约刷新开始的事件，携带着当前的一些上下文
			refreshBeginAction := events.NewAction(ActionWatchDogRefreshBegin).
				AddPayload(PayloadContinueErrorCount, continueErrorCount).
				AddPayload(PayloadRefreshSuccessCount, refreshSuccessCount)
			x.e.Fork().AddAction(refreshBeginAction).Publish(context.Background())

			// 调用刷新的方法进行一次刷新
			refreshBeginTime := time.Now()
			err := x.refreshLeaseExpiredTime()
			if err != nil {
				continueErrorCount++

				// 如果锁已经不是自己持有了，则退出
				if errors.Is(err, ErrLockNotBelongYou) {
					notLockOwnerAction := events.NewAction(ActionNotLockOwner).
						AddPayload(PayloadContinueErrorCount, continueErrorCount).
						AddPayload(PayloadRefreshSuccessCount, refreshSuccessCount).
						SetErr(err)
					x.e.Fork().AddAction(notLockOwnerAction).Publish(context.Background())
					return
				}

				// 租约刷新失败事件
				refreshErrorAction := events.NewAction(ActionWatchDogRefreshError).
					AddPayload(PayloadContinueErrorCount, continueErrorCount).
					AddPayload(PayloadRefreshSuccessCount, refreshSuccessCount).
					SetErr(err)
				x.e.Fork().AddAction(refreshErrorAction).Publish(context.Background())

			} else {

				// 记录当前的刷新成功
				refreshSuccessCount++

				// 把连续错误计数清零
				continueErrorCount = 0

				// 发送锁的租约刷新成功的事件
				refreshSuccessAction := events.NewAction(ActionWatchDogRefreshSuccess).
					AddPayload(PayloadContinueErrorCount, continueErrorCount).
					AddPayload(PayloadRefreshSuccessCount, refreshSuccessCount)
				x.e.Fork().AddAction(refreshSuccessAction).Publish(context.Background())

			}

			// 休眠，避免刷新得太频繁导致乐观锁的版本miss率过高对底层存储系统产生负载
			// 使用 select 监听 stop channel，使 Stop 能立即唤醒此 sleep
			select {
			case <-x.stop:
				// Stop 已经被调用，直接退出循环
				return
			case <-time.After(x.computeRefreshSleepDuration(refreshBeginTime)):
				// 正常唤醒，继续下一次刷新
			}
		}

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

	refreshEvent := x.e.Fork().AddActionByName(ActionWatchDogRefresh)

	// 计算操作超时时长，这里就简单的设置为不超过租约的间隔了
	ctx, cancelFunc := context.WithTimeout(context.Background(), x.storageLock.options.LeaseRefreshInterval)
	defer cancelFunc()

	// 查询锁的当前状态
	information, err := x.storageLock.getLockInformation(ctx, x.e, x.lockId)
	if err != nil {

		// 如果是锁已经不存在了，则先将续租协程停掉，以免在短时间内进行大量获取释放操作时积压了太多无用的续租协程过慢的退出
		if errors.Is(err, ErrLockNotFound) {
			refreshEvent.AddAction(events.NewAction(ActionLockNotFoundError).SetErr(err))
			// 注意：这里在 goroutine 内部，不能调用 Stop（Stop 会等待 done，而死锁）
			// 只设置停止标志并 close stop，让循环自然退出
			x.stopInternal()
			refreshEvent.AddAction(events.NewAction(ActionWatchDogStopSuccess).SetErr(err))
		} else {
			refreshEvent.AddAction(events.NewAction(ActionGetLockInformationError).SetErr(err))
		}

		// 当发生错误的时候只是补充一些上下文发送事件，之后就会退出
		refreshEvent.Publish(ctx)
		return err
	}

	// 锁已经不是自己持有了，则直接退出，每个续租狗狗都是很忠贞的只为一个owner续租，并不会进行协程复用
	if information.OwnerId != x.ownerId {
		// 试图刷新不是自己的锁
		refreshEvent.AddAction(events.NewAction(ActionNotLockOwner).AddPayload(storage_events.PayloadLockInformation, information)).
			AddActionByName(ActionWatchDogStop).
			Publish(ctx)
		return ErrLockNotBelongYou
	}

	// 计算租约续租之后的过期时间，这里计算的时候需要使用到Storage中统一时间源
	expireTime, err := x.storageLock.getLeaseExpireTime(ctx, refreshEvent.Fork())
	if err != nil {
		refreshEvent.AddAction(events.NewAction(ActionGetLeaseExpireTimeError).SetErr(err)).Publish(ctx)
		return err
	}
	information.LeaseExpireTime = expireTime

	// 续租算作是一次修改，所以版本号要加一
	lastVersion := information.Version
	information.Version++

	// 尝试更新Storage中存储的锁的信息
	err = x.storageLock.storageExecutor.UpdateWithVersion(ctx, refreshEvent.Fork(), x.lockId, lastVersion, information.Version, information)
	if err != nil {
		refreshEvent.AddAction(events.NewAction(storage_events.ActionStorageUpdateWithVersion + "-error").SetErr(err))
	} else {
		refreshEvent.AddAction(events.NewAction(storage_events.ActionStorageUpdateWithVersion + "-success"))

		// 防御性检查：UpdateWithVersion 成功后，验证锁的 OwnerId 仍然是自己的
		// 这是为了防止在 Get 和 UpdateWithVersion 之间的时间窗口内，锁被其他人抢占后又被我们错误续租
		verifyInfo, verifyErr := x.storageLock.getLockInformation(ctx, x.e, x.lockId)
		if verifyErr == nil && verifyInfo.OwnerId != x.ownerId {
			// OwnerId 已经不是自己了，说明我们可能错误地续租了别人的锁
			// 记录告警事件，但不尝试修正——因为此时锁已经合法地属于别人
			refreshEvent.AddAction(events.NewAction(ActionWatchDogOwnerIdMismatch).
				AddPayload("expected_owner_id", x.ownerId).
				AddPayload("actual_owner_id", verifyInfo.OwnerId))
		}
	}

	refreshEvent.Publish(ctx)
	return err
}

// stopInternal 在 goroutine 内部调用，只设置停止标志并 close stop，
// 不等待 done（否则会死锁：等待自身退出）。goroutine 会在下次 select 时收到 stop 信号退出
func (x *WatchDogCommonsImpl) stopInternal() {
	x.isRunning.Store(false)
	x.stopOnce.Do(func() { close(x.stop) })
}

// Stop 停止续租协程，并等待 goroutine 真正退出后才返回
// 通过 close(stop) 通知 goroutine 立即中断 sleep 退出，避免 UnLock 长时间阻塞
func (x *WatchDogCommonsImpl) Stop(ctx context.Context) error {

	x.isRunning.Store(false)
	// close stop channel，唤醒 goroutine 中正在 select 的 sleep，使其立即退出
	x.stopOnce.Do(func() { close(x.stop) })
	x.e.Fork().AddActionByName(ActionWatchDogStop).Publish(ctx)

	// 等待 goroutine 真正退出（收到 done 信号）
	if x.done != nil {
		select {
		case <-x.done:
			// goroutine 已退出
			return nil
		case <-ctx.Done():
			// 等待超时，goroutine 可能仍在运行，但不再阻塞调用者
			return ctx.Err()
		}
	}

	return nil
}

// SetEvent 允许在创建后更改日志源
func (x *WatchDogCommonsImpl) SetEvent(e *events.Event) {

	// 更新事件源
	x.e = e

	// 触发看门狗事件源更改事件
	ctx, cancelFunc := context.WithTimeout(context.Background(), time.Minute*5)
	defer cancelFunc()
	x.e.AddAction(events.NewAction(ActionWatchDogSetEvent)).Publish(ctx)
}
