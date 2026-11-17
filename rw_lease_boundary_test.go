package storage_lock

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/storage-lock/go-events"
	"github.com/stretchr/testify/require"
)

// TestSharedReadRenewalDoesNotFalseLoseAndExactExpiry 覆盖发布门槛：
// 两个读者同时持有时，后加入者改写 OwnerId 不得让先加入者收到持有丢失；
// 至少发生一次续租后两人仍算持有，写者不能取得这把锁；
// 存储时间恰好等于 LeaseExpireTime 时，读路径不得再把租约推远。
func TestSharedReadRenewalDoesNotFalseLoseAndExactExpiry(t *testing.T) {
	store := newMockSubscribeStorage()
	clock := newMockSubscribeTimeProvider(time.Now())
	const lockID = "rw-lease"
	var lost atomic.Bool
	readerA := newSharedReadLock(t, store, clock, lockID, &lost)
	readerB := newSharedReadLock(t, store, clock, lockID, nil)
	writer := newSharedReadLock(t, store, clock, lockID, nil)
	ctx := context.Background()

	require.NoError(t, readerA.RLock(ctx, "reader-a"))
	require.NoError(t, readerB.RLock(ctx, "reader-b"))
	joined, err := readerA.Inspect(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, joined.Information.ReadDepth("reader-a"))
	require.Equal(t, 1, joined.Information.ReadDepth("reader-b"))
	joinedVersion := joined.Information.Version

	require.Eventually(t, func() bool {
		if lost.Load() {
			return false
		}
		status, err := readerA.Inspect(ctx)
		if err != nil {
			return false
		}
		return status.Information.Version > joinedVersion &&
			status.Information.ReadDepth("reader-a") > 0 &&
			status.Information.ReadDepth("reader-b") > 0
	}, 5*time.Second, 20*time.Millisecond)
	require.False(t, lost.Load(), "先加入的读者在仍然持有时收到了持有丢失")

	require.ErrorIs(t, writer.TryLock(ctx, "writer"), ErrLockBusy)
	held, err := readerA.Inspect(ctx)
	require.NoError(t, err)
	require.Greater(t, held.Information.ReadDepth("reader-a"), 0)
	require.Greater(t, held.Information.ReadDepth("reader-b"), 0)
	require.True(t, held.Held)

	time.Sleep(300 * time.Millisecond)
	require.False(t, lost.Load(), "续租之后先加入的读者仍被判定丢失")

	readerA.stopWatchDog(ctx, events.NewEvent(lockID), nil)
	readerB.stopWatchDog(ctx, events.NewEvent(lockID), nil)
	before, err := readerA.Inspect(ctx)
	require.NoError(t, err)
	beforeExpire := before.Information.LeaseExpireTime
	beforeVersion := before.Information.Version
	depthA := before.Information.ReadDepth("reader-a")
	depthB := before.Information.ReadDepth("reader-b")

	clock.Set(beforeExpire)
	err = readerA.RLock(ctx, "reader-a")
	require.ErrorIs(t, err, ErrLeaseExpired)
	after, err := readerA.Inspect(ctx)
	require.NoError(t, err)
	require.False(t, after.Information.LeaseExpireTime.After(beforeExpire), "读路径在恰好到期时把租约从 %s 推到了 %s", beforeExpire, after.Information.LeaseExpireTime)
	require.Equal(t, beforeVersion, after.Information.Version)
	require.Equal(t, depthA, after.Information.ReadDepth("reader-a"))
	require.Equal(t, depthB, after.Information.ReadDepth("reader-b"))
	t.Logf("renewed past version %d without lock-lost; exact expiry %s was not extended", joinedVersion, beforeExpire)
}

// TestRUnlockWhileOtherReaderRemainsDoesNotReportLockLost 覆盖正常释放：
// A、B 同时持有读锁，A 释放自己的最后一层时不得触发 OnLockLost，B 仍持有且续租继续。
func TestRUnlockWhileOtherReaderRemainsDoesNotReportLockLost(t *testing.T) {
	store := newMockSubscribeStorage()
	clock := newMockSubscribeTimeProvider(time.Now())
	const lockID = "rw-runlock-lost"
	var lost atomic.Bool
	readerA := newSharedReadLock(t, store, clock, lockID, &lost)
	readerB := newSharedReadLock(t, store, clock, lockID, nil)
	ctx := context.Background()

	require.NoError(t, readerA.RLock(ctx, "reader-a"))
	require.NoError(t, readerB.RLock(ctx, "reader-b"))
	joined, err := readerA.Inspect(ctx)
	require.NoError(t, err)
	joinedVersion := joined.Information.Version

	renewed := false
	require.Eventually(t, func() bool {
		if lost.Load() {
			return true
		}
		status, err := readerB.Inspect(ctx)
		if err != nil {
			return false
		}
		if status.Information.Version > joinedVersion &&
			status.Information.ReadDepth("reader-a") > 0 &&
			status.Information.ReadDepth("reader-b") > 0 {
			renewed = true
			return true
		}
		return false
	}, 5*time.Second, 20*time.Millisecond)
	require.False(t, lost.Load(), "释放之前先加入的读者收到了持有丢失")
	require.True(t, renewed, "释放之前没有发生续租")

	require.NoError(t, readerA.RUnLock(ctx, "reader-a"))
	released, err := readerB.Inspect(ctx)
	require.NoError(t, err)
	require.Equal(t, 0, released.Information.ReadDepth("reader-a"))
	require.Greater(t, released.Information.ReadDepth("reader-b"), 0)
	afterUnlock := released.Information.Version

	bRenewed := false
	require.Eventually(t, func() bool {
		if lost.Load() {
			return true
		}
		status, err := readerB.Inspect(ctx)
		if err != nil {
			return false
		}
		if status.Information.Version > afterUnlock &&
			status.Information.ReadDepth("reader-b") > 0 &&
			status.Information.ReadDepth("reader-a") == 0 &&
			status.Held {
			bRenewed = true
			return true
		}
		return false
	}, 5*time.Second, 20*time.Millisecond)
	require.False(t, lost.Load(), "正常 RUnLock 之后收到了持有丢失")
	require.True(t, bRenewed, "其他读者释放后没有继续续租")

	time.Sleep(300 * time.Millisecond)
	require.False(t, lost.Load(), "续租窗口之后正常释放仍被判定为持有丢失")
	still, err := readerB.Inspect(ctx)
	require.NoError(t, err)
	require.Greater(t, still.Information.ReadDepth("reader-b"), 0)
	require.Equal(t, 0, still.Information.ReadDepth("reader-a"))
	require.True(t, still.Held)
	readerB.stopWatchDog(ctx, events.NewEvent(lockID), nil)
}

func newSharedReadLock(t *testing.T, store *mockSubscribeStorage, clock *mockSubscribeTimeProvider, lockID string, lost *atomic.Bool) *StorageLock {
	t.Helper()
	options := NewStorageLockOptionsWithLockId(lockID).
		SetTimeProvider(clock).
		SetLeaseExpireAfter(3*time.Second).
		SetLeaseRefreshInterval(time.Second).
		SetVersionMissRetryInterval(10*time.Millisecond)
	if lost != nil {
		options.SetOnLockLost(func(error) { lost.Store(true) })
	}
	lock, err := NewStorageLockWithOptions(store, options)
	require.NoError(t, err)
	return lock
}
