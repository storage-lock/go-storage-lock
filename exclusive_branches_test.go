package storage_lock

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/storage-lock/go-storage"
	"github.com/stretchr/testify/require"
)

func TestExclusiveBranches(t *testing.T) {
	ctx := context.Background()

	t.Run("first acquire version is 1", func(t *testing.T) {
		store, clock := newBranchClock()
		lock := openBranchLock(t, store, clock, "ex-first", false, 0, nil)
		require.NoError(t, lock.Lock(ctx, "owner"))
		info := lockSnap(t, lock)
		require.Equal(t, storage.Version(1), info.Version)
		require.Equal(t, "owner", info.OwnerId)
		require.Equal(t, 1, info.LockCount)
		require.False(t, info.ReadLock)
		token, err := lock.GetFencingToken(ctx, "owner")
		require.NoError(t, err)
		require.Equal(t, storage.Version(1), token)
	})

	t.Run("reentry deepens one layer", func(t *testing.T) {
		store, clock := newBranchClock()
		lock := openBranchLock(t, store, clock, "ex-reentry", false, 0, nil)
		require.NoError(t, lock.Lock(ctx, "owner"))
		before := lockSnap(t, lock)
		require.NoError(t, lock.Lock(ctx, "owner"))
		after := lockSnap(t, lock)
		require.Equal(t, "owner", after.OwnerId)
		require.Equal(t, before.LockCount+1, after.LockCount)
		require.Greater(t, after.Version, before.Version)
	})

	t.Run("reentry limit leaves the record", func(t *testing.T) {
		store, clock := newBranchClock()
		lock := openBranchLock(t, store, clock, "ex-depth", false, 2, nil)
		require.NoError(t, lock.Lock(ctx, "owner"))
		require.NoError(t, lock.Lock(ctx, "owner"))
		before := lockSnap(t, lock)
		require.ErrorIs(t, lock.Lock(ctx, "owner"), ErrMaxReentrantDepth)
		require.ErrorIs(t, lock.TryLock(ctx, "owner"), ErrMaxReentrantDepth)
		requireStable(t, lock, before)
	})

	t.Run("other exclusive holder is busy", func(t *testing.T) {
		store, clock := newBranchClock()
		holder := openBranchLock(t, store, clock, "ex-busy", false, 0, nil)
		other := openBranchLock(t, store, clock, "ex-busy", false, 0, nil)
		require.NoError(t, holder.Lock(ctx, "holder"))
		before := lockSnap(t, holder)
		require.ErrorIs(t, other.TryLock(ctx, "other"), ErrLockBusy)
		requireStable(t, holder, before)
	})

	t.Run("valid read lock is busy for a writer", func(t *testing.T) {
		store, clock := newBranchClock()
		reader := openBranchLock(t, store, clock, "ex-read-busy", false, 0, nil)
		writer := openBranchLock(t, store, clock, "ex-read-busy", false, 0, nil)
		require.NoError(t, reader.RLock(ctx, "reader"))
		before := lockSnap(t, reader)
		require.ErrorIs(t, writer.TryLock(ctx, "writer"), ErrLockBusy)
		requireStable(t, reader, before, "reader")
	})

	t.Run("equal expiry can be taken over", func(t *testing.T) {
		store, clock := newBranchClock()
		holder := openBranchLock(t, store, clock, "ex-eq", false, 0, nil)
		next := openBranchLock(t, store, clock, "ex-eq", false, 0, nil)
		require.NoError(t, holder.Lock(ctx, "holder"))
		before := lockSnap(t, holder)
		clock.Set(before.LeaseExpireTime)
		require.NoError(t, next.TryLock(ctx, "next"))
		after := lockSnap(t, next)
		require.Equal(t, "next", after.OwnerId)
		require.Greater(t, after.Version, before.Version)
		require.Equal(t, 1, after.LockCount)
	})

	t.Run("later expiry can be taken over", func(t *testing.T) {
		store, clock := newBranchClock()
		holder := openBranchLock(t, store, clock, "ex-late", false, 0, nil)
		next := openBranchLock(t, store, clock, "ex-late", false, 0, nil)
		require.NoError(t, holder.Lock(ctx, "holder"))
		before := lockSnap(t, holder)
		clock.Set(before.LeaseExpireTime.Add(time.Second))
		require.NoError(t, next.TryLock(ctx, "next"))
		after := lockSnap(t, next)
		require.Equal(t, "next", after.OwnerId)
		require.Greater(t, after.Version, before.Version)
	})

	t.Run("future tombstone can be taken over", func(t *testing.T) {
		store, clock := newBranchClock()
		lock := openBranchLock(t, store, clock, "ex-tomb", false, 0, nil)
		now, err := clock.GetTime(ctx)
		require.NoError(t, err)
		store.mu.Lock()
		store.data["ex-tomb"] = &storage.LockInformation{
			LockId:          "ex-tomb",
			OwnerId:         "previous",
			Version:         4,
			LockCount:       0,
			LockBeginTime:   now,
			LeaseExpireTime: now.Add(time.Hour),
		}
		store.mu.Unlock()
		require.NoError(t, lock.Lock(ctx, "next"))
		after := lockSnap(t, lock)
		require.Equal(t, "next", after.OwnerId)
		require.Greater(t, after.Version, storage.Version(4))
		require.Equal(t, 1, after.LockCount)
	})

	t.Run("waiting acquire retries a version miss", func(t *testing.T) {
		base, clock := newBranchClock()
		store := &scriptedCAS{mockSubscribeStorage: base, failCreates: 1, createErr: ErrVersionMiss}
		lock := openBranchLock(t, store, clock, "ex-miss", false, 0, nil)
		require.NoError(t, lock.Lock(ctx, "owner"))
		info := lockSnap(t, lock)
		require.Equal(t, storage.Version(1), info.Version)
		require.Equal(t, "owner", info.OwnerId)
	})

	t.Run("single try folds version miss into busy", func(t *testing.T) {
		base, clock := newBranchClock()
		store := &scriptedCAS{mockSubscribeStorage: base, failCreates: 1, createErr: ErrVersionMiss}
		lock := openBranchLock(t, store, clock, "ex-try-miss", false, 0, nil)
		require.ErrorIs(t, lock.TryLock(ctx, "owner"), ErrLockBusy)
		requireAbsent(t, base, "ex-try-miss")
	})

	t.Run("single try folds already exists into busy", func(t *testing.T) {
		base, clock := newBranchClock()
		store := &scriptedCAS{mockSubscribeStorage: base, failCreates: 1, createErr: ErrLockAlreadyExists}
		lock := openBranchLock(t, store, clock, "ex-try-exists", false, 0, nil)
		require.ErrorIs(t, lock.TryLock(ctx, "owner"), ErrLockBusy)
		requireAbsent(t, base, "ex-try-exists")
	})

	t.Run("waiting acquire retries already exists", func(t *testing.T) {
		base, clock := newBranchClock()
		store := &scriptedCAS{mockSubscribeStorage: base, failCreates: 1, createErr: ErrLockAlreadyExists}
		lock := openBranchLock(t, store, clock, "ex-retry-exists", false, 0, nil)
		require.NoError(t, lock.Lock(ctx, "owner"))
		require.Equal(t, storage.Version(1), lockSnap(t, lock).Version)
	})

	t.Run("reentry version miss is retried by lock and busy for try", func(t *testing.T) {
		base, clock := newBranchClock()
		store := &scriptedCAS{mockSubscribeStorage: base}
		lock := openBranchLock(t, store, clock, "ex-re-miss", false, 0, nil)
		tryLock := openBranchLock(t, store, clock, "ex-re-miss", false, 0, nil)
		require.NoError(t, lock.Lock(ctx, "owner"))
		before := lockSnap(t, lock)
		store.gate.Lock()
		store.failUpdates = 1
		store.updateErr = ErrVersionMiss
		store.gate.Unlock()
		require.ErrorIs(t, tryLock.TryLock(ctx, "owner"), ErrLockBusy)
		requireStable(t, lock, before)
		require.NoError(t, lock.Lock(ctx, "owner"))
		after := lockSnap(t, lock)
		require.Equal(t, before.LockCount+1, after.LockCount)
		require.Equal(t, "owner", after.OwnerId)
		require.Greater(t, after.Version, before.Version)
	})

	t.Run("empty owner and canceled context do not write", func(t *testing.T) {
		store, clock := newBranchClock()
		lock := openBranchLock(t, store, clock, "ex-bad-call", false, 0, nil)
		require.ErrorIs(t, lock.Lock(ctx, ""), ErrOwnerIdEmpty)
		require.ErrorIs(t, lock.TryLock(ctx, ""), ErrOwnerIdEmpty)
		require.ErrorIs(t, lock.UnLock(ctx, ""), ErrOwnerIdEmpty)
		canceled, cancel := context.WithCancel(ctx)
		cancel()
		require.ErrorIs(t, lock.Lock(canceled, "owner"), context.Canceled)
		require.ErrorIs(t, lock.TryLock(canceled, "owner"), context.Canceled)
		require.ErrorIs(t, lock.UnLock(canceled, "owner"), context.Canceled)
		requireAbsent(t, store, "ex-bad-call")
	})

	t.Run("non owner and over release leave the record", func(t *testing.T) {
		store, clock := newBranchClock()
		lock := openBranchLock(t, store, clock, "ex-release-err", false, 0, nil)
		other := openBranchLock(t, store, clock, "ex-release-err", false, 0, nil)
		require.NoError(t, lock.Lock(ctx, "owner"))
		before := lockSnap(t, lock)
		require.ErrorIs(t, other.UnLock(ctx, "other"), ErrLockNotBelongYou)
		requireStable(t, lock, before)
		require.NoError(t, lock.UnLock(ctx, "owner"))
		tomb := lockSnap(t, lock)
		require.Equal(t, 0, tomb.LockCount)
		require.Greater(t, tomb.Version, before.Version)
		require.ErrorIs(t, lock.UnLock(ctx, "owner"), ErrLockNotFound)
		requireStable(t, lock, tomb)
	})

	t.Run("one reentry layer keeps renewal and the last layer does not report loss", func(t *testing.T) {
		store, clock := newBranchClock()
		var lost atomic.Bool
		lock := openBranchLock(t, store, clock, "ex-watch", true, 0, &lost)
		next := openBranchLock(t, store, clock, "ex-watch", false, 0, nil)
		require.NoError(t, lock.Lock(ctx, "owner"))
		require.NoError(t, lock.Lock(ctx, "owner"))
		held := lockSnap(t, lock)
		require.Equal(t, 2, held.LockCount)
		require.Eventually(t, func() bool {
			if lost.Load() {
				return true
			}
			return lockSnap(t, lock).Version > held.Version && lockSnap(t, lock).LockCount == 2
		}, 5*time.Second, 20*time.Millisecond)
		require.False(t, lost.Load())
		require.NoError(t, lock.UnLock(ctx, "owner"))
		partial := lockSnap(t, lock)
		require.Equal(t, 1, partial.LockCount)
		require.Equal(t, "owner", partial.OwnerId)
		require.False(t, lost.Load())
		require.Eventually(t, func() bool {
			if lost.Load() {
				return true
			}
			info := lockSnap(t, lock)
			return info.Version > partial.Version && info.LockCount == 1 && info.OwnerId == "owner"
		}, 5*time.Second, 20*time.Millisecond)
		require.False(t, lost.Load(), "释放一层重入后收到了持有丢失")

		beforeRelease := lockSnap(t, lock)
		require.NoError(t, lock.UnLock(ctx, "owner"))
		tomb := lockSnap(t, lock)
		require.Equal(t, 0, tomb.LockCount)
		require.Greater(t, tomb.Version, beforeRelease.Version)
		time.Sleep(1200 * time.Millisecond)
		require.False(t, lost.Load(), "正常独占释放收到了持有丢失")
		require.NoError(t, next.Lock(ctx, "next"))
		again := lockSnap(t, next)
		require.Greater(t, again.Version, tomb.Version)
		require.NotEqual(t, storage.Version(1), again.Version)
		require.Equal(t, "next", again.OwnerId)
	})
}
