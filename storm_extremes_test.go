package storage_lock

import (
	"context"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/storage-lock/go-storage"
	"github.com/stretchr/testify/require"
)

// 现有 TestFairStorageLock_Stress 只看错误码，不统计临界区重叠。
// 这里的次数固定，对错由临界区计数和存储里的版本决定，不靠睡眠分出先后。
const (
	stormCallers = 8
	stormRounds  = 20
)

func TestExclusiveStorm(t *testing.T) {
	store := newMockSubscribeStorage()
	clock := newMockSubscribeTimeProvider(time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC))
	const lockID = "storm-exclusive"
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	locks := make([]*StorageLock, stormCallers)
	for i := 0; i < stormCallers; i++ {
		locks[i] = stormClient(t, store, clock, lockID)
	}

	var current atomic.Int32
	var maxCritical atomic.Int32
	var listMu sync.Mutex
	versions := make([]storage.Version, 0, stormCallers*stormRounds)
	var errMu sync.Mutex
	var problems []string
	note := func(format string, args ...any) {
		errMu.Lock()
		problems = append(problems, fmt.Sprintf(format, args...))
		errMu.Unlock()
	}

	var wg sync.WaitGroup
	for i := 0; i < stormCallers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			owner := fmt.Sprintf("caller-%d", i)
			for round := 0; round < stormRounds; round++ {
				if err := locks[i].Lock(ctx, owner); err != nil {
					note("lock %s round %d: %v", owner, round, err)
					return
				}
				// 计数在公开 Lock 成功之后、UnLock 之前保持，其它调用方若也进来就能看见重叠。
				holders := current.Add(1)
				for {
					seen := maxCritical.Load()
					if holders <= seen || maxCritical.CompareAndSwap(seen, holders) {
						break
					}
				}
				if holders > 1 {
					note("critical section overlapped: %d", holders)
				}
				status, err := locks[i].Inspect(ctx)
				if err != nil {
					note("inspect %s: %v", owner, err)
				} else if status.Information.OwnerId != owner || status.Information.LockCount <= 0 {
					note("holder %s saw owner %s count %d", owner, status.Information.OwnerId, status.Information.LockCount)
				} else {
					listMu.Lock()
					versions = append(versions, status.Information.Version)
					listMu.Unlock()
				}
				current.Add(-1)
				if err := locks[i].UnLock(ctx, owner); err != nil {
					note("unlock %s round %d: %v", owner, round, err)
					return
				}
			}
		}(i)
	}
	wg.Wait()
	require.Empty(t, problems)
	require.Equal(t, int32(1), maxCritical.Load())
	require.Len(t, versions, stormCallers*stormRounds)
	for i := 1; i < len(versions); i++ {
		require.Greater(t, versions[i], versions[i-1], "fencing version went backwards at %d", i)
	}
	fmt.Printf("PASS storm exclusive max-in-critical=%d acquires=%d versions-strict\n", maxCritical.Load(), len(versions))

	holder := stormClient(t, store, clock, lockID)
	require.NoError(t, holder.Lock(ctx, "holder"))
	before := lockSnap(t, holder)
	outsiders := make([]*StorageLock, stormCallers)
	for i := 0; i < stormCallers; i++ {
		outsiders[i] = stormClient(t, store, clock, lockID)
	}
	var tryWg sync.WaitGroup
	stolen := 0
	var stolenMu sync.Mutex
	for i := 0; i < stormCallers; i++ {
		tryWg.Add(1)
		go func(i int) {
			defer tryWg.Done()
			owner := fmt.Sprintf("outsider-%d", i)
			err := outsiders[i].TryLock(ctx, owner)
			if err == nil {
				stolenMu.Lock()
				stolen++
				stolenMu.Unlock()
				_ = outsiders[i].UnLock(ctx, owner)
			}
		}(i)
	}
	tryWg.Wait()
	after := lockSnap(t, holder)
	require.Zero(t, stolen)
	require.Equal(t, before.Version, after.Version)
	require.Equal(t, before.OwnerId, after.OwnerId)
	require.Equal(t, before.LockCount, after.LockCount)
	require.True(t, before.LeaseExpireTime.Equal(after.LeaseExpireTime))
	require.NoError(t, holder.UnLock(ctx, "holder"))
	fmt.Println("PASS storm failed acquire left holder unchanged")

	released := lockSnap(t, holder)
	require.Equal(t, 0, released.LockCount)
	require.False(t, released.LockCount > 0 && clockNow(t, clock).Before(released.LeaseExpireTime))
	fmt.Println("PASS storm released")
}

func TestExtremeInputs(t *testing.T) {
	ctx := context.Background()

	t.Run("readers and writer", func(t *testing.T) {
		store := newMockSubscribeStorage()
		clock := newMockSubscribeTimeProvider(time.Date(2026, 3, 2, 1, 0, 0, 0, time.UTC))
		const lockID = "extreme-rw"
		runCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		readerA := stormClient(t, store, clock, lockID)
		readerB := stormClient(t, store, clock, lockID)
		require.NoError(t, readerA.RLock(ctx, "reader-a"))
		require.NoError(t, readerB.RLock(ctx, "reader-b"))
		shared := lockSnap(t, readerA)
		require.Equal(t, 1, shared.ReadDepth("reader-a"))
		require.Equal(t, 1, shared.ReadDepth("reader-b"))
		require.Greater(t, shared.LockCount, 1)
		writer := stormClient(t, store, clock, lockID)
		require.ErrorIs(t, writer.TryLock(ctx, "writer"), ErrLockBusy)
		unchanged := lockSnap(t, readerA)
		require.Equal(t, shared.Version, unchanged.Version)
		require.Equal(t, shared.OwnerId, unchanged.OwnerId)
		require.NoError(t, readerA.RUnLock(ctx, "reader-a"))
		require.NoError(t, readerB.RUnLock(ctx, "reader-b"))

		const readers = 4
		const rounds = 12
		clients := make([]*StorageLock, readers+1)
		for i := 0; i < readers; i++ {
			clients[i] = stormClient(t, store, clock, lockID)
		}
		clients[readers] = stormClient(t, store, clock, lockID)
		var reading atomic.Int32
		var writing atomic.Bool
		var errMu sync.Mutex
		var problems []string
		note := func(format string, args ...any) {
			errMu.Lock()
			problems = append(problems, fmt.Sprintf(format, args...))
			errMu.Unlock()
		}
		var wg sync.WaitGroup
		for i := 0; i < readers; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				owner := fmt.Sprintf("reader-%d", i)
				for round := 0; round < rounds; round++ {
					if err := clients[i].RLock(runCtx, owner); err != nil {
						note("rlock %s: %v", owner, err)
						return
					}
					reading.Add(1)
					// 固定空转，不靠睡眠决定谁先拿到锁。窗口留着，写者若同时进来就能被看见。
					for spin := 0; spin < 4096; spin++ {
						if writing.Load() {
							note("reader %s entered while writer held", owner)
							break
						}
					}
					reading.Add(-1)
					if err := clients[i].RUnLock(runCtx, owner); err != nil {
						note("runlock %s: %v", owner, err)
						return
					}
				}
			}(i)
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			for round := 0; round < rounds; round++ {
				if err := clients[readers].Lock(runCtx, "writer"); err != nil {
					note("wlock: %v", err)
					return
				}
				if !writing.CompareAndSwap(false, true) {
					note("two writers entered")
				}
				for spin := 0; spin < 4096; spin++ {
					if n := reading.Load(); n > 0 {
						note("writer entered while readers=%d", n)
						break
					}
				}
				writing.Store(false)
				if err := clients[readers].UnLock(runCtx, "writer"); err != nil {
					note("wunlock: %v", err)
					return
				}
			}
		}()
		wg.Wait()
		require.Empty(t, problems)
		left := lockSnap(t, writer)
		require.Equal(t, 0, left.LockCount)
		fmt.Println("PASS readers overlap and writer stays exclusive")
	})

	t.Run("fair head", func(t *testing.T) {
		store := newMockSubscribeStorage()
		clock := newMockSubscribeTimeProvider(time.Date(2026, 3, 2, 2, 0, 0, 0, time.UTC))
		now, err := clock.GetTime(ctx)
		require.NoError(t, err)
		const lockID = "extreme-fair"
		store.data[lockID] = &storage.LockInformation{
			LockId:          lockID,
			OwnerId:         "previous",
			Version:         3,
			LockCount:       0,
			LeaseExpireTime: now.Add(-time.Second),
			Waiters: []storage.LockWaiter{
				{OwnerId: "live", Mode: waiterModeWrite, Since: now, Until: now.Add(time.Hour)},
				{OwnerId: "later", Mode: waiterModeWrite, Since: now, Until: now.Add(time.Hour)},
			},
		}
		later := fairClient(t, store, clock, lockID)
		require.ErrorIs(t, later.TryLock(ctx, "later"), ErrNotYourTurn)
		blocked := lockSnap(t, later)
		require.NotEqual(t, "later", blocked.OwnerId)
		require.Equal(t, 0, blocked.LockCount)
		require.Equal(t, "live", blocked.Waiters[0].OwnerId)
		fmt.Println("PASS fair unexpired head rejects the later caller")

		store.data[lockID] = &storage.LockInformation{
			LockId:          lockID,
			OwnerId:         "previous",
			Version:         blocked.Version,
			LockCount:       0,
			LeaseExpireTime: now.Add(-time.Second),
			Waiters: []storage.LockWaiter{
				{OwnerId: "dead", Mode: waiterModeWrite, Since: now.Add(-time.Minute), Until: now.Add(-time.Second)},
				{OwnerId: "live", Mode: waiterModeWrite, Since: now, Until: now.Add(time.Hour)},
			},
		}
		live := fairClient(t, store, clock, lockID)
		require.NoError(t, live.Lock(ctx, "live"))
		owned := lockSnap(t, live)
		require.Equal(t, "live", owned.OwnerId)
		require.Greater(t, owned.LockCount, 0)
		for _, waiter := range owned.Waiters {
			require.NotEqual(t, "dead", waiter.OwnerId)
		}
		require.NoError(t, live.UnLock(ctx, "live"))
		fmt.Println("PASS expired fair head yields to the next")

		const legacyID = "extreme-legacy"
		store.data[legacyID] = &storage.LockInformation{
			LockId:          legacyID,
			OwnerId:         "previous",
			Version:         6,
			LockCount:       0,
			LeaseExpireTime: now.Add(-time.Second),
			Waiters: []storage.LockWaiter{
				{OwnerId: "legacy", Mode: waiterModeWrite, Since: now.Add(-time.Minute)},
				{OwnerId: "dead", Mode: waiterModeWrite, Since: now, Until: now.Add(-time.Second)},
			},
		}
		cutter := fairClient(t, store, clock, legacyID)
		require.ErrorIs(t, cutter.TryLock(ctx, "cutter"), ErrNotYourTurn)
		kept := lockSnap(t, cutter)
		require.NotEqual(t, "cutter", kept.OwnerId)
		require.Len(t, kept.Waiters, 1)
		require.Equal(t, "legacy", kept.Waiters[0].OwnerId)
		require.True(t, kept.Waiters[0].Until.IsZero())
		legacy := fairClient(t, store, clock, legacyID)
		require.NoError(t, legacy.Lock(ctx, "legacy"))
		require.Equal(t, "legacy", lockSnap(t, legacy).OwnerId)
		require.NoError(t, legacy.UnLock(ctx, "legacy"))
		fmt.Println("PASS zero deadline waiter kept")
	})

	t.Run("batch reject", func(t *testing.T) {
		store := newBatchCASStorage()
		clock := newMockSubscribeTimeProvider(time.Date(2026, 3, 2, 3, 0, 0, 0, time.UTC))
		options := func(id string) *StorageLockOptions {
			return NewStorageLockOptionsWithLockId(id).
				SetTimeProvider(clock).
				SetLeaseExpireAfter(5 * time.Minute).
				SetDisableWatchDog(true)
		}
		first, err := NewStorageLockWithOptions(store, options("extreme-a"))
		require.NoError(t, err)
		second, err := NewStorageLockWithOptions(store, options("extreme-b"))
		require.NoError(t, err)
		multi, err := NewMultiLock("owner", first, second)
		require.NoError(t, err)
		require.NoError(t, multi.Lock(ctx))
		beforeA := mustSnap(t, store, "extreme-a")
		beforeB := mustSnap(t, store, "extreme-b")
		require.Equal(t, "owner", beforeA.OwnerId)
		require.Equal(t, "owner", beforeB.OwnerId)
		require.Greater(t, beforeA.LockCount, 0)
		require.Greater(t, beforeB.LockCount, 0)
		fmt.Println("PASS batch success holds both locks")
		store.failBatches = 1
		require.Error(t, multi.UnLock(ctx))
		afterA := mustSnap(t, store, "extreme-a")
		afterB := mustSnap(t, store, "extreme-b")
		require.Equal(t, beforeA.Version, afterA.Version)
		require.Equal(t, beforeB.Version, afterB.Version)
		require.Equal(t, beforeA.OwnerId, afterA.OwnerId)
		require.Equal(t, beforeB.OwnerId, afterB.OwnerId)
		require.True(t, beforeA.LeaseExpireTime.Equal(afterA.LeaseExpireTime))
		require.True(t, beforeB.LeaseExpireTime.Equal(afterB.LeaseExpireTime))
		fmt.Println("PASS batch reject leaves both locks unchanged")
	})

	t.Run("version exhausted", func(t *testing.T) {
		store := newMockSubscribeStorage()
		clock := newMockSubscribeTimeProvider(time.Date(2026, 3, 2, 4, 0, 0, 0, time.UTC))
		lock := stormClient(t, store, clock, "extreme-version")
		require.NoError(t, lock.Lock(ctx, "owner"))
		store.mu.Lock()
		store.data["extreme-version"].Version = math.MaxUint64
		store.mu.Unlock()
		before := storedSnap(t, store, "extreme-version")
		require.Equal(t, storage.Version(math.MaxUint64), before.Version)
		require.ErrorIs(t, lock.Lock(ctx, "owner"), ErrVersionExhausted)
		require.ErrorIs(t, lock.Renew(ctx, "owner"), ErrVersionExhausted)
		require.ErrorIs(t, lock.UnLock(ctx, "owner"), ErrVersionExhausted)
		got := storedSnap(t, store, "extreme-version")
		requireSameRecord(t, before, got)
		fmt.Println("PASS version exhausted acquire renew unlock do not write")
	})

	t.Run("time source failure", func(t *testing.T) {
		store := newMockSubscribeStorage()
		clock := &storedClock{mockSubscribeTimeProvider: newMockSubscribeTimeProvider(time.Date(2026, 3, 2, 5, 0, 0, 0, time.UTC))}
		lock := openStoredLock(t, store, clock, "extreme-clock", false, nil)
		require.NoError(t, lock.Lock(ctx, "owner"))
		require.NoError(t, lock.Lock(ctx, "owner"))
		before := storedSnap(t, store, "extreme-clock")
		require.Greater(t, before.LockCount, 1)
		clock.fail.Store(true)
		require.ErrorIs(t, lock.Lock(ctx, "owner"), errStoredClock)
		require.ErrorIs(t, lock.Renew(ctx, "owner"), errStoredClock)
		require.ErrorIs(t, lock.UnLock(ctx, "owner"), errStoredClock)
		got := storedSnap(t, store, "extreme-clock")
		requireSameRecord(t, before, got)
		fmt.Println("PASS time source failure acquire renew unlock do not write")
	})
}

func stormClient(t *testing.T, store *mockSubscribeStorage, clock *mockSubscribeTimeProvider, id string) *StorageLock {
	t.Helper()
	options := NewStorageLockOptionsWithLockId(id).
		SetTimeProvider(clock).
		SetLeaseExpireAfter(5 * time.Minute).
		SetDisableWatchDog(true).
		SetVersionMissRetryInterval(time.Millisecond)
	lock, err := NewStorageLockWithOptions(store, options)
	require.NoError(t, err)
	return lock
}

func fairClient(t *testing.T, store *mockSubscribeStorage, clock *mockSubscribeTimeProvider, id string) *StorageLock {
	t.Helper()
	lock := stormClient(t, store, clock, id)
	lock.SetFair(true)
	return lock
}

func clockNow(t *testing.T, clock *mockSubscribeTimeProvider) time.Time {
	t.Helper()
	now, err := clock.GetTime(context.Background())
	require.NoError(t, err)
	return now
}

func requireSameRecord(t *testing.T, before, got storage.LockInformation) {
	t.Helper()
	require.Equal(t, before.Version, got.Version)
	require.Equal(t, before.OwnerId, got.OwnerId)
	require.Equal(t, before.LockCount, got.LockCount)
	require.True(t, before.LeaseExpireTime.Equal(got.LeaseExpireTime))
	require.Equal(t, before.ReadLock, got.ReadLock)
}
