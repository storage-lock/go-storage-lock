package storage_lock

import (
	"context"
	"errors"
	"time"

	"github.com/storage-lock/go-storage"
)

// refreshDecision 是看门狗一轮续租失败之后该怎么收场。
type refreshDecision int

const (
	// refreshContinue 这次失败还不能证明租约无法再延长，下一轮继续试。
	refreshContinue refreshDecision = iota
	// refreshDelegate 交给看门狗主循环里原有的分支：被接管和记录消失由它通知，
	// 正常释放（循环已经停了）由它安静退出。
	refreshDelegate
	// refreshNotifyAndStop 租约已经无法诚实延长。通知一次，并停止同一次持有的续租。
	refreshNotifyAndStop
)

// classifyWatchdogRefresh 把一次续租结果收成上面三种收场。
// pastKnown 为真表示时间源已经不早于上次确认的到期时刻，而且这次把时间读到了。
func classifyWatchdogRefresh(err error, running, pastKnown bool) refreshDecision {
	if err == nil {
		return refreshContinue
	}
	if errors.Is(err, ErrLockNotBelongYou) || errors.Is(err, ErrLockNotFound) {
		return refreshDelegate
	}
	if errors.Is(err, ErrLeaseExpired) {
		if running {
			return refreshNotifyAndStop
		}
		return refreshDelegate
	}
	if pastKnown {
		return refreshNotifyAndStop
	}
	return refreshContinue
}

// watchdogStillOwns 报告这条记录是不是 owner 仍在持有的有效租约。
func watchdogStillOwns(info *storage.LockInformation, ownerId string) bool {
	if info == nil || info.LockCount <= 0 {
		return false
	}
	if info.ReadLock {
		return info.ReadDepth(ownerId) > 0
	}
	return info.OwnerId == ownerId
}

// rememberExpiry 记下这次持有上一次还能确认的到期时刻。
func (x *WatchDogCommonsImpl) rememberExpiry(expires time.Time) {
	x.expiryMu.Lock()
	x.knownExpiry = expires
	x.expiryMu.Unlock()
}

// noteKnownLease 在续租前后读一次记录，把仍属于自己的到期时刻记下来。
// 读失败或记录已经不是自己的时候保留上次的值，避免把未知当成“已经过期”。
func (x *WatchDogCommonsImpl) noteKnownLease() {
	ctx, cancel := context.WithTimeout(x.runCtx, x.storageLock.options.LeaseRefreshInterval)
	defer cancel()
	info, err := x.storageLock.getLockInformation(ctx, x.e.Load(), x.lockId)
	if err != nil {
		return
	}
	if !watchdogStillOwns(info, x.ownerId) {
		return
	}
	x.rememberExpiry(info.LeaseExpireTime)
}

// knownLeasePast 用锁的时间源判断上次确认的到期时刻是不是已经到了。
// 第二个返回值是 false 时，时间没读到或还没有确认过到期时刻，调用方不能据此通知。
func (x *WatchDogCommonsImpl) knownLeasePast() (past bool, confirmed bool) {
	x.expiryMu.Lock()
	known := x.knownExpiry
	x.expiryMu.Unlock()
	if known.IsZero() {
		return false, false
	}
	ctx, cancel := context.WithTimeout(x.runCtx, x.storageLock.options.LeaseRefreshInterval)
	defer cancel()
	now, err := x.storageLock.getTime(ctx, x.e.Load())
	if err != nil {
		return false, false
	}
	return !now.Before(known), true
}

// loseHold 对同一次持有只通知一次，然后停掉续租。
// 停在通知之前，后面的刷新循环不会再把这条租约写长。
func (x *WatchDogCommonsImpl) loseHold(err error) {
	if !x.holdLost.CompareAndSwap(false, true) {
		return
	}
	x.stopInternal()
	x.storageLock.signalOwnershipLost()
	x.storageLock.publishLost(context.Background(), x.ownerId)
	x.notifyLockLost(err)
}

// refreshOnce 做一轮续租，并在无法再诚实延长时通知失锁。
// 已经通知过的同一次持有直接返回，不再写存储。
func (x *WatchDogCommonsImpl) refreshOnce() error {
	if x.holdLost.Load() {
		return ErrLeaseExpired
	}
	x.noteKnownLease()
	err := x.refreshLeaseExpiredTime()
	if err == nil {
		x.noteKnownLease()
		return nil
	}
	past, confirmed := x.knownLeasePast()
	if classifyWatchdogRefresh(err, x.isRunning.Load(), confirmed && past) == refreshNotifyAndStop {
		x.loseHold(err)
		return ErrLeaseExpired
	}
	return err
}
