package storage_lock

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/storage-lock/go-storage"
	"github.com/stretchr/testify/require"
)

func TestReadBranches(t *testing.T) {
	ctx := context.Background()

	t.Run("nobody held creates a read lock", func(t *testing.T) {
		store, clock := newBranchClock()
		lock := openBranchLock(t, store, clock, "rd-create", false, 0, nil)
		require.NoError(t, lock.RLock(ctx, "reader"))
		info := lockSnap(t, lock)
		require.True(t, info.ReadLock)
		require.Equal(t, storage.Version(1), info.Version)
		require.Equal(t, 1, info.LockCount)
		require.Equal(t, 1, info.ReadDepth("reader"))
	})

	t.Run("another holder can join", func(t *testing.T) {
		store, clock := newBranchClock()
		first := openBranchLock(t, store, clock, "rd-join", false, 0, nil)
		second := openBranchLock(t, store, clock, "rd-join", false, 0, nil)
		require.NoError(t, first.RLock(ctx, "reader-a"))
		require.NoError(t, second.RLock(ctx, "reader-b"))
		info := lockSnap(t, first)
		require.True(t, info.ReadLock)
		require.Equal(t, 1, info.ReadDepth("reader-a"))
		require.Equal(t, 1, info.ReadDepth("reader-b"))
		require.Equal(t, 2, info.LockCount)
	})

	t.Run("reader can reenter until the limit", func(t *testing.T) {
		store, clock := newBranchClock()
		lock := openBranchLock(t, store, clock, "rd-depth", false, 2, nil)
		require.NoError(t, lock.RLock(ctx, "reader"))
		before := lockSnap(t, lock)
		require.NoError(t, lock.RLock(ctx, "reader"))
		deep := lockSnap(t, lock)
		require.Equal(t, before.ReadDepth("reader")+1, deep.ReadDepth("reader"))
		require.Equal(t, 2, deep.LockCount)
		require.ErrorIs(t, lock.RLock(ctx, "reader"), ErrMaxReentrantDepth)
		require.ErrorIs(t, lock.TryRLock(ctx, "reader"), ErrMaxReentrantDepth)
		requireStable(t, lock, deep, "reader")
	})

	t.Run("exact expiry does not extend the lease", func(t *testing.T) {
		store, clock := newBranchClock()
		holder := openBranchLock(t, store, clock, "rd-eq", false, 0, nil)
		other := openBranchLock(t, store, clock, "rd-eq", false, 0, nil)
		require.NoError(t, holder.RLock(ctx, "reader"))
		before := lockSnap(t, holder)
		clock.Set(before.LeaseExpireTime)
		require.ErrorIs(t, other.TryRLock(ctx, "late"), ErrLeaseExpired)
		require.ErrorIs(t, holder.RLock(ctx, "reader"), ErrLeaseExpired)
		requireStable(t, holder, before, "reader")
	})

	t.Run("strictly later time reclaims when nothing is queued", func(t *testing.T) {
		store, clock := newBranchClock()
		holder := openBranchLock(t, store, clock, "rd-after", false, 0, nil)
		next := openBranchLock(t, store, clock, "rd-after", false, 0, nil)
		require.NoError(t, holder.RLock(ctx, "old"))
		before := lockSnap(t, holder)
		clock.Set(before.LeaseExpireTime.Add(time.Second))
		require.NoError(t, next.TryRLock(ctx, "new"))
		after := lockSnap(t, next)
		require.True(t, after.ReadLock)
		require.Equal(t, 1, after.ReadDepth("new"))
		require.Equal(t, 0, after.ReadDepth("old"))
		require.Greater(t, after.Version, before.Version)
	})

	t.Run("tombstone reclaims when nothing is queued", func(t *testing.T) {
		store, clock := newBranchClock()
		lock := openBranchLock(t, store, clock, "rd-tomb", false, 0, nil)
		now, err := clock.GetTime(ctx)
		require.NoError(t, err)
		store.mu.Lock()
		store.data["rd-tomb"] = &storage.LockInformation{
			LockId:          "rd-tomb",
			Version:         3,
			LockCount:       0,
			ReadLock:        true,
			LockBeginTime:   now,
			LeaseExpireTime: now.Add(time.Hour),
		}
		store.mu.Unlock()
		require.NoError(t, lock.TryRLock(ctx, "reader"))
		after := lockSnap(t, lock)
		require.Equal(t, 1, after.ReadDepth("reader"))
		require.Greater(t, after.Version, storage.Version(3))
	})

	t.Run("queued reclaim is rejected without a write", func(t *testing.T) {
		store, clock := newBranchClock()
		lock := openBranchLock(t, store, clock, "rd-reclaim-queue", false, 0, nil)
		now, err := clock.GetTime(ctx)
		require.NoError(t, err)
		store.mu.Lock()
		store.data["rd-reclaim-queue"] = &storage.LockInformation{
			LockId:           "rd-reclaim-queue",
			OwnerId:          "old",
			Version:          6,
			LockCount:        1,
			ReadLock:         true,
			ReadOwners:       map[string]int{"old": 1},
			LockBeginTime:    now.Add(-time.Hour),
			LeaseExpireTime:  now.Add(-time.Second),
			WriteIntentOwner: "writer",
			WriteIntentSince: now.Add(-time.Minute),
		}
		store.mu.Unlock()
		before := lockSnap(t, lock)
		require.ErrorIs(t, lock.TryRLock(ctx, "reader"), ErrUpgradeConflict)
		require.ErrorIs(t, lock.RLock(ctx, "reader"), ErrUpgradeConflict)
		requireStable(t, lock, before, "old")

		store.mu.Lock()
		info := store.data["rd-reclaim-queue"]
		info.WriteIntentOwner = ""
		info.WriteIntentSince = time.Time{}
		info.Waiters = []storage.LockWaiter{{OwnerId: "head", Mode: waiterModeWrite, Since: now}}
		store.mu.Unlock()
		before = lockSnap(t, lock)
		require.ErrorIs(t, lock.TryRLock(ctx, "reader"), ErrNotYourTurn)
		require.ErrorIs(t, lock.RLock(ctx, "reader"), ErrNotYourTurn)
		requireStable(t, lock, before, "old")
	})

	t.Run("active read rejects a new reader but allows reentry", func(t *testing.T) {
		store, clock := newBranchClock()
		holder := openBranchLock(t, store, clock, "rd-active", false, 0, nil)
		newbie := openBranchLock(t, store, clock, "rd-active", false, 0, nil)
		require.NoError(t, holder.RLock(ctx, "reader"))
		now, err := clock.GetTime(ctx)
		require.NoError(t, err)
		store.mu.Lock()
		store.data["rd-active"].WriteIntentOwner = "writer"
		store.data["rd-active"].WriteIntentSince = now
		store.mu.Unlock()
		before := lockSnap(t, holder)
		require.ErrorIs(t, newbie.TryRLock(ctx, "late"), ErrUpgradeConflict)
		require.ErrorIs(t, newbie.RLock(ctx, "late"), ErrUpgradeConflict)
		requireStable(t, holder, before, "reader", "late")
		require.NoError(t, holder.RLock(ctx, "reader"))
		reentered := lockSnap(t, holder)
		require.Equal(t, 2, reentered.ReadDepth("reader"))
		require.Equal(t, "writer", reentered.WriteIntentOwner)

		store.mu.Lock()
		store.data["rd-active"].WriteIntentOwner = ""
		store.data["rd-active"].Waiters = []storage.LockWaiter{{OwnerId: "head", Mode: waiterModeWrite, Since: now}}
		store.mu.Unlock()
		before = lockSnap(t, holder)
		require.ErrorIs(t, newbie.TryRLock(ctx, "late"), ErrNotYourTurn)
		require.ErrorIs(t, newbie.RLock(ctx, "late"), ErrNotYourTurn)
		requireStable(t, holder, before, "reader", "late")
	})

	t.Run("valid exclusive lock rejects a read", func(t *testing.T) {
		store, clock := newBranchClock()
		writer := openBranchLock(t, store, clock, "rd-ex", false, 0, nil)
		reader := openBranchLock(t, store, clock, "rd-ex", false, 0, nil)
		require.NoError(t, writer.Lock(ctx, "writer"))
		before := lockSnap(t, writer)
		require.ErrorIs(t, reader.TryRLock(ctx, "reader"), ErrLockBusy)
		requireStable(t, writer, before)
	})

	t.Run("remaining reentry keeps renewal", func(t *testing.T) {
		store, clock := newBranchClock()
		var lost atomic.Bool
		lock := openBranchLock(t, store, clock, "rd-partial", true, 0, &lost)
		require.NoError(t, lock.RLock(ctx, "reader"))
		require.NoError(t, lock.RLock(ctx, "reader"))
		held := lockSnap(t, lock)
		require.Equal(t, 2, held.ReadDepth("reader"))
		require.Eventually(t, func() bool {
			info := lockSnap(t, lock)
			return info.Version > held.Version && info.ReadDepth("reader") == 2
		}, 5*time.Second, 20*time.Millisecond)
		require.NoError(t, lock.RUnLock(ctx, "reader"))
		partial := lockSnap(t, lock)
		require.Equal(t, 1, partial.ReadDepth("reader"))
		require.False(t, lost.Load())
		require.Eventually(t, func() bool {
			if lost.Load() {
				return true
			}
			info := lockSnap(t, lock)
			return info.Version > partial.Version && info.ReadDepth("reader") == 1
		}, 5*time.Second, 20*time.Millisecond)
		require.False(t, lost.Load(), "还有重入层时释放一层收到了持有丢失")
	})

	t.Run("last layer leaves other holders renewing", func(t *testing.T) {
		store, clock := newBranchClock()
		var lost atomic.Bool
		first := openBranchLock(t, store, clock, "rd-last", true, 0, &lost)
		second := openBranchLock(t, store, clock, "rd-last", true, 0, nil)
		require.NoError(t, first.RLock(ctx, "reader-a"))
		require.NoError(t, second.RLock(ctx, "reader-b"))
		joined := lockSnap(t, first)
		require.Eventually(t, func() bool {
			info := lockSnap(t, second)
			return info.Version > joined.Version && info.ReadDepth("reader-a") > 0 && info.ReadDepth("reader-b") > 0
		}, 5*time.Second, 20*time.Millisecond)
		require.NoError(t, first.RUnLock(ctx, "reader-a"))
		released := lockSnap(t, second)
		require.Equal(t, 0, released.ReadDepth("reader-a"))
		require.Greater(t, released.ReadDepth("reader-b"), 0)
		require.Eventually(t, func() bool {
			if lost.Load() {
				return true
			}
			info := lockSnap(t, second)
			return info.Version > released.Version && info.ReadDepth("reader-b") > 0 && info.ReadDepth("reader-a") == 0
		}, 5*time.Second, 20*time.Millisecond)
		require.False(t, lost.Load(), "放下最后一层后收到了持有丢失")
	})

	t.Run("bad read release leaves the record", func(t *testing.T) {
		store, clock := newBranchClock()
		reader := openBranchLock(t, store, clock, "rd-bad-release", false, 0, nil)
		other := openBranchLock(t, store, clock, "rd-bad-release", false, 0, nil)
		require.ErrorIs(t, reader.RUnLock(ctx, "reader"), ErrLockNotFound)
		requireAbsent(t, store, "rd-bad-release")

		writer := openBranchLock(t, store, clock, "rd-on-ex", false, 0, nil)
		read := openBranchLock(t, store, clock, "rd-on-ex", false, 0, nil)
		require.NoError(t, writer.Lock(ctx, "writer"))
		before := lockSnap(t, writer)
		require.ErrorIs(t, read.RUnLock(ctx, "writer"), ErrLockNotBelongYou)
		require.ErrorIs(t, read.RUnLock(ctx, "reader"), ErrLockNotBelongYou)
		requireStable(t, writer, before)

		require.NoError(t, reader.RLock(ctx, "reader"))
		held := lockSnap(t, reader)
		require.ErrorIs(t, other.RUnLock(ctx, "stranger"), ErrLockNotBelongYou)
		requireStable(t, reader, held, "reader")
	})
}
