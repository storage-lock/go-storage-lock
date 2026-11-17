package storage_lock

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/storage-lock/go-storage"
	"github.com/stretchr/testify/require"
)

// 已有 TestExclusiveStorm 是 8 路。这里单独提高并发，不替换那场。
// 次数固定。对错由持有期间的计数和存储版本决定，不靠睡眠分出先后。
const (
	highCallers = 128
	highReaders = 64
	highWriters = 8
	highRounds  = 4
)

func TestHighExclusive(t *testing.T) {
	store := newMockSubscribeStorage()
	clock := newMockSubscribeTimeProvider(time.Date(2026, 3, 3, 0, 0, 0, 0, time.UTC))
	const lockID = "high-exclusive"
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	locks := make([]*StorageLock, highCallers)
	for i := 0; i < highCallers; i++ {
		locks[i] = highClient(t, store, clock, lockID)
	}

	var current atomic.Int32
	var maxCritical atomic.Int32
	var listMu sync.Mutex
	versions := make([]storage.Version, 0, highCallers*highRounds)
	var errMu sync.Mutex
	var problems []string
	note := func(format string, args ...any) {
		errMu.Lock()
		problems = append(problems, fmt.Sprintf(format, args...))
		errMu.Unlock()
	}

	var wg sync.WaitGroup
	for i := 0; i < highCallers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			owner := fmt.Sprintf("caller-%d", i)
			for round := 0; round < highRounds; round++ {
				if err := locks[i].Lock(ctx, owner); err != nil {
					note("lock %s round %d: %v", owner, round, err)
					return
				}
				// 计数在公开获取成功之后、释放之前保持，不能用另一把互斥把重叠藏起来。
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
				} else if !status.Held || status.Information.OwnerId != owner || status.Information.LockCount <= 0 {
					note("holder %s saw owner %s count %d held %v", owner, status.Information.OwnerId, status.Information.LockCount, status.Held)
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
	require.GreaterOrEqual(t, highCallers, 128)
	require.GreaterOrEqual(t, highRounds, 4)
	require.Len(t, versions, highCallers*highRounds)
	for i := 1; i < len(versions); i++ {
		require.Greater(t, versions[i], versions[i-1], "fencing version went backwards at %d", i)
	}
	fmt.Printf("PASS high exclusive callers=%d rounds=%d max-in-critical=%d acquires=%d versions-strict\n",
		highCallers, highRounds, maxCritical.Load(), len(versions))

	holder := highClient(t, store, clock, lockID)
	require.NoError(t, holder.Lock(ctx, "holder"))
	before := lockSnap(t, holder)
	outsiders := make([]*StorageLock, highCallers)
	for i := 0; i < highCallers; i++ {
		outsiders[i] = highClient(t, store, clock, lockID)
	}
	var tryWg sync.WaitGroup
	stolen := 0
	var stolenMu sync.Mutex
	for i := 0; i < highCallers; i++ {
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
	fmt.Println("PASS high exclusive failed acquire left holder unchanged")

	released, err := holder.Inspect(ctx)
	require.NoError(t, err)
	require.False(t, released.Held)
	require.Equal(t, 0, released.Information.LockCount)
	fmt.Println("PASS high exclusive released")
}

func TestHighReadWrite(t *testing.T) {
	store := newMockSubscribeStorage()
	clock := newMockSubscribeTimeProvider(time.Date(2026, 3, 3, 1, 0, 0, 0, time.UTC))
	const lockID = "high-rw"
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	readers := make([]*StorageLock, highReaders)
	for i := 0; i < highReaders; i++ {
		readers[i] = highClient(t, store, clock, lockID)
	}
	writers := make([]*StorageLock, highWriters)
	for i := 0; i < highWriters; i++ {
		writers[i] = highClient(t, store, clock, lockID)
	}

	var reading atomic.Int32
	var writing atomic.Bool
	var finished atomic.Int32
	var errMu sync.Mutex
	var problems []string
	note := func(format string, args ...any) {
		errMu.Lock()
		problems = append(problems, fmt.Sprintf(format, args...))
		errMu.Unlock()
	}

	var wg sync.WaitGroup
	for i := 0; i < highReaders; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			owner := fmt.Sprintf("reader-%d", i)
			for round := 0; round < highRounds; round++ {
				if err := readers[i].RLock(ctx, owner); err != nil {
					note("rlock %s round %d: %v", owner, round, err)
					return
				}
				reading.Add(1)
				if writing.Load() {
					note("reader %s entered while writer held", owner)
				}
				status, err := readers[i].Inspect(ctx)
				if err != nil {
					note("reader inspect %s: %v", owner, err)
				} else if !status.Held || !status.Information.ReadLock || status.Information.ReadDepth(owner) <= 0 {
					note("reader %s saw read=%v depth=%d held=%v", owner, status.Information.ReadLock, status.Information.ReadDepth(owner), status.Held)
				}
				if writing.Load() {
					note("reader %s still inside while writer held", owner)
				}
				reading.Add(-1)
				if err := readers[i].RUnLock(ctx, owner); err != nil {
					note("runlock %s round %d: %v", owner, round, err)
					return
				}
				finished.Add(1)
			}
		}(i)
	}
	for i := 0; i < highWriters; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			owner := fmt.Sprintf("writer-%d", i)
			for round := 0; round < highRounds; round++ {
				if err := writers[i].Lock(ctx, owner); err != nil {
					note("wlock %s round %d: %v", owner, round, err)
					return
				}
				if !writing.CompareAndSwap(false, true) {
					note("two writers entered")
				}
				if n := reading.Load(); n > 0 {
					note("writer %s entered while readers=%d", owner, n)
				}
				status, err := writers[i].Inspect(ctx)
				if err != nil {
					note("writer inspect %s: %v", owner, err)
				} else if !status.Held || status.Information.ReadLock || status.Information.OwnerId != owner || status.Information.LockCount <= 0 {
					note("writer %s saw owner %s read=%v count=%d", owner, status.Information.OwnerId, status.Information.ReadLock, status.Information.LockCount)
				}
				if n := reading.Load(); n > 0 {
					note("writer %s still inside while readers=%d", owner, n)
				}
				writing.Store(false)
				if err := writers[i].UnLock(ctx, owner); err != nil {
					note("wunlock %s round %d: %v", owner, round, err)
					return
				}
				finished.Add(1)
			}
		}(i)
	}
	wg.Wait()
	require.Empty(t, problems)
	require.GreaterOrEqual(t, highReaders, 64)
	require.GreaterOrEqual(t, highWriters, 8)
	require.GreaterOrEqual(t, highRounds, 4)
	require.Equal(t, int32((highReaders+highWriters)*highRounds), finished.Load())

	released, err := writers[0].Inspect(ctx)
	require.NoError(t, err)
	require.False(t, released.Held)
	require.Equal(t, 0, released.Information.LockCount)
	fmt.Printf("PASS high rw readers=%d writers=%d rounds=%d acquires=%d writer-exclusive released\n",
		highReaders, highWriters, highRounds, finished.Load())
}

func highClient(t *testing.T, store *mockSubscribeStorage, clock *mockSubscribeTimeProvider, id string) *StorageLock {
	t.Helper()
	options := NewStorageLockOptionsWithLockId(id).
		SetTimeProvider(clock).
		SetLeaseExpireAfter(5 * time.Minute).
		SetDisableWatchDog(true).
		SetVersionMissRetryInterval(100 * time.Microsecond)
	lock, err := NewStorageLockWithOptions(store, options)
	require.NoError(t, err)
	return lock
}
