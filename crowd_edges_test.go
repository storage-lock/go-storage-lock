package storage_lock

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/storage-lock/go-events"
	"github.com/storage-lock/go-storage"
	"github.com/stretchr/testify/require"
)

// 已有用例只覆盖单人取消，以及不带外人挤兑的重入。
// 这里三场次数固定，不靠睡眠决定谁拿到锁。
const (
	takeoverCallers   = 32
	takeoverRounds    = 2
	cancelWaiters     = 64
	reentryDepth      = 8
	reentryOutsiders  = 128
	edgeRetryInterval = 100 * time.Microsecond
)

func TestExpiryTakeover(t *testing.T) {
	store := newMockSubscribeStorage()
	clock := newMockSubscribeTimeProvider(time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC))
	const lockID = "expiry-takeover"
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	expired := edgeClient(t, store, clock, lockID, 3*time.Second, nil)
	require.NoError(t, expired.Lock(ctx, "expired"))
	held := lockSnap(t, expired)
	expiredVersion := held.Version
	// 把可控时间推到租约到期点。恰好到期已经无效，不靠墙钟睡眠。
	clock.Set(held.LeaseExpireTime)

	successor := edgeClient(t, store, clock, lockID, 3*time.Second, nil)
	require.NoError(t, successor.Lock(ctx, "successor"))
	taken := lockSnap(t, successor)
	require.Equal(t, "successor", taken.OwnerId)
	require.Greater(t, taken.LockCount, 0)
	require.Greater(t, taken.Version, expiredVersion)

	require.Error(t, expired.UnLock(ctx, "expired"))
	require.Error(t, expired.Renew(ctx, "expired"))
	afterReject := lockSnap(t, successor)
	requireSameRecord(t, taken, afterReject)
	fmt.Println("PASS takeover expired holder unlock and renew left successor unchanged")

	require.NoError(t, successor.UnLock(ctx, "successor"))

	locks := make([]*StorageLock, takeoverCallers)
	for i := 0; i < takeoverCallers; i++ {
		locks[i] = edgeClient(t, store, clock, lockID, 3*time.Second, nil)
	}
	var current atomic.Int32
	var maxCritical atomic.Int32
	var listMu sync.Mutex
	versions := make([]storage.Version, 0, takeoverCallers*takeoverRounds)
	var errMu sync.Mutex
	var problems []string
	note := func(format string, args ...any) {
		errMu.Lock()
		problems = append(problems, fmt.Sprintf(format, args...))
		errMu.Unlock()
	}
	var wg sync.WaitGroup
	for i := 0; i < takeoverCallers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			owner := fmt.Sprintf("caller-%d", i)
			for round := 0; round < takeoverRounds; round++ {
				if err := locks[i].Lock(ctx, owner); err != nil {
					note("lock %s round %d: %v", owner, round, err)
					return
				}
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
					note("holder %s saw owner %s count %d", owner, status.Information.OwnerId, status.Information.LockCount)
				} else if status.Information.Version <= expiredVersion {
					note("version %d is not above expired %d", status.Information.Version, expiredVersion)
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
	require.GreaterOrEqual(t, takeoverCallers, 32)
	require.GreaterOrEqual(t, takeoverRounds, 2)
	require.Len(t, versions, takeoverCallers*takeoverRounds)
	for i := 0; i < len(versions); i++ {
		require.Greater(t, versions[i], expiredVersion)
		if i > 0 {
			require.Greater(t, versions[i], versions[i-1], "fencing version went backwards at %d", i)
		}
	}
	released, err := successor.Inspect(ctx)
	require.NoError(t, err)
	require.False(t, released.Held)
	require.Equal(t, 0, released.Information.LockCount)
	fmt.Printf("PASS takeover callers=%d rounds=%d max-in-critical=%d acquires=%d versions-strict above-expired released\n",
		takeoverCallers, takeoverRounds, maxCritical.Load(), len(versions))
}

func TestCancelWhileAcquiring(t *testing.T) {
	store := newMockSubscribeStorage()
	clock := newMockSubscribeTimeProvider(time.Date(2026, 3, 4, 1, 0, 0, 0, time.UTC))
	const lockID = "cancel-crowd"
	ctx, stop := context.WithTimeout(context.Background(), 45*time.Second)
	defer stop()

	var entered atomic.Int32
	var once sync.Map
	listener := events.NewListenerWrapper("cancel-entered", func(_ context.Context, e *events.Event) {
		if lastActionName(e) != ActionLockBusy {
			return
		}
		if e.OwnerId == "" {
			return
		}
		if _, loaded := once.LoadOrStore(e.OwnerId, struct{}{}); loaded {
			return
		}
		entered.Add(1)
	})

	holder := edgeClient(t, store, clock, lockID, 5*time.Minute, nil)
	require.NoError(t, holder.Lock(ctx, "holder"))
	before := lockSnap(t, holder)

	waiters := make([]*StorageLock, cancelWaiters)
	cancels := make([]context.CancelFunc, cancelWaiters)
	defer func() {
		for _, cancel := range cancels {
			if cancel != nil {
				cancel()
			}
		}
	}()
	for i := 0; i < cancelWaiters; i++ {
		waiters[i] = edgeClient(t, store, clock, lockID, 5*time.Minute, listener)
	}

	var successes atomic.Int32
	var errMu sync.Mutex
	var problems []string
	note := func(format string, args ...any) {
		errMu.Lock()
		problems = append(problems, fmt.Sprintf(format, args...))
		errMu.Unlock()
	}
	var wg sync.WaitGroup
	for i := 0; i < cancelWaiters; i++ {
		wg.Add(1)
		waiterCtx, cancel := context.WithCancel(context.Background())
		cancels[i] = cancel
		go func(i int) {
			defer wg.Done()
			owner := fmt.Sprintf("waiter-%d", i)
			err := waiters[i].Lock(waiterCtx, owner)
			if err == nil {
				successes.Add(1)
				_ = waiters[i].UnLock(context.Background(), owner)
				note("waiter %s acquired", owner)
				return
			}
			if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
				note("waiter %s: %v", owner, err)
			}
		}(i)
	}
	for int(entered.Load()) < cancelWaiters {
		if ctx.Err() != nil {
			break
		}
		runtime.Gosched()
	}
	require.GreaterOrEqual(t, cancelWaiters, 64)
	require.Equal(t, int32(cancelWaiters), entered.Load())
	still := lockSnap(t, holder)
	requireSameRecord(t, before, still)

	for i := 0; i < cancelWaiters; i++ {
		cancels[i]()
	}
	wg.Wait()
	require.Empty(t, problems)
	require.Equal(t, int32(0), successes.Load())
	after := lockSnap(t, holder)
	requireSameRecord(t, before, after)
	require.Equal(t, "holder", after.OwnerId)
	require.Greater(t, after.LockCount, 0)
	fmt.Printf("PASS cancel waiters=%d entered=%d successes=%d holder-unchanged\n",
		cancelWaiters, entered.Load(), successes.Load())

	require.NoError(t, holder.UnLock(ctx, "holder"))
	next := edgeClient(t, store, clock, lockID, 5*time.Minute, nil)
	require.NoError(t, next.Lock(ctx, "next"))
	owned := lockSnap(t, next)
	require.Equal(t, "next", owned.OwnerId)
	require.Greater(t, owned.LockCount, 0)
	require.NoError(t, next.UnLock(ctx, "next"))
	released, err := next.Inspect(ctx)
	require.NoError(t, err)
	require.False(t, released.Held)
	require.Equal(t, 0, released.Information.LockCount)
	fmt.Println("PASS cancel reacquire released")
}

func TestReentryCrowd(t *testing.T) {
	store := newMockSubscribeStorage()
	clock := newMockSubscribeTimeProvider(time.Date(2026, 3, 4, 2, 0, 0, 0, time.UTC))
	const lockID = "reentry-crowd"
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	holder := edgeClient(t, store, clock, lockID, 5*time.Minute, nil)
	for i := 0; i < reentryDepth; i++ {
		require.NoError(t, holder.Lock(ctx, "holder"))
	}
	before := lockSnap(t, holder)
	require.GreaterOrEqual(t, reentryDepth, 8)
	require.Equal(t, reentryDepth, before.LockCount)
	require.Equal(t, "holder", before.OwnerId)
	require.False(t, before.ReadLock)

	outsiders := make([]*StorageLock, reentryOutsiders)
	for i := 0; i < reentryOutsiders; i++ {
		outsiders[i] = edgeClient(t, store, clock, lockID, 5*time.Minute, nil)
	}
	var errMu sync.Mutex
	var problems []string
	note := func(format string, args ...any) {
		errMu.Lock()
		problems = append(problems, fmt.Sprintf(format, args...))
		errMu.Unlock()
	}
	var acquired atomic.Int32
	var releasedByOutsider atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < reentryOutsiders; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			owner := fmt.Sprintf("outsider-%d", i)
			if err := outsiders[i].TryLock(ctx, owner); err == nil {
				acquired.Add(1)
				_ = outsiders[i].UnLock(context.Background(), owner)
				note("outsider %s acquired", owner)
			}
			if err := outsiders[i].UnLock(ctx, owner); err == nil {
				releasedByOutsider.Add(1)
				note("outsider %s released", owner)
			}
		}(i)
	}
	wg.Wait()
	require.Empty(t, problems)
	require.GreaterOrEqual(t, reentryOutsiders, 128)
	require.Equal(t, int32(0), acquired.Load())
	require.Equal(t, int32(0), releasedByOutsider.Load())
	after := lockSnap(t, holder)
	requireSameRecord(t, before, after)
	fmt.Printf("PASS reentry depth=%d outsiders=%d failed-acquire=%d failed-unlock=%d unchanged\n",
		before.LockCount, reentryOutsiders, reentryOutsiders, reentryOutsiders)

	for i := 0; i < reentryDepth; i++ {
		require.NoError(t, holder.UnLock(ctx, "holder"))
	}
	freed := lockSnap(t, holder)
	require.Equal(t, 0, freed.LockCount)
	held, err := holder.IsLocked(ctx)
	require.NoError(t, err)
	require.False(t, held)

	require.Error(t, holder.UnLock(ctx, "holder"))
	extra := lockSnap(t, holder)
	requireSameRecord(t, freed, extra)
	held, err = holder.IsLocked(ctx)
	require.NoError(t, err)
	require.False(t, held)
	fmt.Printf("PASS reentry released depth=%d extra-unlock-failed unheld\n", freed.LockCount)
}

func edgeClient(t *testing.T, store *mockSubscribeStorage, clock *mockSubscribeTimeProvider, id string, lease time.Duration, listener events.Listener) *StorageLock {
	t.Helper()
	options := NewStorageLockOptionsWithLockId(id).
		SetTimeProvider(clock).
		SetLeaseExpireAfter(lease).
		SetDisableWatchDog(true).
		SetVersionMissRetryInterval(edgeRetryInterval)
	if listener != nil {
		options.AddEventListeners(listener)
	}
	lock, err := NewStorageLockWithOptions(store, options)
	require.NoError(t, err)
	return lock
}

func lastActionName(e *events.Event) string {
	if e == nil || len(e.Actions) == 0 || e.Actions[len(e.Actions)-1] == nil {
		return ""
	}
	return e.Actions[len(e.Actions)-1].Name
}
