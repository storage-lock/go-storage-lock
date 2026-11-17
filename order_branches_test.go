package storage_lock

import (
	"context"
	"testing"
	"time"

	"github.com/storage-lock/go-storage"
	"github.com/stretchr/testify/require"
)

func TestOrderBranches(t *testing.T) {
	ctx := context.Background()

	t.Run("sole reader upgrade keeps waiters and intent", func(t *testing.T) {
		store, clock := newBranchClock()
		lock := openBranchLock(t, store, clock, "up-keep", false, 0, nil)
		require.NoError(t, lock.RLock(ctx, "reader"))
		require.NoError(t, lock.prepareWrite(ctx, "writer"))
		before := lockSnap(t, lock)
		require.NotEmpty(t, before.WriteIntentOwner)
		require.NotEmpty(t, before.Waiters)
		require.NoError(t, lock.TryUpgrade(ctx, "reader"))
		after := lockSnap(t, lock)
		require.False(t, after.ReadLock)
		require.Equal(t, "reader", after.OwnerId)
		require.Equal(t, 1, after.LockCount)
		require.Equal(t, before.WriteIntentOwner, after.WriteIntentOwner)
		require.Equal(t, len(before.Waiters), len(after.Waiters))
		require.Greater(t, after.Version, before.Version)
	})

	t.Run("upgrade rejects other readers without a write", func(t *testing.T) {
		store, clock := newBranchClock()
		lock := openBranchLock(t, store, clock, "up-others", false, 0, nil)
		other := openBranchLock(t, store, clock, "up-others", false, 0, nil)
		require.NoError(t, lock.RLock(ctx, "reader"))
		require.NoError(t, other.RLock(ctx, "second"))
		before := lockSnap(t, lock)
		err := lock.TryUpgrade(ctx, "reader")
		require.ErrorIs(t, err, ErrUpgradeConflict)
		require.NotErrorIs(t, err, ErrLockBusy)
		requireStable(t, lock, before, "reader", "second")
	})

	t.Run("upgrade rejects a non reader without a write", func(t *testing.T) {
		store, clock := newBranchClock()
		lock := openBranchLock(t, store, clock, "up-stranger", false, 0, nil)
		require.NoError(t, lock.RLock(ctx, "reader"))
		before := lockSnap(t, lock)
		require.ErrorIs(t, lock.TryUpgrade(ctx, "stranger"), ErrLockNotBelongYou)
		requireStable(t, lock, before, "reader")
	})

	t.Run("upgrade rejects an expired read lock without a write", func(t *testing.T) {
		store, clock := newBranchClock()
		lock := openBranchLock(t, store, clock, "up-expired", false, 0, nil)
		require.NoError(t, lock.RLock(ctx, "reader"))
		expire := lockSnap(t, lock).LeaseExpireTime
		clock.Set(expire)
		before := lockSnap(t, lock)
		require.ErrorIs(t, lock.TryUpgrade(ctx, "reader"), ErrLeaseExpired)
		requireStable(t, lock, before, "reader")
	})

	t.Run("upgrade version conflict leaves the read lock", func(t *testing.T) {
		base, clock := newBranchClock()
		store := &scriptedCAS{mockSubscribeStorage: base}
		lock := openBranchLock(t, store, clock, "up-miss", false, 0, nil)
		require.NoError(t, lock.RLock(ctx, "reader"))
		before := lockSnap(t, lock)
		store.gate.Lock()
		store.failUpdates = 1
		store.updateErr = ErrVersionMiss
		store.gate.Unlock()
		require.ErrorIs(t, lock.TryUpgrade(ctx, "reader"), ErrLockBusy)
		requireStable(t, lock, before, "reader")
	})

	t.Run("downgrade above one stays exclusive", func(t *testing.T) {
		store, clock := newBranchClock()
		lock := openBranchLock(t, store, clock, "down-deep", false, 0, nil)
		require.NoError(t, lock.Lock(ctx, "writer"))
		require.NoError(t, lock.Lock(ctx, "writer"))
		before := lockSnap(t, lock)
		require.Greater(t, before.LockCount, 1)
		require.NoError(t, lock.Downgrade(ctx, "writer"))
		after := lockSnap(t, lock)
		require.False(t, after.ReadLock)
		require.Equal(t, "writer", after.OwnerId)
		require.Equal(t, before.LockCount-1, after.LockCount)
		require.Greater(t, after.Version, before.Version)
	})

	t.Run("last exclusive layer downgrades to a read lock", func(t *testing.T) {
		store, clock := newBranchClock()
		lock := openBranchLock(t, store, clock, "down-last", false, 0, nil)
		reader := openBranchLock(t, store, clock, "down-last", false, 0, nil)
		require.NoError(t, lock.Lock(ctx, "writer"))
		require.NoError(t, lock.Downgrade(ctx, "writer"))
		after := lockSnap(t, lock)
		require.True(t, after.ReadLock)
		require.Equal(t, 1, after.ReadDepth("writer"))
		require.NoError(t, reader.TryRLock(ctx, "reader"))
		joined := lockSnap(t, lock)
		require.Equal(t, 1, joined.ReadDepth("reader"))
		require.Equal(t, 1, joined.ReadDepth("writer"))
	})

	t.Run("downgrade keeps intent and blocks a new reader", func(t *testing.T) {
		store, clock := newBranchClock()
		lock := openBranchLock(t, store, clock, "down-intent", false, 0, nil)
		require.NoError(t, lock.Lock(ctx, "writer"))
		require.NoError(t, lock.prepareWrite(ctx, "waiter"))
		require.NoError(t, lock.Downgrade(ctx, "writer"))
		before := lockSnap(t, lock)
		require.True(t, before.ReadLock)
		require.NotEmpty(t, before.WriteIntentOwner)
		require.ErrorIs(t, lock.TryRLock(ctx, "newbie"), ErrUpgradeConflict)
		requireStable(t, lock, before, "writer", "newbie")
	})

	t.Run("release downgrade and upgrade keep the queue in force", func(t *testing.T) {
		store, clock := newBranchClock()
		lock := openBranchLock(t, store, clock, "queue-survives", false, 0, nil)
		fair := branchFair(t, store, clock, "queue-survives")
		require.NoError(t, lock.RLock(ctx, "reader"))
		require.NoError(t, lock.prepareWrite(ctx, "head"))
		require.NoError(t, lock.RUnLock(ctx, "reader"))
		released := lockSnap(t, lock)
		require.Equal(t, 0, released.LockCount)
		require.Equal(t, "head", released.WriteIntentOwner)
		require.ErrorIs(t, lock.TryRLock(ctx, "newbie"), ErrUpgradeConflict)
		requireStable(t, lock, released)
		require.ErrorIs(t, fair.TryLock(ctx, "cutter"), ErrNotYourTurn)
		require.NotErrorIs(t, fair.TryLock(ctx, "cutter"), ErrLockBusy)
		requireStable(t, lock, released)

		store.mu.Lock()
		store.data["queue-survives"].WriteIntentOwner = ""
		store.mu.Unlock()
		queued := lockSnap(t, lock)
		require.ErrorIs(t, lock.TryRLock(ctx, "newbie"), ErrNotYourTurn)
		requireStable(t, lock, queued)

		writer := openBranchLock(t, store, clock, "down-queue", false, 0, nil)
		downFair := branchFair(t, store, clock, "down-queue")
		require.NoError(t, writer.Lock(ctx, "writer"))
		require.NoError(t, writer.prepareWrite(ctx, "head"))
		require.NoError(t, writer.Downgrade(ctx, "writer"))
		downgraded := lockSnap(t, writer)
		clock.Set(downgraded.LeaseExpireTime.Add(time.Second))
		expiredDown := lockSnap(t, writer)
		require.ErrorIs(t, downFair.TryLock(ctx, "cutter"), ErrNotYourTurn)
		requireStable(t, writer, expiredDown, "writer")

		up := openBranchLock(t, store, clock, "up-queue", false, 0, nil)
		upFair := branchFair(t, store, clock, "up-queue")
		require.NoError(t, up.RLock(ctx, "reader"))
		require.NoError(t, up.prepareWrite(ctx, "head"))
		require.NoError(t, up.TryUpgrade(ctx, "reader"))
		upgraded := lockSnap(t, up)
		clock.Set(upgraded.LeaseExpireTime.Add(time.Second))
		expiredUp := lockSnap(t, up)
		require.ErrorIs(t, upFair.TryLock(ctx, "cutter"), ErrNotYourTurn)
		requireStable(t, up, expiredUp)
	})

	t.Run("fair try when head is self takes an expired lock", func(t *testing.T) {
		store, clock := newBranchClock()
		now, err := clock.GetTime(ctx)
		require.NoError(t, err)
		const id = "fair-head-self"
		store.mu.Lock()
		store.data[id] = &storage.LockInformation{
			LockId:          id,
			OwnerId:         "previous",
			Version:         8,
			LockCount:       0,
			LeaseExpireTime: now.Add(time.Hour),
			Waiters:         []storage.LockWaiter{{OwnerId: "head", Mode: waiterModeWrite, Since: now}},
		}
		store.mu.Unlock()
		fair := branchFair(t, store, clock, id)
		require.NoError(t, fair.TryLock(ctx, "head"))
		after := lockSnap(t, fair.StorageLock())
		require.Equal(t, "head", after.OwnerId)
		require.Greater(t, after.Version, storage.Version(8))
		require.Equal(t, 1, after.LockCount)
	})

	t.Run("ordered lock gives the earlier waiter the lock", func(t *testing.T) {
		store, clock := newBranchClock()
		opt := NewStorageLockOptionsWithLockId("ordered-fifo").
			SetTimeProvider(clock).
			SetDisableWatchDog(true).
			SetLeaseExpireAfter(5 * time.Minute).
			SetVersionMissRetryInterval(15 * time.Millisecond)
		ordered, err := NewLocalOrderedLockWithOptions(store, opt)
		require.NoError(t, err)
		require.NoError(t, ordered.Lock(ctx, "holder"))

		earlyReady := make(chan struct{})
		releaseEarly := make(chan struct{})
		earlyDone := make(chan error, 1)
		lateDone := make(chan error, 1)
		go func() {
			if err := ordered.Lock(ctx, "early"); err != nil {
				earlyDone <- err
				return
			}
			close(earlyReady)
			<-releaseEarly
			earlyDone <- ordered.UnLock(ctx, "early")
		}()
		require.Eventually(t, func() bool { return ordered.QueueLength() == 1 }, time.Second, 5*time.Millisecond)
		go func() {
			if err := ordered.Lock(ctx, "late"); err != nil {
				lateDone <- err
				return
			}
			lateDone <- ordered.UnLock(ctx, "late")
		}()
		require.Eventually(t, func() bool { return ordered.QueueLength() == 2 }, time.Second, 5*time.Millisecond)
		require.NoError(t, ordered.UnLock(ctx, "holder"))
		select {
		case <-earlyReady:
		case <-time.After(2 * time.Second):
			t.Fatal("先排队者没有拿到锁")
		}
		status, err := ordered.GetStorageLock().Inspect(ctx)
		require.NoError(t, err)
		require.Equal(t, "early", status.Information.OwnerId)
		close(releaseEarly)
		require.NoError(t, <-earlyDone)
		require.NoError(t, <-lateDone)
	})

	t.Run("canceled later waiter does not change the holder", func(t *testing.T) {
		store, clock := newBranchClock()
		opt := NewStorageLockOptionsWithLockId("ordered-cancel").
			SetTimeProvider(clock).
			SetDisableWatchDog(true).
			SetLeaseExpireAfter(5 * time.Minute).
			SetVersionMissRetryInterval(15 * time.Millisecond)
		ordered, err := NewLocalOrderedLockWithOptions(store, opt)
		require.NoError(t, err)
		require.NoError(t, ordered.Lock(ctx, "holder"))
		before := lockSnap(t, ordered.GetStorageLock())

		earlyDone := make(chan error, 1)
		go func() { earlyDone <- ordered.Lock(ctx, "early") }()
		require.Eventually(t, func() bool { return ordered.QueueLength() == 1 }, time.Second, 5*time.Millisecond)

		lateCtx, cancel := context.WithTimeout(ctx, 40*time.Millisecond)
		defer cancel()
		require.Error(t, ordered.Lock(lateCtx, "late"))
		requireStable(t, ordered.GetStorageLock(), before)
		require.Equal(t, 1, ordered.QueueLength())

		require.NoError(t, ordered.UnLock(ctx, "holder"))
		require.NoError(t, <-earlyDone)
		held := lockSnap(t, ordered.GetStorageLock())
		require.Equal(t, "early", held.OwnerId)
		require.NotEqual(t, "late", held.OwnerId)
		require.NoError(t, ordered.UnLock(ctx, "early"))
	})

	t.Run("multi lock holds every key once", func(t *testing.T) {
		store := newBatchCASStorage()
		clock := newMockSubscribeTimeProvider(time.Now())
		a := orderLock(t, store, clock, "multi-a", 0)
		b := orderLock(t, store, clock, "multi-b", 0)
		multi, err := NewMultiLock("owner", a, b)
		require.NoError(t, err)
		require.NoError(t, multi.Lock(ctx))
		require.Equal(t, 2, multi.HeldCount())
		left := lockSnap(t, a)
		right := lockSnap(t, b)
		require.Equal(t, "owner", left.OwnerId)
		require.Equal(t, "owner", right.OwnerId)
		require.Equal(t, storage.Version(1), left.Version)
		require.Equal(t, storage.Version(1), right.Version)

		require.NoError(t, multi.Lock(ctx))
		require.Equal(t, left.Version, lockSnap(t, a).Version)
		require.Equal(t, right.Version, lockSnap(t, b).Version)
		require.Equal(t, "owner", lockSnap(t, a).OwnerId)
		require.Equal(t, "owner", lockSnap(t, b).OwnerId)
	})

	t.Run("multi unlock when not held leaves the record", func(t *testing.T) {
		store := newBatchCASStorage()
		clock := newMockSubscribeTimeProvider(time.Now())
		now, err := clock.GetTime(ctx)
		require.NoError(t, err)
		const id = "multi-idle"
		store.mu.Lock()
		store.data[id] = &storage.LockInformation{
			LockId: id, OwnerId: "previous", Version: 3, LockCount: 1,
			LeaseExpireTime: now.Add(time.Hour),
		}
		store.mu.Unlock()
		lock := orderLock(t, store, clock, id, 0)
		other := orderLock(t, store, clock, "multi-idle-b", 0)
		multi, err := NewMultiLock("owner", lock, other)
		require.NoError(t, err)
		before := lockSnap(t, lock)
		require.NoError(t, multi.UnLock(ctx))
		requireStable(t, lock, before)
		requireAbsent(t, store.mockSubscribeStorage, "multi-idle-b")
	})

	t.Run("multi rejects a busy key without holding the sibling", func(t *testing.T) {
		store := newBatchCASStorage()
		clock := newMockSubscribeTimeProvider(time.Now())
		held := orderLock(t, store, clock, "multi-busy", 0)
		fresh := orderLock(t, store, clock, "multi-fresh", 0)
		require.NoError(t, held.Lock(ctx, "other"))
		before := lockSnap(t, held)
		multi, err := NewMultiLock("owner", held, fresh)
		require.NoError(t, err)
		require.ErrorIs(t, multi.Lock(ctx), ErrLockBusy)
		require.Equal(t, 0, multi.HeldCount())
		requireStable(t, held, before)
		requireAbsent(t, store.mockSubscribeStorage, "multi-fresh")
	})

	t.Run("multi rejects when it is not the queue head", func(t *testing.T) {
		store := newBatchCASStorage()
		clock := newMockSubscribeTimeProvider(time.Now())
		ctx := context.Background()
		now, err := clock.GetTime(ctx)
		require.NoError(t, err)
		const queuedID = "multi-queued"
		store.mu.Lock()
		store.data[queuedID] = &storage.LockInformation{
			LockId: queuedID, OwnerId: "previous", Version: 7, LockCount: 0,
			LeaseExpireTime: now.Add(-time.Second),
			Waiters:         []storage.LockWaiter{{OwnerId: "head", Mode: waiterModeWrite, Since: now}},
		}
		store.mu.Unlock()
		queued := orderLock(t, store, clock, queuedID, 0)
		other := orderLock(t, store, clock, "multi-queued-b", 0)
		multi, err := NewMultiLock("cutter", queued, other)
		require.NoError(t, err)
		require.ErrorIs(t, multi.Lock(ctx), ErrNotYourTurn)
		after := lockSnap(t, queued)
		require.Equal(t, storage.Version(7), after.Version)
		require.Equal(t, "previous", after.OwnerId)
		requireAbsent(t, store.mockSubscribeStorage, "multi-queued-b")
	})

	t.Run("multi rejects max reentry without a write", func(t *testing.T) {
		store := newBatchCASStorage()
		clock := newMockSubscribeTimeProvider(time.Now())
		a := orderLock(t, store, clock, "multi-depth-a", 1)
		b := orderLock(t, store, clock, "multi-depth-b", 1)
		require.NoError(t, a.Lock(ctx, "owner"))
		before := lockSnap(t, a)
		multi, err := NewMultiLock("owner", a, b)
		require.NoError(t, err)
		require.ErrorIs(t, multi.Lock(ctx), ErrMaxReentrantDepth)
		require.Equal(t, 0, multi.HeldCount())
		requireStable(t, a, before)
		requireAbsent(t, store.mockSubscribeStorage, "multi-depth-b")
	})

	t.Run("multi batch version conflict holds nothing", func(t *testing.T) {
		store := newBatchCASStorage()
		store.failBatches = 1
		clock := newMockSubscribeTimeProvider(time.Now())
		a := orderLock(t, store, clock, "multi-miss-a", 0)
		b := orderLock(t, store, clock, "multi-miss-b", 0)
		multi, err := NewMultiLock("owner", a, b)
		require.NoError(t, err)
		require.ErrorIs(t, multi.Lock(ctx), ErrVersionMiss)
		require.Equal(t, 0, multi.HeldCount())
		requireAbsent(t, store.mockSubscribeStorage, "multi-miss-a")
		requireAbsent(t, store.mockSubscribeStorage, "multi-miss-b")
	})
}

func branchFair(t *testing.T, store storage.Storage, clock *mockSubscribeTimeProvider, id string) *FairLock {
	t.Helper()
	opt := NewStorageLockOptionsWithLockId(id).
		SetTimeProvider(clock).
		SetDisableWatchDog(true).
		SetLeaseExpireAfter(5 * time.Minute).
		SetVersionMissRetryInterval(5 * time.Millisecond)
	fair, err := NewFairLockWithOptions(store, opt)
	require.NoError(t, err)
	return fair
}

func orderLock(t *testing.T, store storage.Storage, clock *mockSubscribeTimeProvider, id string, depth int) *StorageLock {
	t.Helper()
	return openBranchLock(t, store, clock, id, false, depth, nil)
}
