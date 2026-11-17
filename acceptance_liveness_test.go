package storage_lock

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/storage-lock/go-storage"
	"github.com/stretchr/testify/require"
)

// gateStorage 在真实的版本校验之上注入读写故障。版本对不上时仍然失败。
type gateStorage struct {
	*mockSubscribeStorage
	failGet         atomic.Bool
	failUpdate      atomic.Bool
	failUpdatesLeft atomic.Int32
}

func (s *gateStorage) Get(ctx context.Context, lockId string) (string, error) {
	if s.failGet.Load() {
		return "", errors.New("get down")
	}
	return s.mockSubscribeStorage.Get(ctx, lockId)
}

func (s *gateStorage) UpdateWithVersion(ctx context.Context, lockId string, oldVersion, newVersion storage.Version, information *storage.LockInformation) error {
	if s.failUpdate.Load() {
		return errors.New("update down")
	}
	if s.failUpdatesLeft.Load() > 0 {
		s.failUpdatesLeft.Add(-1)
		return errors.New("update blip")
	}
	return s.mockSubscribeStorage.UpdateWithVersion(ctx, lockId, oldVersion, newVersion, information)
}

// flakyClock 可以在测试里关掉时间源，用来确认读不到时间时不会误报失锁。
type flakyClock struct {
	*mockSubscribeTimeProvider
	fail atomic.Bool
}

func (f *flakyClock) GetTime(ctx context.Context) (time.Time, error) {
	if f.fail.Load() {
		return time.Time{}, errors.New("time down")
	}
	return f.mockSubscribeTimeProvider.GetTime(ctx)
}

type unwatchableStorage struct {
	storage.Storage
}

type watchErrorStorage struct {
	*mockSubscribeStorage
}

func (s *watchErrorStorage) Watch(ctx context.Context, lockId string) (<-chan *storage.LockInformation, error) {
	return nil, errors.New("watch down")
}

type closedNotifyStorage struct {
	*mockSubscribeStorage
}

func (s *closedNotifyStorage) Watch(ctx context.Context, lockId string) (<-chan *storage.LockInformation, error) {
	ch := make(chan *storage.LockInformation)
	close(ch)
	return ch, nil
}

type nilWatchStorage struct {
	*mockSubscribeStorage
}

func (s *nilWatchStorage) Watch(ctx context.Context, lockId string) (<-chan *storage.LockInformation, error) {
	return nil, nil
}

// missAcquireStorage 让指定持有者的第一次成功条件写返回版本冲突，之后恢复真实 CAS。
type missAcquireStorage struct {
	*mockSubscribeStorage
	owner  string
	once   sync.Once
	missed atomic.Bool
}

func (s *missAcquireStorage) UpdateWithVersion(ctx context.Context, lockId string, oldVersion, newVersion storage.Version, information *storage.LockInformation) error {
	fail := false
	if information != nil && information.OwnerId == s.owner && information.LockCount > 0 {
		s.once.Do(func() { fail = true })
	}
	if fail {
		s.missed.Store(true)
		return ErrVersionMiss
	}
	return s.mockSubscribeStorage.UpdateWithVersion(ctx, lockId, oldVersion, newVersion, information)
}

func requireVersionChecked(t *testing.T, store storage.Storage, lockId string) {
	t.Helper()
	err := store.UpdateWithVersion(context.Background(), lockId, 999, 1000, &storage.LockInformation{LockId: lockId, OwnerId: "nobody"})
	require.ErrorIs(t, err, ErrVersionMiss)
}

func livenessLock(t *testing.T, store storage.Storage, lockId string, clock storage.TimeProvider, lost func(error)) *StorageLock {
	t.Helper()
	options := NewStorageLockOptionsWithLockId(lockId).
		SetTimeProvider(clock).
		SetLeaseExpireAfter(3 * time.Second).
		SetLeaseRefreshInterval(100 * time.Millisecond).
		SetVersionMissRetryInterval(10 * time.Millisecond)
	if lost != nil {
		options.SetOnLockLost(lost)
	}
	lock, err := NewStorageLockWithOptions(store, options)
	require.NoError(t, err)
	return lock
}

func countLost() (func(error), *atomic.Int32, <-chan error) {
	var n atomic.Int32
	first := make(chan error, 1)
	return func(err error) {
		if n.Add(1) == 1 {
			first <- err
		}
	}, &n, first
}

func TestLostNotifyWhenLeaseCannotBeExtended(t *testing.T) {
	store := &gateStorage{mockSubscribeStorage: newMockSubscribeStorage()}
	clock := newMockSubscribeTimeProvider(time.Now())
	callback, count, first := countLost()
	lock := livenessLock(t, store, "lease-due", clock, callback)
	owned, stop, err := lock.BeginLease(context.Background(), "owner")
	require.NoError(t, err)
	defer stop()
	requireVersionChecked(t, store, "lease-due")
	before, err := lock.Inspect(context.Background())
	require.NoError(t, err)

	clock.Advance(4 * time.Second)
	select {
	case err := <-first:
		require.ErrorIs(t, err, ErrLeaseExpired)
	case <-time.After(2 * time.Second):
		t.Fatal("租约到点且无法再延长时没有失锁通知")
	}
	require.Eventually(t, func() bool {
		return errors.Is(context.Cause(owned), ErrLockLost)
	}, time.Second, 10*time.Millisecond)

	store.failUpdate.Store(false)
	time.Sleep(400 * time.Millisecond)
	require.Equal(t, int32(1), count.Load())
	after, err := lock.Inspect(context.Background())
	require.NoError(t, err)
	require.False(t, after.Information.LeaseExpireTime.After(before.Information.LeaseExpireTime), "通知之后同一次持有又被续长了")
	require.Equal(t, before.Information.Version, after.Information.Version)
	dog := lock.storageLockWatchDog.(*WatchDogCommonsImpl)
	require.ErrorIs(t, dog.refreshOnce(), ErrLeaseExpired)
	again, err := lock.Inspect(context.Background())
	require.NoError(t, err)
	require.Equal(t, before.Information.Version, again.Information.Version)
}

func TestLostNotifyWhenRenewKeepsFailingPastExpiry(t *testing.T) {
	store := &gateStorage{mockSubscribeStorage: newMockSubscribeStorage()}
	clock := newMockSubscribeTimeProvider(time.Now())
	callback, count, first := countLost()
	lock := livenessLock(t, store, "renew-fail", clock, callback)
	owned, stop, err := lock.BeginLease(context.Background(), "owner")
	require.NoError(t, err)
	defer stop()
	requireVersionChecked(t, store, "renew-fail")
	require.Eventually(t, func() bool {
		status, err := lock.Inspect(context.Background())
		return err == nil && status.Information.Version > 1
	}, time.Second, 20*time.Millisecond)
	before, err := lock.Inspect(context.Background())
	require.NoError(t, err)

	store.failGet.Store(true)
	clock.Advance(4 * time.Second)
	select {
	case <-first:
	case <-time.After(2 * time.Second):
		t.Fatal("续租持续失败直到租约过期时没有失锁通知")
	}
	require.Eventually(t, func() bool {
		return errors.Is(context.Cause(owned), ErrLockLost)
	}, time.Second, 10*time.Millisecond)
	store.failGet.Store(false)
	time.Sleep(400 * time.Millisecond)
	require.Equal(t, int32(1), count.Load())
	after, err := lock.Inspect(context.Background())
	require.NoError(t, err)
	require.Equal(t, before.Information.Version, after.Information.Version)
	require.False(t, after.Information.LeaseExpireTime.After(before.Information.LeaseExpireTime))
}

func TestRenewFailureThenSuccessDoesNotNotify(t *testing.T) {
	store := &gateStorage{mockSubscribeStorage: newMockSubscribeStorage()}
	clock := newMockSubscribeTimeProvider(time.Now())
	callback, count, _ := countLost()
	lock := livenessLock(t, store, "renew-blip", clock, callback)
	require.NoError(t, lock.Lock(context.Background(), "owner"))
	defer lock.UnLock(context.Background(), "owner")
	time.Sleep(250 * time.Millisecond)
	before, err := lock.Inspect(context.Background())
	require.NoError(t, err)

	store.failUpdatesLeft.Store(1)
	clock.Advance(time.Second)
	require.Eventually(t, func() bool {
		status, err := lock.Inspect(context.Background())
		return err == nil && status.Information.LeaseExpireTime.After(before.Information.LeaseExpireTime)
	}, 2*time.Second, 20*time.Millisecond)
	time.Sleep(300 * time.Millisecond)
	require.Equal(t, int32(0), count.Load())
	owner, err := lock.GetOwner(context.Background())
	require.NoError(t, err)
	require.Equal(t, "owner", owner)
}

func TestUnlockDoesNotNotifyLockLost(t *testing.T) {
	store := newMockSubscribeStorage()
	clock := newMockSubscribeTimeProvider(time.Now())
	callback, count, _ := countLost()
	lock := livenessLock(t, store, "unlock-quiet", clock, callback)
	require.NoError(t, lock.Lock(context.Background(), "owner"))
	require.NoError(t, lock.UnLock(context.Background(), "owner"))
	time.Sleep(400 * time.Millisecond)
	require.Equal(t, int32(0), count.Load())
}

func TestOnLockLostPanicDoesNotKillProcess(t *testing.T) {
	store := newMockSubscribeStorage()
	clock := newMockSubscribeTimeProvider(time.Now())
	var n atomic.Int32
	lock := livenessLock(t, store, "panic-listener", clock, func(error) {
		n.Add(1)
		panic("listener failed")
	})
	owned, stop, err := lock.BeginLease(context.Background(), "owner")
	require.NoError(t, err)
	defer stop()
	before, err := lock.Inspect(context.Background())
	require.NoError(t, err)
	clock.Advance(4 * time.Second)
	require.Eventually(t, func() bool { return n.Load() == 1 }, 2*time.Second, 20*time.Millisecond)
	require.Eventually(t, func() bool {
		return errors.Is(context.Cause(owned), ErrLockLost)
	}, time.Second, 10*time.Millisecond)
	time.Sleep(300 * time.Millisecond)
	require.Equal(t, int32(1), n.Load())
	dog := lock.storageLockWatchDog.(*WatchDogCommonsImpl)
	dog.loseHold(errors.New("again"))
	require.Equal(t, int32(1), n.Load())
	after, err := lock.Inspect(context.Background())
	require.NoError(t, err)
	require.Equal(t, before.Information.Version, after.Information.Version)
}

func TestTimeUnconfirmedDoesNotFalseNotify(t *testing.T) {
	store := &gateStorage{mockSubscribeStorage: newMockSubscribeStorage()}
	clock := &flakyClock{mockSubscribeTimeProvider: newMockSubscribeTimeProvider(time.Now())}
	callback, count, first := countLost()
	lock := livenessLock(t, store, "time-down", clock, callback)
	require.NoError(t, lock.Lock(context.Background(), "owner"))
	time.Sleep(250 * time.Millisecond)
	store.failGet.Store(true)
	clock.fail.Store(true)
	clock.Advance(4 * time.Second)
	time.Sleep(400 * time.Millisecond)
	require.Equal(t, int32(0), count.Load())
	store.failGet.Store(false)
	clock.fail.Store(false)
	select {
	case <-first:
	case <-time.After(2 * time.Second):
		t.Fatal("时间源恢复并且租约已到点时没有失锁通知")
	}
	require.Equal(t, int32(1), count.Load())
}

func TestFairQueueEvictsCrashedWaiterKeepsOrder(t *testing.T) {
	store := newMockSubscribeStorage()
	clock := newMockSubscribeTimeProvider(time.Now())
	ctx := context.Background()
	now, err := clock.GetTime(ctx)
	require.NoError(t, err)
	const lockID = "fair-evict"
	store.data[lockID] = &storage.LockInformation{
		LockId:           lockID,
		OwnerId:          "previous",
		Version:          4,
		LockCount:        0,
		LeaseExpireTime:  now.Add(-time.Second),
		WriteIntentOwner: "dead",
		WriteIntentSince: now.Add(-time.Minute),
		Waiters: []storage.LockWaiter{
			{OwnerId: "dead", Mode: waiterModeWrite, Since: now.Add(-time.Minute), Until: now.Add(-time.Second)},
			{OwnerId: "live", Mode: waiterModeWrite, Since: now.Add(-time.Second), Until: now.Add(time.Hour)},
			{OwnerId: "later", Mode: waiterModeWrite, Since: now, Until: now.Add(time.Hour)},
		},
	}
	requireVersionChecked(t, store, lockID)
	cutter := fairLock(t, store, clock, lockID)
	require.ErrorIs(t, cutter.TryLock(ctx, "cutter"), ErrNotYourTurn)
	pruned, err := cutter.Inspect(ctx)
	require.NoError(t, err)
	require.Equal(t, "previous", pruned.Information.OwnerId)
	require.Equal(t, []string{"live", "later"}, []string{pruned.Waiters[0].OwnerId, pruned.Waiters[1].OwnerId})
	require.Equal(t, "live", pruned.WriteIntentOwner)

	live := fairLock(t, store, clock, lockID)
	require.NoError(t, live.Lock(ctx, "live"))
	waiters, err := live.Waiters(ctx)
	require.NoError(t, err)
	require.Len(t, waiters, 1)
	require.Equal(t, "later", waiters[0].OwnerId)
	status, err := live.Inspect(ctx)
	require.NoError(t, err)
	require.Equal(t, "live", status.Information.OwnerId)

	require.Error(t, cutter.TryLock(ctx, "cutter"))
	later := fairLock(t, store, clock, lockID)
	done := make(chan error, 1)
	go func() { done <- later.Lock(ctx, "later") }()
	select {
	case err := <-done:
		t.Fatalf("靠后的等待者越过了仍持有的人: %v", err)
	case <-time.After(150 * time.Millisecond):
	}
	require.NoError(t, live.UnLock(ctx, "live"))
	require.NoError(t, <-done)
	owned, err := later.GetOwner(ctx)
	require.NoError(t, err)
	require.Equal(t, "later", owned)
	require.NoError(t, later.UnLock(ctx, "later"))
}

func TestFairQueueKeepsZeroDeadlineWaiter(t *testing.T) {
	store := newMockSubscribeStorage()
	clock := newMockSubscribeTimeProvider(time.Now())
	ctx := context.Background()
	now, err := clock.GetTime(ctx)
	require.NoError(t, err)
	const lockID = "fair-legacy"
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
	require.ErrorIs(t, cutter.TryLock(ctx, "cutter"), ErrNotYourTurn)
	status, err := cutter.Inspect(ctx)
	require.NoError(t, err)
	require.NotEqual(t, "cutter", status.Information.OwnerId)
	require.Len(t, status.Waiters, 1)
	require.Equal(t, "legacy", status.Waiters[0].OwnerId)

	legacy := fairLock(t, store, clock, lockID)
	require.NoError(t, legacy.Lock(ctx, "legacy"))
	require.NoError(t, legacy.UnLock(ctx, "legacy"))
}

func TestFairWaiterRenewsWhileBlocking(t *testing.T) {
	store := newMockSubscribeStorage()
	clock := newMockSubscribeTimeProvider(time.Now())
	ctx := context.Background()
	holder := fairLock(t, store, clock, "fair-renew")
	holder.options.DisableWatchDog = true
	early := fairLock(t, store, clock, "fair-renew")
	late := fairLock(t, store, clock, "fair-renew")
	require.NoError(t, holder.Lock(ctx, "holder"))

	earlyCtx, cancelEarly := context.WithCancel(ctx)
	lateCtx, cancelLate := context.WithCancel(ctx)
	defer cancelEarly()
	defer cancelLate()
	go func() { _ = early.Lock(earlyCtx, "early") }()
	require.Eventually(t, func() bool {
		waiters, err := holder.Waiters(ctx)
		return err == nil && len(waiters) == 1 && waiters[0].OwnerId == "early"
	}, time.Second, 5*time.Millisecond)
	go func() { _ = late.Lock(lateCtx, "late") }()
	var first, second time.Time
	require.Eventually(t, func() bool {
		waiters, err := holder.Waiters(ctx)
		if err != nil || len(waiters) != 2 {
			return false
		}
		first, second = waiters[0].Until, waiters[1].Until
		return waiters[0].OwnerId == "early" && waiters[1].OwnerId == "late" && !first.IsZero() && !second.IsZero()
	}, time.Second, 5*time.Millisecond)

	clock.Advance(300 * time.Millisecond)
	require.Eventually(t, func() bool {
		waiters, err := holder.Waiters(ctx)
		if err != nil || len(waiters) != 2 {
			return false
		}
		return waiters[0].OwnerId == "early" && waiters[1].OwnerId == "late" &&
			waiters[0].Until.After(first) && waiters[1].Until.After(second)
	}, 2*time.Second, 5*time.Millisecond)
	require.NoError(t, holder.UnLock(ctx, "holder"))
}

func TestCancelFairWaiterRemovesImmediately(t *testing.T) {
	store := newMockSubscribeStorage()
	clock := newMockSubscribeTimeProvider(time.Now())
	ctx := context.Background()
	holder := fairLock(t, store, clock, "fair-cancel")
	waiter := fairLock(t, store, clock, "fair-cancel")
	require.NoError(t, holder.Lock(ctx, "holder"))
	waitCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- waiter.Lock(waitCtx, "waiting") }()
	require.Eventually(t, func() bool {
		waiters, err := holder.Waiters(ctx)
		return err == nil && len(waiters) == 1 && waiters[0].OwnerId == "waiting"
	}, time.Second, 5*time.Millisecond)
	cancel()
	require.Error(t, <-done)
	waiters, err := holder.Waiters(ctx)
	require.NoError(t, err)
	require.Empty(t, waiters)
	require.NoError(t, holder.UnLock(ctx, "holder"))
}

func TestNonFairAcquireDoesNotRequireQueue(t *testing.T) {
	store := newMockSubscribeStorage()
	clock := newMockSubscribeTimeProvider(time.Now())
	ctx := context.Background()
	options := NewStorageLockOptionsWithLockId("unfair-free").
		SetTimeProvider(clock).
		SetLeaseExpireAfter(5 * time.Minute).
		SetDisableWatchDog(true).
		SetVersionMissRetryInterval(10 * time.Millisecond)
	lock, err := NewStorageLockWithOptions(store, options)
	require.NoError(t, err)
	require.NoError(t, lock.Lock(ctx, "owner"))
	status, err := lock.Inspect(ctx)
	require.NoError(t, err)
	require.Empty(t, status.Waiters)
	require.Equal(t, "owner", status.Information.OwnerId)
	require.NoError(t, lock.UnLock(ctx, "owner"))

	now, err := clock.GetTime(ctx)
	require.NoError(t, err)
	store.data["unfair-free"] = &storage.LockInformation{
		LockId: "unfair-free", OwnerId: "previous", Version: 3, LockCount: 0, LeaseExpireTime: now.Add(-time.Second),
	}
	require.NoError(t, lock.Lock(ctx, "next"))
	require.NoError(t, lock.UnLock(ctx, "next"))

	holder, err := NewStorageLockWithOptions(store, options)
	require.NoError(t, err)
	require.NoError(t, holder.Lock(ctx, "holder"))
	busyCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() { _ = lock.Lock(busyCtx, "cutter") }()
	time.Sleep(80 * time.Millisecond)
	waiters, err := holder.Waiters(ctx)
	require.NoError(t, err)
	require.Empty(t, waiters)
	cancel()
	require.NoError(t, holder.UnLock(ctx, "holder"))
}

func TestBlockingAcquireWakesBeforeRetryInterval(t *testing.T) {
	cases := []struct {
		name    string
		call    func(lock *StorageLock, ctx context.Context, owner string) error
		release func(lock *StorageLock, ctx context.Context, owner string) error
	}{
		{name: "Lock", call: (*StorageLock).Lock, release: (*StorageLock).UnLock},
		{name: "RLock", call: (*StorageLock).RLock, release: (*StorageLock).RUnLock},
		{name: "WLock", call: (*StorageLock).WLock, release: (*StorageLock).WUnLock},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := newMockSubscribeStorage()
			store.emitWatch = true
			clock := newMockSubscribeTimeProvider(time.Now())
			options := NewStorageLockOptionsWithLockId("wake-" + tc.name).
				SetTimeProvider(clock).
				SetLeaseExpireAfter(5 * time.Minute).
				SetDisableWatchDog(true).
				SetVersionMissRetryInterval(20 * time.Second)
			holder, err := NewStorageLockWithOptions(store, options)
			require.NoError(t, err)
			waiter, err := NewStorageLockWithOptions(store, options)
			require.NoError(t, err)
			ctx := context.Background()
			require.NoError(t, holder.Lock(ctx, "holder"))
			done := make(chan error, 1)
			go func() { done <- tc.call(waiter, ctx, "waiter") }()
			time.Sleep(200 * time.Millisecond)
			select {
			case err := <-done:
				t.Fatalf("持有者还没释放就返回了: %v", err)
			default:
			}
			require.NoError(t, holder.UnLock(ctx, "holder"))
			started := time.Now()
			select {
			case err := <-done:
				require.NoError(t, err)
				require.Less(t, time.Since(started), 3*time.Second)
			case <-time.After(3 * time.Second):
				t.Fatal("释放后的存储通知没有在重试间隔结束前唤醒阻塞获取")
			}
			require.NoError(t, tc.release(waiter, ctx, "waiter"))
		})
	}
}

func TestBlockingAcquireFallsBackWithoutWatch(t *testing.T) {
	store := newMockSubscribeStorage()
	clock := newMockSubscribeTimeProvider(time.Now())
	options := func(store storage.Storage) *StorageLockOptions {
		return NewStorageLockOptionsWithLockId("no-watch").
			SetTimeProvider(clock).
			SetLeaseExpireAfter(5 * time.Minute).
			SetDisableWatchDog(true).
			SetVersionMissRetryInterval(15 * time.Millisecond)
	}
	holder, err := NewStorageLockWithOptions(&unwatchableStorage{Storage: store}, options(store))
	require.NoError(t, err)
	waiter, err := NewStorageLockWithOptions(&unwatchableStorage{Storage: store}, options(store))
	require.NoError(t, err)
	ctx := context.Background()
	require.NoError(t, holder.Lock(ctx, "holder"))
	done := make(chan error, 1)
	go func() { done <- waiter.Lock(ctx, "waiter") }()
	time.Sleep(40 * time.Millisecond)
	require.NoError(t, holder.UnLock(ctx, "holder"))
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("没有通知能力时阻塞获取没有靠重试拿到锁")
	}
	require.NoError(t, waiter.UnLock(ctx, "waiter"))

	errStore := &watchErrorStorage{mockSubscribeStorage: newMockSubscribeStorage()}
	errHolder, err := NewStorageLockWithOptions(errStore, options(errStore))
	require.NoError(t, err)
	errWaiter, err := NewStorageLockWithOptions(errStore, options(errStore))
	require.NoError(t, err)
	require.NoError(t, errHolder.Lock(ctx, "holder"))
	done = make(chan error, 1)
	go func() { done <- errWaiter.Lock(ctx, "waiter") }()
	time.Sleep(40 * time.Millisecond)
	require.NoError(t, errHolder.UnLock(ctx, "holder"))
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("通知订阅失败时阻塞获取没有靠重试拿到锁")
	}
	require.NoError(t, errWaiter.UnLock(ctx, "waiter"))
}

func TestBlockingAcquireHonorsCancelWithoutNotify(t *testing.T) {
	store := newMockSubscribeStorage()
	clock := newMockSubscribeTimeProvider(time.Now())
	options := NewStorageLockOptionsWithLockId("cancel-wait").
		SetTimeProvider(clock).
		SetLeaseExpireAfter(5 * time.Minute).
		SetDisableWatchDog(true).
		SetVersionMissRetryInterval(30 * time.Second)
	holder, err := NewStorageLockWithOptions(&unwatchableStorage{Storage: store}, options)
	require.NoError(t, err)
	waiter, err := NewStorageLockWithOptions(&unwatchableStorage{Storage: store}, options)
	require.NoError(t, err)
	ctx := context.Background()
	require.NoError(t, holder.Lock(ctx, "holder"))
	waitCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- waiter.Lock(waitCtx, "waiter") }()
	time.Sleep(50 * time.Millisecond)
	started := time.Now()
	cancel()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
		require.Less(t, time.Since(started), time.Second)
	case <-time.After(time.Second):
		t.Fatal("没有通知时取消上下文没有结束阻塞获取")
	}
	require.NoError(t, holder.UnLock(ctx, "holder"))
}

func TestNotifyDoesNotSkipEarlierFairWaiter(t *testing.T) {
	store := newMockSubscribeStorage()
	store.emitWatch = true
	clock := newMockSubscribeTimeProvider(time.Now())
	options := NewStorageLockOptionsWithLockId("fair-wake").
		SetTimeProvider(clock).
		SetFair(true).
		SetLeaseExpireAfter(5 * time.Minute).
		SetDisableWatchDog(true).
		SetVersionMissRetryInterval(20 * time.Second)
	holder, err := NewStorageLockWithOptions(store, options)
	require.NoError(t, err)
	first, err := NewStorageLockWithOptions(store, options)
	require.NoError(t, err)
	second, err := NewStorageLockWithOptions(store, options)
	require.NoError(t, err)
	ctx := context.Background()
	require.NoError(t, holder.Lock(ctx, "holder"))
	firstDone := make(chan error, 1)
	secondDone := make(chan error, 1)
	go func() { firstDone <- first.Lock(ctx, "first") }()
	require.Eventually(t, func() bool {
		waiters, err := holder.Waiters(ctx)
		return err == nil && len(waiters) == 1
	}, time.Second, 5*time.Millisecond)
	go func() { secondDone <- second.Lock(ctx, "second") }()
	require.Eventually(t, func() bool {
		waiters, err := holder.Waiters(ctx)
		return err == nil && len(waiters) == 2 && waiters[0].OwnerId == "first"
	}, time.Second, 5*time.Millisecond)
	time.Sleep(100 * time.Millisecond)
	store.PushWatchEvent("fair-wake", &storage.LockInformation{LockId: "fair-wake"})
	store.PushWatchEvent("fair-wake", &storage.LockInformation{LockId: "fair-wake"})
	time.Sleep(100 * time.Millisecond)
	select {
	case err := <-secondDone:
		t.Fatalf("重复通知让后一位拿到了锁: %v", err)
	default:
	}
	require.NoError(t, holder.UnLock(ctx, "holder"))
	select {
	case err := <-firstDone:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("队首没有被释放通知唤醒")
	}
	select {
	case err := <-secondDone:
		t.Fatalf("队尾在队首释放前拿到了锁: %v", err)
	case <-time.After(150 * time.Millisecond):
	}
	require.NoError(t, first.UnLock(ctx, "first"))
	select {
	case err := <-secondDone:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("队首释放后队尾没有拿到锁")
	}
	require.NoError(t, second.UnLock(ctx, "second"))
}

func TestNotifyDoesNotTreatVersionMissAsAcquire(t *testing.T) {
	inner := newMockSubscribeStorage()
	inner.emitWatch = true
	store := &missAcquireStorage{mockSubscribeStorage: inner, owner: "waiter"}
	clock := newMockSubscribeTimeProvider(time.Now())
	options := NewStorageLockOptionsWithLockId("cas-wake").
		SetTimeProvider(clock).
		SetLeaseExpireAfter(5 * time.Minute).
		SetDisableWatchDog(true).
		SetVersionMissRetryInterval(200 * time.Millisecond)
	holder, err := NewStorageLockWithOptions(store, options)
	require.NoError(t, err)
	waiter, err := NewStorageLockWithOptions(store, options)
	require.NoError(t, err)
	ctx := context.Background()
	require.NoError(t, holder.Lock(ctx, "holder"))
	done := make(chan error, 1)
	go func() { done <- waiter.Lock(ctx, "waiter") }()
	time.Sleep(80 * time.Millisecond)
	store.PushWatchEvent("cas-wake", &storage.LockInformation{LockId: "cas-wake", OwnerId: "holder"})
	time.Sleep(80 * time.Millisecond)
	select {
	case err := <-done:
		t.Fatalf("锁还被持有时，通知被当成获取成功: %v", err)
	default:
	}
	require.NoError(t, holder.UnLock(ctx, "holder"))
	require.Eventually(t, func() bool { return store.missed.Load() }, time.Second, 5*time.Millisecond)
	select {
	case err := <-done:
		t.Fatalf("条件写冲突被通知当成获取成功: %v", err)
	default:
	}
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("版本冲突之后没有重试成功")
	}
	owner, err := waiter.GetOwner(ctx)
	require.NoError(t, err)
	require.Equal(t, "waiter", owner)
	require.NoError(t, waiter.UnLock(ctx, "waiter"))
}

func TestTryLockDoesNotWaitForNotify(t *testing.T) {
	store := newMockSubscribeStorage()
	store.emitWatch = true
	clock := newMockSubscribeTimeProvider(time.Now())
	options := NewStorageLockOptionsWithLockId("try-once").
		SetTimeProvider(clock).
		SetLeaseExpireAfter(5 * time.Minute).
		SetDisableWatchDog(true).
		SetVersionMissRetryInterval(30 * time.Second)
	holder, err := NewStorageLockWithOptions(store, options)
	require.NoError(t, err)
	other, err := NewStorageLockWithOptions(store, options)
	require.NoError(t, err)
	ctx := context.Background()
	require.NoError(t, holder.Lock(ctx, "holder"))
	started := time.Now()
	err = other.TryLock(ctx, "other")
	require.ErrorIs(t, err, ErrLockBusy)
	require.Less(t, time.Since(started), 200*time.Millisecond)
	require.NoError(t, holder.UnLock(ctx, "holder"))
}
