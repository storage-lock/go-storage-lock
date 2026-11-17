package storage_lock

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/storage-lock/go-storage"
	"github.com/stretchr/testify/require"
)

func TestClassifyWatchdogRefresh(t *testing.T) {
	wrapped := errors.Join(errors.New("storage"), ErrLockNotBelongYou)
	cases := []struct {
		name      string
		err       error
		running   bool
		pastKnown bool
		want      refreshDecision
	}{
		{name: "success", want: refreshContinue},
		{name: "taken over", err: ErrLockNotBelongYou, running: true, want: refreshDelegate},
		{name: "wrapped taken over", err: wrapped, want: refreshDelegate},
		{name: "missing", err: ErrLockNotFound, want: refreshDelegate},
		{name: "due while running", err: ErrLeaseExpired, running: true, want: refreshNotifyAndStop},
		{name: "due after stop", err: ErrLeaseExpired, running: false, want: refreshDelegate},
		{name: "other before deadline", err: errors.New("io"), pastKnown: false, want: refreshContinue},
		{name: "other after deadline", err: errors.New("io"), pastKnown: true, want: refreshNotifyAndStop},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, classifyWatchdogRefresh(tc.err, tc.running, tc.pastKnown))
		})
	}
}

func TestWatchdogStillOwns(t *testing.T) {
	require.False(t, watchdogStillOwns(nil, "owner"))
	require.False(t, watchdogStillOwns(&storage.LockInformation{}, "owner"))
	require.True(t, watchdogStillOwns(&storage.LockInformation{OwnerId: "owner", LockCount: 1}, "owner"))
	require.False(t, watchdogStillOwns(&storage.LockInformation{OwnerId: "other", LockCount: 1}, "owner"))
	require.True(t, watchdogStillOwns(&storage.LockInformation{
		ReadLock: true, LockCount: 1, ReadOwners: map[string]int{"owner": 1},
	}, "owner"))
	require.False(t, watchdogStillOwns(&storage.LockInformation{
		ReadLock: true, LockCount: 1, ReadOwners: map[string]int{"other": 1},
	}, "owner"))
}

func TestPruneAndReconcileWaiters(t *testing.T) {
	now := time.Now()
	kept, changed := pruneExpiredWaiters(nil, now)
	require.False(t, changed)
	require.Empty(t, kept)
	original := []storage.LockWaiter{
		{OwnerId: "legacy", Mode: waiterModeWrite, Since: now},
		{OwnerId: "dead", Mode: waiterModeWrite, Since: now, Until: now},
		{OwnerId: "live", Mode: waiterModeWrite, Since: now, Until: now.Add(time.Second)},
		{OwnerId: "reader", Mode: waiterModeRead, Since: now, Until: now.Add(-time.Second)},
	}
	kept, changed = pruneExpiredWaiters(original, now)
	require.True(t, changed)
	require.Equal(t, []string{"legacy", "live"}, []string{kept[0].OwnerId, kept[1].OwnerId})
	same, changed := pruneExpiredWaiters(kept, now)
	require.False(t, changed)
	require.Equal(t, kept, same)

	owner, since := reconcileWriteIntent(original, "", time.Time{}, kept)
	require.Empty(t, owner)
	require.True(t, since.IsZero())
	owner, since = reconcileWriteIntent(original, "outsider", now, kept)
	require.Equal(t, "outsider", owner)
	require.Equal(t, now, since)
	owner, since = reconcileWriteIntent(original, "live", now.Add(time.Minute), kept)
	require.Equal(t, "live", owner)
	require.Equal(t, now.Add(time.Minute), since)
	owner, since = reconcileWriteIntent(original, "dead", now, kept)
	require.Equal(t, "legacy", owner)
	require.Equal(t, kept[0].Since, since)
	owner, _ = reconcileWriteIntent([]storage.LockWaiter{{OwnerId: "dead", Mode: waiterModeWrite}}, "dead", now, nil)
	require.Equal(t, "", owner)
	require.Equal(t, time.Second, waiterDeadlineWindow(0))
	require.Equal(t, time.Second, waiterDeadlineWindow(100*time.Millisecond))
	require.Equal(t, 4*time.Second, waiterDeadlineWindow(time.Second))
	require.False(t, waiterExpired(storage.LockWaiter{}, now))
	require.True(t, waiterExpired(storage.LockWaiter{Until: now}, now))
}

func TestNewWaiterDeadlineFollowsFair(t *testing.T) {
	lock, _, clock := testSubscribeStorageLock(t, "new-waiter")
	now, err := clock.GetTime(context.Background())
	require.NoError(t, err)
	plain := lock.newWaiter("owner", waiterModeWrite, now)
	require.True(t, plain.Until.IsZero())
	lock.options.Fair = true
	lock.options.VersionMissRetryInterval = 10 * time.Millisecond
	fair := lock.newWaiter("owner", waiterModeWrite, now)
	require.Equal(t, now.Add(time.Second), fair.Until)
}

func TestDropExpiredWaitersBranches(t *testing.T) {
	ctx := context.Background()
	lock, store, clock := testSubscribeStorageLock(t, "drop-branches")
	lock.options.DisableWatchDog = true
	require.NoError(t, lock.dropExpiredWaiters(ctx, lock.operationEvent("owner"), nil, time.Time{}))
	require.NoError(t, lock.dropExpiredWaiters(ctx, lock.operationEvent("owner"), &storage.LockInformation{}, time.Time{}))

	now, err := clock.GetTime(ctx)
	require.NoError(t, err)
	info := &storage.LockInformation{
		LockId: "drop-branches", Version: math.MaxUint64, LockCount: 0,
		Waiters: []storage.LockWaiter{{OwnerId: "dead", Mode: waiterModeWrite, Until: now.Add(-time.Second)}},
	}
	require.ErrorIs(t, lock.dropExpiredWaiters(ctx, lock.operationEvent("owner"), info, now), ErrVersionExhausted)
	require.Equal(t, storage.Version(math.MaxUint64), info.Version)

	flaky := &flakyClock{mockSubscribeTimeProvider: clock}
	lock.options.TimeProvider = flaky
	flaky.fail.Store(true)
	info.Version = 2
	require.Error(t, lock.dropExpiredWaiters(ctx, lock.operationEvent("owner"), info, time.Time{}))
	flaky.fail.Store(false)

	store.data["drop-branches"] = &storage.LockInformation{
		LockId: "drop-branches", Version: 2, LockCount: 0, LeaseExpireTime: now.Add(-time.Second),
		Waiters: []storage.LockWaiter{{OwnerId: "dead", Mode: waiterModeWrite, Since: now, Until: now.Add(-time.Second)}},
	}
	bad := &errUpdateStorage{mockSubscribeStorage: store}
	badLock, err := NewStorageLockWithOptions(bad, NewStorageLockOptionsWithLockId("drop-branches").
		SetTimeProvider(clock).SetLeaseExpireAfter(5*time.Minute).SetDisableWatchDog(true))
	require.NoError(t, err)
	require.Error(t, badLock.TryLock(ctx, "owner"))

	miss := &missOnceStorage{mockSubscribeStorage: newMockSubscribeStorage(), left: 1}
	missNow, err := clock.GetTime(ctx)
	require.NoError(t, err)
	miss.data["drop-miss"] = &storage.LockInformation{
		LockId: "drop-miss", Version: 2, LockCount: 0, LeaseExpireTime: missNow.Add(-time.Second),
		Waiters: []storage.LockWaiter{{OwnerId: "dead", Mode: waiterModeWrite, Since: missNow, Until: missNow.Add(-time.Second)}},
	}
	missLock, err := NewStorageLockWithOptions(miss, NewStorageLockOptionsWithLockId("drop-miss").
		SetTimeProvider(clock).SetLeaseExpireAfter(5*time.Minute).SetDisableWatchDog(true).SetVersionMissRetryInterval(5*time.Millisecond))
	require.NoError(t, err)
	require.NoError(t, missLock.Lock(ctx, "owner"))
	require.Zero(t, miss.left)
	require.NoError(t, missLock.UnLock(ctx, "owner"))
}

func TestCloneLockInformationDoesNotAlias(t *testing.T) {
	src := &storage.LockInformation{
		LockId:     "clone",
		Waiters:    []storage.LockWaiter{{OwnerId: "a", Mode: waiterModeWrite}},
		ReadOwners: map[string]int{"a": 2},
		Extra:      map[string]string{"k": "v"},
	}
	clone := cloneLockInformation(src)
	clone.Waiters[0].OwnerId = "b"
	clone.ReadOwners["a"] = 9
	clone.Extra["k"] = "changed"
	clone.Extra["n"] = "new"
	require.Equal(t, "a", src.Waiters[0].OwnerId)
	require.Equal(t, 2, src.ReadOwners["a"])
	require.Equal(t, "v", src.Extra["k"])
	_, aliased := src.Extra["n"]
	require.False(t, aliased)

	empty := cloneLockInformation(&storage.LockInformation{LockId: "empty"})
	require.Nil(t, empty.Waiters)
	require.Nil(t, empty.ReadOwners)
	require.Nil(t, empty.Extra)
	require.Equal(t, "empty", empty.LockId)
}

func TestReadLockReturnsWhenWaiterPruneFails(t *testing.T) {
	lock, store, clock := testRWStorageLock(t, "reader-drop-err")
	lock.options.DisableWatchDog = true
	ctx := context.Background()
	now, err := clock.GetTime(ctx)
	require.NoError(t, err)
	store.data["reader-drop-err"] = &storage.LockInformation{
		LockId: "reader-drop-err", OwnerId: "reader", Version: math.MaxUint64, LockCount: 1, ReadLock: true,
		ReadOwners: map[string]int{"reader": 1}, LeaseExpireTime: now.Add(time.Minute),
		Waiters: []storage.LockWaiter{{OwnerId: "dead", Mode: waiterModeWrite, Since: now, Until: now.Add(-time.Second)}},
	}
	require.ErrorIs(t, lock.RLock(ctx, "new-reader"), ErrVersionExhausted)
	require.Equal(t, storage.Version(math.MaxUint64), store.data["reader-drop-err"].Version)
	require.Len(t, store.data["reader-drop-err"].Waiters, 1)
}

func TestExpiredWriteWaiterDoesNotBlockReader(t *testing.T) {
	lock, store, clock := testRWStorageLock(t, "reader-prune")
	lock.options.DisableWatchDog = true
	ctx := context.Background()
	now, err := clock.GetTime(ctx)
	require.NoError(t, err)
	store.data["reader-prune"] = &storage.LockInformation{
		LockId: "reader-prune", OwnerId: "reader", Version: 2, LockCount: 1, ReadLock: true,
		ReadOwners: map[string]int{"reader": 1}, LeaseExpireTime: now.Add(time.Minute),
		Waiters: []storage.LockWaiter{{OwnerId: "dead", Mode: waiterModeWrite, Since: now, Until: now.Add(-time.Second)}},
	}
	require.NoError(t, lock.RLock(ctx, "new-reader"))
	status, err := lock.Inspect(ctx)
	require.NoError(t, err)
	require.Empty(t, status.Waiters)
	require.Equal(t, 1, status.Information.ReadDepth("new-reader"))
}

func TestRenewWaiterRegistrationBranches(t *testing.T) {
	ctx := context.Background()
	lock, store, clock := testSubscribeStorageLock(t, "renew-branch")
	lock.options.DisableWatchDog = true
	require.NoError(t, lock.renewWaiterRegistration(ctx, "owner", waiterModeWrite))
	raw, err := store.Get(ctx, "renew-branch")
	require.ErrorIs(t, err, ErrLockNotFound)
	require.Empty(t, raw)

	lock.options.Fair = true
	flaky := &flakyClock{mockSubscribeTimeProvider: clock}
	lock.options.TimeProvider = flaky
	flaky.fail.Store(true)
	require.Error(t, lock.renewWaiterRegistration(ctx, "owner", waiterModeWrite))
	flaky.fail.Store(false)
	require.ErrorIs(t, lock.renewWaiterRegistration(ctx, "owner", waiterModeWrite), ErrLockNotFound)

	now, err := clock.GetTime(ctx)
	require.NoError(t, err)
	store.data["renew-branch"] = &storage.LockInformation{
		LockId: "renew-branch", Version: 3, LockCount: 1, OwnerId: "holder", LeaseExpireTime: now.Add(time.Minute),
		Waiters: []storage.LockWaiter{{OwnerId: "other", Mode: waiterModeWrite, Since: now, Until: now.Add(time.Second)}},
	}
	require.NoError(t, lock.renewWaiterRegistration(ctx, "owner", waiterModeWrite))
	store.data["renew-branch"].Waiters = append(store.data["renew-branch"].Waiters, storage.LockWaiter{
		OwnerId: "owner", Mode: waiterModeWrite, Since: now, Until: now.Add(2 * time.Second),
	})
	require.NoError(t, lock.renewWaiterRegistration(ctx, "owner", waiterModeWrite))
	require.Equal(t, storage.Version(3), store.data["renew-branch"].Version)

	store.data["renew-branch"].Waiters[1].Until = now
	store.data["renew-branch"].Version = math.MaxUint64
	require.ErrorIs(t, lock.renewWaiterRegistration(ctx, "owner", waiterModeWrite), ErrVersionExhausted)

	store.data["renew-branch"].Version = 4
	bad := &errUpdateStorage{mockSubscribeStorage: store}
	badLock, err := NewStorageLockWithOptions(bad, NewStorageLockOptionsWithLockId("renew-branch").
		SetTimeProvider(clock).SetFair(true).SetLeaseExpireAfter(5*time.Minute).SetDisableWatchDog(true))
	require.NoError(t, err)
	require.Error(t, badLock.renewWaiterRegistration(ctx, "owner", waiterModeWrite))
}

func TestWaitFallbacksAndWake(t *testing.T) {
	ctx := context.Background()
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	lock, store, _ := testSubscribeStorageLock(t, "wait-fallback")
	require.ErrorIs(t, lock.waitForLockChange(cancelled, time.Hour), context.Canceled)
	require.ErrorIs(t, lock.waitBeforeRetry(cancelled, "owner", waiterModeWrite, time.Hour), context.Canceled)

	plain := &unwatchableStorage{Storage: store}
	plainLock, err := NewStorageLockWithOptions(plain, NewStorageLockOptionsWithLockId("wait-fallback").
		SetTimeProvider(lock.options.TimeProvider).SetLeaseExpireAfter(5*time.Minute).SetDisableWatchDog(true).
		SetVersionMissRetryInterval(5*time.Millisecond))
	require.NoError(t, err)
	require.NoError(t, plainLock.waitForLockChange(ctx, 5*time.Millisecond))

	errWatch := &watchErrorStorage{mockSubscribeStorage: newMockSubscribeStorage()}
	errLock, err := NewStorageLockWithOptions(errWatch, NewStorageLockOptionsWithLockId("wait-err").
		SetTimeProvider(lock.options.TimeProvider).SetLeaseExpireAfter(5*time.Minute).SetDisableWatchDog(true))
	require.NoError(t, err)
	require.NoError(t, errLock.waitForLockChange(ctx, 5*time.Millisecond))

	nilWatch := &nilWatchStorage{mockSubscribeStorage: newMockSubscribeStorage()}
	nilLock, err := NewStorageLockWithOptions(nilWatch, NewStorageLockOptionsWithLockId("wait-nil").
		SetTimeProvider(lock.options.TimeProvider).SetLeaseExpireAfter(5*time.Minute).SetDisableWatchDog(true))
	require.NoError(t, err)
	require.NoError(t, nilLock.waitForLockChange(ctx, 5*time.Millisecond))

	closedWatch := &closedNotifyStorage{mockSubscribeStorage: newMockSubscribeStorage()}
	closedLock, err := NewStorageLockWithOptions(closedWatch, NewStorageLockOptionsWithLockId("wait-closed").
		SetTimeProvider(lock.options.TimeProvider).SetLeaseExpireAfter(5*time.Minute).SetDisableWatchDog(true))
	require.NoError(t, err)
	require.NoError(t, closedLock.waitForLockChange(ctx, 5*time.Millisecond))
	waitCtx, waitCancel := context.WithCancel(ctx)
	returned := make(chan error, 1)
	go func() { returned <- closedLock.waitForLockChange(waitCtx, time.Hour) }()
	time.Sleep(20 * time.Millisecond)
	waitCancel()
	select {
	case err := <-returned:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("关闭的通知通道没有退回可取消的等待")
	}

	send := &sendWatchStorage{mockSubscribeStorage: newMockSubscribeStorage()}
	sendLock, err := NewStorageLockWithOptions(send, NewStorageLockOptionsWithLockId("wait-send").
		SetTimeProvider(lock.options.TimeProvider).SetLeaseExpireAfter(5*time.Minute).SetDisableWatchDog(true))
	require.NoError(t, err)
	require.NoError(t, sendLock.waitForLockChange(ctx, time.Hour))
	require.NoError(t, finishWatchWake(ctx))
	require.ErrorIs(t, finishWatchWake(cancelled), context.Canceled)

	fired := time.NewTimer(time.Millisecond)
	time.Sleep(5 * time.Millisecond)
	require.NoError(t, waitTimer(ctx, fired))
	stopped := time.NewTimer(time.Hour)
	defer stopped.Stop()
	require.ErrorIs(t, waitTimer(cancelled, stopped), context.Canceled)
	slow := time.NewTimer(time.Hour)
	defer slow.Stop()
	slowCtx, slowCancel := context.WithCancel(ctx)
	go func() {
		time.Sleep(10 * time.Millisecond)
		slowCancel()
	}()
	require.ErrorIs(t, waitTimer(slowCtx, slow), context.Canceled)
}

func TestFairLockErrorPathsAreReturned(t *testing.T) {
	ctx := context.Background()
	clock := newMockSubscribeTimeProvider(time.Now())
	createDown := &errCreateStorage{mockSubscribeStorage: newMockSubscribeStorage()}
	lock, err := NewStorageLockWithOptions(createDown, NewStorageLockOptionsWithLockId("fair-create").
		SetTimeProvider(clock).SetFair(true).SetLeaseExpireAfter(5*time.Minute).SetDisableWatchDog(true))
	require.NoError(t, err)
	require.Error(t, lock.Lock(ctx, "owner"))

	getDown := &failSecondGetStorage{mockSubscribeStorage: newMockSubscribeStorage()}
	fair, err := NewStorageLockWithOptions(getDown, NewStorageLockOptionsWithLockId("fair-get").
		SetTimeProvider(clock).SetFair(true).SetLeaseExpireAfter(5*time.Minute).SetDisableWatchDog(true))
	require.NoError(t, err)
	require.EqualError(t, fair.Lock(ctx, "owner"), "get down")

	reader, _, _ := testRWStorageLock(t, "rlock-empty-owner")
	require.ErrorIs(t, reader.RLock(ctx, ""), ErrOwnerIdEmpty)
}

func TestKnownLeaseBranches(t *testing.T) {
	inner := newMockSubscribeStorage()
	store := &gateStorage{mockSubscribeStorage: inner}
	clock := &flakyClock{mockSubscribeTimeProvider: newMockSubscribeTimeProvider(time.Now())}
	lock := livenessLock(t, store, "known-lease", clock, nil)
	require.NoError(t, lock.Lock(context.Background(), "owner"))
	dog := lock.storageLockWatchDog.(*WatchDogCommonsImpl)
	require.NoError(t, dog.Stop(context.Background()))
	dog.runCtx, dog.runCancel = context.WithCancel(context.Background())
	defer dog.runCancel()

	dog.expiryMu.Lock()
	dog.knownExpiry = time.Time{}
	dog.expiryMu.Unlock()
	past, confirmed := dog.knownLeasePast()
	require.False(t, past)
	require.False(t, confirmed)

	dog.rememberExpiry(time.Unix(10, 0))
	clock.fail.Store(true)
	_, confirmed = dog.knownLeasePast()
	require.False(t, confirmed)
	clock.fail.Store(false)
	past, confirmed = dog.knownLeasePast()
	require.True(t, confirmed)
	require.True(t, past)

	now, err := clock.GetTime(context.Background())
	require.NoError(t, err)
	dog.rememberExpiry(now.Add(time.Hour))
	past, confirmed = dog.knownLeasePast()
	require.True(t, confirmed)
	require.False(t, past)

	dog.rememberExpiry(time.Unix(1, 0))
	store.mu.Lock()
	store.data["known-lease"].OwnerId = "other"
	store.mu.Unlock()
	dog.noteKnownLease()
	dog.expiryMu.Lock()
	require.Equal(t, time.Unix(1, 0), dog.knownExpiry)
	dog.expiryMu.Unlock()

	store.failGet.Store(true)
	dog.noteKnownLease()
	dog.expiryMu.Lock()
	require.Equal(t, time.Unix(1, 0), dog.knownExpiry)
	dog.expiryMu.Unlock()
	store.failGet.Store(false)

	store.mu.Lock()
	store.data["known-lease"].OwnerId = "owner"
	store.data["known-lease"].LockCount = 1
	expires := store.data["known-lease"].LeaseExpireTime
	store.mu.Unlock()
	dog.noteKnownLease()
	dog.expiryMu.Lock()
	require.True(t, expires.Equal(dog.knownExpiry))
	dog.expiryMu.Unlock()
}

type errUpdateStorage struct {
	*mockSubscribeStorage
}

func (s *errUpdateStorage) UpdateWithVersion(ctx context.Context, lockId string, oldVersion, newVersion storage.Version, information *storage.LockInformation) error {
	return errors.New("disk")
}

type errCreateStorage struct {
	*mockSubscribeStorage
}

func (s *errCreateStorage) CreateWithVersion(ctx context.Context, lockId string, version storage.Version, information *storage.LockInformation) error {
	return errors.New("create down")
}

type failSecondGetStorage struct {
	*mockSubscribeStorage
	gets int
}

func (s *failSecondGetStorage) Get(ctx context.Context, lockId string) (string, error) {
	s.gets++
	if s.gets > 1 {
		return "", errors.New("get down")
	}
	return s.mockSubscribeStorage.Get(ctx, lockId)
}

type missOnceStorage struct {
	*mockSubscribeStorage
	left int32
}

func (s *missOnceStorage) UpdateWithVersion(ctx context.Context, lockId string, oldVersion, newVersion storage.Version, information *storage.LockInformation) error {
	if s.left > 0 {
		s.left--
		return ErrVersionMiss
	}
	return s.mockSubscribeStorage.UpdateWithVersion(ctx, lockId, oldVersion, newVersion, information)
}

type sendWatchStorage struct {
	*mockSubscribeStorage
}

func (s *sendWatchStorage) Watch(ctx context.Context, lockId string) (<-chan *storage.LockInformation, error) {
	ch := make(chan *storage.LockInformation, 1)
	ch <- &storage.LockInformation{LockId: lockId}
	return ch, nil
}
