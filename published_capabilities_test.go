package storage_lock

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/storage-lock/go-storage"
	"github.com/stretchr/testify/require"
)

func TestPublishedCapabilities(t *testing.T) {
	ctx := context.Background()

	t.Run("metadata stays with the holder", func(t *testing.T) {
		store := newMockSubscribeStorage()
		clock := newMockSubscribeTimeProvider(time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC))
		options := NewStorageLockOptionsWithLockId("published-meta").
			SetTimeProvider(clock).
			SetLeaseExpireAfter(5 * time.Minute).
			SetDisableWatchDog(true).
			SetVersionMissRetryInterval(5 * time.Millisecond)
		held, err := NewStorageLockWithOptions(store, options)
		require.NoError(t, err)
		lock := &StorageLockWithMetadata{StorageLock: held}
		extra := map[string]string{"taskId": "task-1", "region": "west"}
		require.NoError(t, lock.LockWithMetadata(ctx, "owner", &LockMetadata{Extra: extra}))

		require.NoError(t, lock.Lock(ctx, "owner"))
		got, err := lock.GetMetadata(ctx)
		require.NoError(t, err)
		require.Equal(t, map[string]string{"taskId": "task-1", "region": "west"}, got.Extra)
		require.Equal(t, 2, got.LockCount)

		before, err := lock.GetMetadata(ctx)
		require.NoError(t, err)
		require.NoError(t, lock.Renew(ctx, "owner"))
		renewed, err := lock.GetMetadata(ctx)
		require.NoError(t, err)
		require.Equal(t, before.Extra, renewed.Extra)
		require.Equal(t, before.LockCount, renewed.LockCount)
		require.Greater(t, renewed.Version, before.Version)
		fmt.Println("PASS metadata kept across reentry and renew")

		otherOptions := NewStorageLockOptionsWithLockId("published-meta").
			SetTimeProvider(clock).
			SetLeaseExpireAfter(5 * time.Minute).
			SetDisableWatchDog(true).
			SetVersionMissRetryInterval(5 * time.Millisecond)
		otherLock, err := NewStorageLockWithOptions(store, otherOptions)
		require.NoError(t, err)
		other := &StorageLockWithMetadata{StorageLock: otherLock}
		busyCtx, cancel := context.WithTimeout(ctx, 40*time.Millisecond)
		defer cancel()
		require.Error(t, other.LockWithMetadata(busyCtx, "stranger", &LockMetadata{Extra: map[string]string{"taskId": "theirs"}}))
		still, err := lock.GetMetadata(ctx)
		require.NoError(t, err)
		require.Equal(t, "owner", still.OwnerId)
		require.Equal(t, map[string]string{"taskId": "task-1", "region": "west"}, still.Extra)
		fmt.Println("PASS other holder cannot replace metadata")
	})

	t.Run("expired fair waiter yields to the next", func(t *testing.T) {
		store := newMockSubscribeStorage()
		clock := newMockSubscribeTimeProvider(time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC))
		now, err := clock.GetTime(ctx)
		require.NoError(t, err)
		const lockID = "published-fair"
		store.data[lockID] = &storage.LockInformation{
			LockId:          lockID,
			OwnerId:         "previous",
			Version:         4,
			LockCount:       0,
			LeaseExpireTime: now.Add(-time.Second),
			Waiters: []storage.LockWaiter{
				{OwnerId: "dead", Mode: waiterModeWrite, Since: now.Add(-time.Minute), Until: now.Add(-time.Second)},
				{OwnerId: "live", Mode: waiterModeWrite, Since: now, Until: now.Add(time.Hour)},
			},
		}
		cutter := fairLock(t, store, clock, lockID)
		cutter.options.DisableWatchDog = true
		require.ErrorIs(t, cutter.TryLock(ctx, "cutter"), ErrNotYourTurn)
		pruned, err := cutter.Inspect(ctx)
		require.NoError(t, err)
		require.Equal(t, []string{"live"}, waiterIDs(pruned.Waiters))

		live := fairLock(t, store, clock, lockID)
		live.options.DisableWatchDog = true
		require.NoError(t, live.Lock(ctx, "live"))
		owned, err := live.GetOwner(ctx)
		require.NoError(t, err)
		require.Equal(t, "live", owned)
		require.NoError(t, live.UnLock(ctx, "live"))
		fmt.Println("PASS expired fair waiter removed and next acquires")
	})

	t.Run("zero deadline waiter stays", func(t *testing.T) {
		store := newMockSubscribeStorage()
		clock := newMockSubscribeTimeProvider(time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC))
		now, err := clock.GetTime(ctx)
		require.NoError(t, err)
		const lockID = "published-legacy"
		store.data[lockID] = &storage.LockInformation{
			LockId:          lockID,
			OwnerId:         "previous",
			Version:         6,
			LockCount:       0,
			LeaseExpireTime: now.Add(-time.Second),
			Waiters: []storage.LockWaiter{
				{OwnerId: "legacy", Mode: waiterModeWrite, Since: now.Add(-time.Minute)},
				{OwnerId: "dead", Mode: waiterModeWrite, Since: now, Until: now.Add(-time.Second)},
			},
		}
		cutter := fairLock(t, store, clock, lockID)
		cutter.options.DisableWatchDog = true
		require.ErrorIs(t, cutter.TryLock(ctx, "cutter"), ErrNotYourTurn)
		status, err := cutter.Inspect(ctx)
		require.NoError(t, err)
		require.Equal(t, []string{"legacy"}, waiterIDs(status.Waiters))
		require.True(t, status.Waiters[0].Until.IsZero())

		legacy := fairLock(t, store, clock, lockID)
		legacy.options.DisableWatchDog = true
		require.NoError(t, legacy.Lock(ctx, "legacy"))
		owned, err := legacy.GetOwner(ctx)
		require.NoError(t, err)
		require.Equal(t, "legacy", owned)
		require.NoError(t, legacy.UnLock(ctx, "legacy"))
		fmt.Println("PASS zero deadline waiter kept")
	})

	t.Run("multi lock is all or nothing", func(t *testing.T) {
		store := newBatchCASStorage()
		clock := newMockSubscribeTimeProvider(time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC))
		options := func(id string) *StorageLockOptions {
			return NewStorageLockOptionsWithLockId(id).
				SetTimeProvider(clock).
				SetLeaseExpireAfter(5 * time.Minute).
				SetDisableWatchDog(true)
		}
		first, err := NewStorageLockWithOptions(store, options("batch-a"))
		require.NoError(t, err)
		second, err := NewStorageLockWithOptions(store, options("batch-b"))
		require.NoError(t, err)
		multi, err := NewMultiLock("owner", first, second)
		require.NoError(t, err)

		store.failBatches = 1
		require.Error(t, multi.Lock(ctx))
		requireAbsent(t, store.mockSubscribeStorage, "batch-a")
		requireAbsent(t, store.mockSubscribeStorage, "batch-b")

		store.failBatches = 0
		require.NoError(t, multi.Lock(ctx))
		heldA := mustSnap(t, store, "batch-a")
		heldB := mustSnap(t, store, "batch-b")
		require.Equal(t, "owner", heldA.OwnerId)
		require.Equal(t, "owner", heldB.OwnerId)
		require.Equal(t, storage.Version(1), heldA.Version)
		require.Equal(t, storage.Version(1), heldB.Version)
		require.Greater(t, heldA.LockCount, 0)
		require.Greater(t, heldB.LockCount, 0)

		store.failBatches = 1
		require.Error(t, multi.UnLock(ctx))
		afterA := mustSnap(t, store, "batch-a")
		afterB := mustSnap(t, store, "batch-b")
		require.Equal(t, heldA.Version, afterA.Version)
		require.Equal(t, heldB.Version, afterB.Version)
		require.Equal(t, heldA.OwnerId, afterA.OwnerId)
		require.Equal(t, heldB.OwnerId, afterB.OwnerId)
		require.True(t, heldA.LeaseExpireTime.Equal(afterA.LeaseExpireTime))
		require.True(t, heldB.LeaseExpireTime.Equal(afterB.LeaseExpireTime))
		fmt.Println("PASS multi lock batch is all or nothing")

		plain := newMockSubscribeStorage()
		left, err := NewStorageLockWithOptions(plain, options("plain-a"))
		require.NoError(t, err)
		right, err := NewStorageLockWithOptions(plain, options("plain-b"))
		require.NoError(t, err)
		_, err = NewMultiLock("owner", left, right)
		require.ErrorIs(t, err, ErrAtomicMultiLockUnsupported)
		requireAbsent(t, plain, "plain-a")
		requireAbsent(t, plain, "plain-b")
		fmt.Println("PASS multi lock without batch writes nothing")
	})
}

func waiterIDs(waiters []storage.LockWaiter) []string {
	ids := make([]string, 0, len(waiters))
	for _, waiter := range waiters {
		ids = append(ids, waiter.OwnerId)
	}
	return ids
}

func mustSnap(t *testing.T, store *batchCASStorage, id string) storage.LockInformation {
	t.Helper()
	raw, err := store.Get(context.Background(), id)
	require.NoError(t, err)
	info, err := storage.LockInformationFromJsonString(raw)
	require.NoError(t, err)
	return *info
}
