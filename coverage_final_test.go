package storage_lock

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/storage-lock/go-events"
	"github.com/storage-lock/go-storage"
	storage_events "github.com/storage-lock/go-storage-events"
)

type watchFailStorage struct{ *mockSubscribeStorage }

func (watchFailStorage) Watch(context.Context, string) (<-chan *storage.LockInformation, error) {
	return nil, errors.New("watch unavailable")
}

type raceCreateStorage struct {
	*mockSubscribeStorage
	once sync.Once
}

func (x *raceCreateStorage) CreateWithVersion(ctx context.Context, id string, version storage.Version, info *storage.LockInformation) error {
	var err error
	x.once.Do(func() {
		winner := *info
		winner.OwnerId = "winner"
		err = x.mockSubscribeStorage.CreateWithVersion(ctx, id, version, &winner)
	})
	if err != nil {
		return err
	}
	return ErrLockAlreadyExists
}

type nullGetStorage struct{ storage.Storage }

func (nullGetStorage) Get(context.Context, string) (string, error) { return "null", nil }
func (nullGetStorage) GetName() string                             { return "null-get" }
func (nullGetStorage) Capabilities() []storage.StorageCapability {
	return []storage.StorageCapability{storage.CapabilityCAS, storage.CapabilityReliableTime}
}

type failingTimeProvider struct{ err error }

func (x failingTimeProvider) GetTime(context.Context) (time.Time, error) {
	return time.Time{}, x.err
}

type sequencedTimeProvider struct {
	mu    sync.Mutex
	times []time.Time
	errAt int
	calls int
}

func (x *sequencedTimeProvider) GetTime(context.Context) (time.Time, error) {
	x.mu.Lock()
	defer x.mu.Unlock()
	x.calls++
	if x.errAt > 0 && x.calls >= x.errAt {
		return time.Time{}, errors.New("late clock")
	}
	if len(x.times) == 0 {
		return time.Now(), nil
	}
	next := x.times[0]
	x.times = x.times[1:]
	return next, nil
}

type closedWatchStorage struct {
	channel <-chan *storage.LockInformation
}

func (x closedWatchStorage) Watch(context.Context, string) (<-chan *storage.LockInformation, error) {
	return x.channel, nil
}

func TestCoverageFinalBranches(t *testing.T) {
	ctx := context.Background()
	event := events.NewEvent("final")
	lock, store := newFaultLock(t, "final")
	if err := lock.Lock(ctx, "owner"); err != nil {
		t.Fatal(err)
	}
	info, err := lock.getLockInformation(ctx, event, "final")
	if err != nil {
		t.Fatal(err)
	}

	store.timeErr = errors.New("clock")
	if err := lock.lockExists(ctx, event, "final", "owner", info); err == nil {
		t.Fatal("existing lock ignored clock")
	}
	if err := lock.lockReentry(ctx, event, "final", "owner", info); err == nil {
		t.Fatal("reentry ignored clock")
	}
	if _, err := lock.Inspect(ctx); err == nil {
		t.Fatal("inspect ignored clock")
	}
	if err := lock.tryRUnlock(ctx, event, "final", "owner"); !errors.Is(err, ErrLockNotBelongYou) {
		t.Fatal(err)
	}
	store.timeErr = nil

	info.LeaseExpireTime = time.Now().Add(2 * time.Hour)
	store.timeValue = time.Now()
	if got, err := lock.activeLeaseExpiry(ctx, event, info); err != nil || !got.Equal(info.LeaseExpireTime) {
		t.Fatalf("lease=%v err=%v", got, err)
	}
	store.timeValue = time.Time{}

	info.ReadLock = true
	store.mockSubscribeStorage.mu.Lock()
	store.mockSubscribeStorage.data["final"] = info
	store.mockSubscribeStorage.mu.Unlock()
	if err := lock.renewOnce(ctx, event, "owner", true); err != nil {
		t.Fatal(err)
	}
	info.ReadLock = false
	info.LockCount = 0
	store.mockSubscribeStorage.mu.Lock()
	store.mockSubscribeStorage.data["final"] = info
	store.mockSubscribeStorage.mu.Unlock()
	raw, rawErr := store.Get(ctx, "final")
	if err := lock.renewOnce(ctx, event, "owner", false); !errors.Is(err, ErrLockNotFound) {
		t.Fatalf("err=%v rawErr=%v raw=%s count=%d read=%v", err, rawErr, raw, info.LockCount, info.ReadLock)
	}

	store.createErr = nil
	busy, busyStore := newFaultLock(t, "busy")
	busyStore.createErr = ErrLockAlreadyExists
	if err := busy.TryLock(ctx, "other"); !errors.Is(err, ErrLockBusy) {
		t.Fatal(err)
	}
	plain, plainStore := newFaultLock(t, "plain-create")
	plainErr := errors.New("create failed")
	plainStore.createErr = plainErr
	if err := plain.TryLock(ctx, "other"); !errors.Is(err, plainErr) {
		t.Fatal(err)
	}
	miss, missStore := newFaultLock(t, "miss")
	missStore.createErr = ErrVersionMiss
	if err := miss.TryRLock(ctx, "reader"); !errors.Is(err, ErrLockBusy) {
		t.Fatal(err)
	}

	badOptions := testOptions("bad-options").SetDisableWatchDog(false)
	badOptions.LeaseRefreshInterval = 0
	if _, err := NewStorageLockWithOptions(newMockSubscribeStorage(), badOptions); err == nil {
		t.Fatal("invalid refresh accepted")
	}

	fair, err := NewFairStorageLockWithOptions(newMockSubscribeStorage(), testOptions("fair-final"))
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	go func() {
		close(started)
		_ = fair.Lock(ctx, "racer")
	}()
	<-started
	time.Sleep(10 * time.Millisecond)
	if err := fair.Close(ctx); err != nil {
		t.Fatal(err)
	}

	oldInterfaces, oldHost := listNetworkInterfaces, currentHostname
	defer func() { listNetworkInterfaces, currentHostname = oldInterfaces, oldHost }()
	listNetworkInterfaces = func() ([]net.Interface, error) { return nil, errors.New("no interfaces") }
	if generateMachineDefaultLockIdPrefix() != "" {
		t.Fatal("interface failure should return empty prefix")
	}
	listNetworkInterfaces = func() ([]net.Interface, error) {
		return []net.Interface{{Name: "lo", HardwareAddr: net.HardwareAddr{1, 2, 3, 4, 5, 6}}}, nil
	}
	currentHostname = func() (string, error) { return "", nil }
	prefix := NewOwnerIdGenerator().defaultLockPrefix
	if prefix == "" || len(prefix) > 100 {
		t.Fatal(prefix)
	}

	watchLock, err := NewStorageLockWithOptions(watchFailStorage{newMockSubscribeStorage()}, testOptions("watch-fail"))
	if err != nil {
		t.Fatal(err)
	}
	watchLock.forwardStorageWatch(ctx, watchLock.storage.(storage.LockWatchable), "watch-fail")
	channel, err := watchLock.Subscribe(ctx, "watch-fail")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		watchLock.notifySubscribers("watch-fail", info)
	}
	select {
	case <-channel:
	default:
	}

	dog := NewWatchDogCommonsImpl(ctx, event, lock, "owner")
	dog.runCtx, dog.runCancel = context.WithCancel(ctx)
	dog.stop = make(chan struct{})
	info.OwnerId = "owner"
	info.LockCount = 1
	info.LeaseExpireTime = time.Now().Add(time.Minute)
	info.Version = 1
	store.mockSubscribeStorage.mu.Lock()
	store.mockSubscribeStorage.data["final"] = info
	store.mockSubscribeStorage.mu.Unlock()
	store.timeErr = errors.New("clock")
	if err := dog.refreshLeaseExpiredTime(); err == nil {
		t.Fatal("watchdog ignored clock")
	}
	store.timeErr = nil
	info.Version = ^storage.Version(0)
	store.mockSubscribeStorage.mu.Lock()
	store.mockSubscribeStorage.data["final"] = info
	store.mockSubscribeStorage.mu.Unlock()
	if err := dog.refreshLeaseExpiredTime(); !errors.Is(err, ErrVersionExhausted) {
		t.Fatal(err)
	}
	store.updateErr = errors.New("renew failed")
	info.Version = 1
	store.mockSubscribeStorage.mu.Lock()
	store.mockSubscribeStorage.data["final"] = info
	store.mockSubscribeStorage.mu.Unlock()
	if err := dog.refreshLeaseExpiredTime(); err == nil {
		t.Fatal("watchdog ignored update failure")
	}
	store.updateErr = nil
	dog.done = nil
	if err := dog.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	dog.runCancel = nil
	dog.stopRunCtx()

	deep, deepStore := newFaultLock(t, "deep")
	if err := deep.Lock(ctx, "owner"); err != nil || deep.Lock(ctx, "owner") != nil {
		t.Fatal("reentry setup failed")
	}
	deepStore.timeErr = errors.New("clock")
	if err := deep.UnLock(ctx, "owner"); err == nil {
		t.Fatal("reentry unlock ignored clock")
	}
	deepStore.timeErr = nil
	deepStore.updateErr = ErrVersionMiss
	retryCtx, retryCancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer retryCancel()
	if err := deep.UnLock(retryCtx, "owner"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	deepStore.updateErr = errors.New("update failed")
	if err := deep.UnLock(ctx, "owner"); err == nil {
		t.Fatal("reentry unlock ignored update failure")
	}
	deepStore.updateErr = nil
	if err := deep.UnLock(ctx, "owner"); err != nil || deep.UnLock(ctx, "owner") != nil {
		t.Fatal("deep cleanup failed")
	}

	read, readStore := newFaultLock(t, "read-branches")
	readStore.timeErr = errors.New("clock")
	if err := read.RLock(ctx, "reader"); err == nil {
		t.Fatal("read create ignored clock")
	}
	readStore.timeErr = nil
	if err := read.RLock(ctx, "reader"); err != nil || read.RLock(ctx, "reader") != nil {
		t.Fatal("read reentry setup failed")
	}
	readStore.timeErr = errors.New("clock")
	if err := read.RLock(ctx, "reader"); err == nil {
		t.Fatal("read reentry ignored clock")
	}
	readStore.timeErr = nil
	readStore.updateErr = errors.New("update failed")
	if err := read.RLock(ctx, "reader"); err == nil {
		t.Fatal("read reentry ignored update failure")
	}
	readStore.updateErr = nil
	if err := read.RUnLock(ctx, "reader"); err != nil {
		t.Fatal(err)
	}
	readInfo, err := read.getLockInformation(ctx, event, "read-branches")
	if err != nil {
		t.Fatal(err)
	}
	readInfo.Version = ^storage.Version(0)
	readStore.mockSubscribeStorage.mu.Lock()
	readStore.mockSubscribeStorage.data["read-branches"] = readInfo
	readStore.mockSubscribeStorage.mu.Unlock()
	if err := read.RUnLock(ctx, "reader"); !errors.Is(err, ErrVersionExhausted) {
		t.Fatal(err)
	}
	readInfo.Version = 1
	readStore.mockSubscribeStorage.mu.Lock()
	readStore.mockSubscribeStorage.data["read-branches"] = readInfo
	readStore.mockSubscribeStorage.mu.Unlock()
	readStore.updateErr = ErrVersionMiss
	readRetry, readRetryCancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer readRetryCancel()
	if err := read.RUnLock(readRetry, "reader"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	readStore.updateErr = nil
	if err := read.RUnLock(ctx, "reader"); err != nil {
		t.Fatal(err)
	}

	expired, expiredStore := newFaultLock(t, "expired-read")
	if err := expired.Lock(ctx, "writer"); err != nil || expired.UnLock(ctx, "writer") != nil {
		t.Fatal("tombstone setup failed")
	}
	expiredStore.updateErr = ErrVersionMiss
	if err := expired.TryRLock(ctx, "reader"); !errors.Is(err, ErrLockBusy) {
		t.Fatal(err)
	}
	expiredStore.updateErr = errors.New("update failed")
	if err := expired.RLock(ctx, "reader"); err == nil {
		t.Fatal("read reclaim ignored update failure")
	}
	expiredStore.updateErr = nil

	renew, renewStore := newFaultLock(t, "renew")
	if err := renew.Lock(ctx, "owner"); err != nil {
		t.Fatal(err)
	}
	renewStore.updateErr = ErrVersionMiss
	renewCtx, renewCancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer renewCancel()
	if err := renew.Renew(renewCtx, "owner"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	renewStore.updateErr = nil
	renewStore.timeErr = errors.New("clock")
	if err := renew.renewOnce(ctx, event, "owner", false); err == nil {
		t.Fatal("renew ignored clock")
	}
	if err := renew.Renew(canceled(ctx), ""); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	renewStore.timeErr = nil
	if err := renew.UnLock(ctx, "owner"); err != nil {
		t.Fatal(err)
	}

	rollback, rollbackStore := newFaultLock(t, "rollback")
	if err := rollback.Lock(ctx, "owner"); err != nil {
		t.Fatal(err)
	}
	rollbackInfo, err := rollback.getLockInformation(ctx, event, "rollback")
	if err != nil {
		t.Fatal(err)
	}
	rollbackInfo.Version = ^storage.Version(0)
	rollbackStore.mockSubscribeStorage.mu.Lock()
	rollbackStore.mockSubscribeStorage.data["rollback"] = rollbackInfo
	rollbackStore.mockSubscribeStorage.mu.Unlock()
	rollback.lockRollback(ctx, event, "rollback", "owner", rollbackInfo)

	longPrefix := strings.Repeat("x", 120)
	if got := (&OwnerIdGenerator{defaultLockPrefix: longPrefix[:100]}); len(got.defaultLockPrefix) != 100 {
		t.Fatal(got.defaultLockPrefix)
	}
	oldInterfaces, oldHost = listNetworkInterfaces, currentHostname
	listNetworkInterfaces = net.Interfaces
	currentHostname = func() (string, error) { return strings.Repeat("h", 120), nil }
	if prefix := generateMachineDefaultLockIdPrefix(); len(prefix) <= 100 {
		t.Fatalf("prefix length=%d", len(prefix))
	}
	if len(NewOwnerIdGenerator().defaultLockPrefix) != 100 {
		t.Fatal("long prefix was not truncated")
	}
	listNetworkInterfaces, currentHostname = oldInterfaces, oldHost

	if _, err := lock.GetFencingToken(canceled(ctx), "owner"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := lock.WithLock(ctx, nil); !errors.Is(err, ErrCallbackNil) {
		t.Fatal(err)
	}
	release, releaseStore := newFaultLock(t, "release")
	releaseStore.updateErr = errors.New("release failed")
	err = release.WithLock(ctx, func(context.Context, storage.Version) error { return errors.New("business") })
	if err == nil || !strings.Contains(err.Error(), "business") || !strings.Contains(err.Error(), "release lock") {
		t.Fatal(err)
	}

	tokenLock, tokenStore := newFaultLock(t, "token")
	if err := tokenLock.Lock(ctx, "owner"); err != nil {
		t.Fatal(err)
	}
	tokenStore.getErr = errors.New("inspect failed")
	if _, err := tokenLock.GetFencingToken(ctx, "owner"); err == nil {
		t.Fatal("fencing ignored inspect failure")
	}
	tokenStore.getErr = nil
	if err := tokenLock.UnLock(ctx, "owner"); err != nil {
		t.Fatal(err)
	}

	if err := lock.renewOnce(canceled(ctx), event, "", false); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	versionLock, versionStore := newFaultLock(t, "renew-version")
	if err := versionLock.Lock(ctx, "owner"); err != nil {
		t.Fatal(err)
	}
	versionInfo, err := versionLock.getLockInformation(ctx, event, "renew-version")
	if err != nil {
		t.Fatal(err)
	}
	versionInfo.Version = ^storage.Version(0)
	versionStore.mockSubscribeStorage.mu.Lock()
	versionStore.mockSubscribeStorage.data["renew-version"] = versionInfo
	versionStore.mockSubscribeStorage.mu.Unlock()
	if err := versionLock.renewOnce(ctx, event, "owner", false); !errors.Is(err, ErrVersionExhausted) {
		t.Fatal(err)
	}

	readGet, readGetStore := newFaultLock(t, "read-get")
	readGetStore.getErr = errors.New("get failed")
	if err := readGet.RLock(ctx, "reader"); err == nil {
		t.Fatal("read ignored get failure")
	}
	readVersion, readVersionStore := newFaultLock(t, "read-version")
	if err := readVersion.RLock(ctx, "reader"); err != nil {
		t.Fatal(err)
	}
	readVersionInfo, err := readVersion.getLockInformation(ctx, event, "read-version")
	if err != nil {
		t.Fatal(err)
	}
	readVersionInfo.Version = ^storage.Version(0)
	readVersionStore.mockSubscribeStorage.mu.Lock()
	readVersionStore.mockSubscribeStorage.data["read-version"] = readVersionInfo
	readVersionStore.mockSubscribeStorage.mu.Unlock()
	if err := readVersion.RLock(ctx, "reader"); !errors.Is(err, ErrVersionExhausted) {
		t.Fatal(err)
	}
	readVersionInfo.Version = 1
	readVersionInfo.LockCount = 2
	readVersionStore.mockSubscribeStorage.mu.Lock()
	readVersionStore.mockSubscribeStorage.data["read-version"] = readVersionInfo
	readVersionStore.mockSubscribeStorage.mu.Unlock()
	readVersionStore.timeErr = errors.New("clock")
	if err := readVersion.RUnLock(ctx, "reader"); err == nil {
		t.Fatal("shared release ignored clock")
	}
	readVersionStore.timeErr = nil
	readVersionInfo.Version = ^storage.Version(0)
	readVersionStore.mockSubscribeStorage.mu.Lock()
	readVersionStore.mockSubscribeStorage.data["read-version"] = readVersionInfo
	readVersionStore.mockSubscribeStorage.mu.Unlock()
	if err := readVersion.tryRUnlock(ctx, event, "read-version", "reader"); !errors.Is(err, ErrVersionExhausted) {
		t.Fatal(err)
	}

	writeVersion, writeVersionStore := newFaultLock(t, "write-version")
	if err := writeVersion.Lock(ctx, "owner"); err != nil || writeVersion.Lock(ctx, "owner") != nil {
		t.Fatal("write reentry setup failed")
	}
	writeVersionInfo, err := writeVersion.getLockInformation(ctx, event, "write-version")
	if err != nil {
		t.Fatal(err)
	}
	writeVersionInfo.Version = ^storage.Version(0)
	writeVersionStore.mockSubscribeStorage.mu.Lock()
	writeVersionStore.mockSubscribeStorage.data["write-version"] = writeVersionInfo
	writeVersionStore.mockSubscribeStorage.mu.Unlock()
	if err := writeVersion.UnLock(ctx, "owner"); !errors.Is(err, ErrVersionExhausted) {
		t.Fatal(err)
	}

	rollbackStore.updateErr = errors.New("rollback failed")
	rollbackInfo.Version = 1
	rollback.lockRollback(ctx, event, "rollback", "owner", rollbackInfo)

	full, err := lock.Subscribe(ctx, "final")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 16; i++ {
		lock.notifySubscribers("final", info)
	}
	lock.notifySubscribers("final", info)
	lock.Unsubscribe("final", full)

	closedWatch := make(chan *storage.LockInformation)
	close(closedWatch)
	lock.forwardStorageWatch(ctx, closedWatchStorage{closedWatch}, "final")

	store.timeValue = info.LeaseExpireTime.Add(time.Minute)
	if err := dog.refreshLeaseExpiredTime(); !errors.Is(err, ErrLeaseExpired) {
		t.Fatal(err)
	}
	store.timeValue = time.Time{}

	retry, retryStore := newFaultLock(t, "retry-success")
	if err := retry.Lock(ctx, "owner"); err != nil || retry.Lock(ctx, "owner") != nil {
		t.Fatal("retry setup failed")
	}
	retryStore.mu.Lock()
	retryStore.updateErr = ErrVersionMiss
	retryStore.updates = 0
	retryStore.mu.Unlock()
	go func() {
		for {
			retryStore.mu.Lock()
			ready := retryStore.updates > 0
			if ready {
				retryStore.updateErr = nil
			}
			retryStore.mu.Unlock()
			if ready {
				return
			}
			time.Sleep(time.Millisecond)
		}
	}()
	if err := retry.UnLock(ctx, "owner"); err != nil || retry.UnLock(ctx, "owner") != nil {
		t.Fatal(err)
	}

	raceStore := &raceCreateStorage{mockSubscribeStorage: newMockSubscribeStorage()}
	race, err := NewStorageLockWithOptions(raceStore, testOptions("race-read"))
	if err != nil {
		t.Fatal(err)
	}
	if err := race.RLock(ctx, "reader"); err != nil {
		t.Fatal(err)
	}
	if err := race.RUnLock(ctx, "reader"); err != nil {
		t.Fatal(err)
	}

	tokenStore.updateErr = errors.New("token release failed")
	if err := tokenLock.WithLock(ctx, func(context.Context, storage.Version) error { return nil }); err == nil {
		t.Fatal("withlock ignored token failure")
	}

	held, err := NewFairStorageLockWithOptions(newMockSubscribeStorage(), testOptions("fair-held"))
	if err != nil || held.Lock(ctx, "owner") != nil {
		t.Fatal(err)
	}
	held.queue = []chan struct{}{make(chan struct{})}
	if err := held.Close(ctx); err != nil {
		t.Fatal(err)
	}

	missing, _ := newFaultLock(t, "missing")
	missing.storage = nullGetStorage{}
	missing.storageExecutor = storage_events.NewWithEventSafeExecutor(missing.storage)
	if err := missing.tryUnlock(ctx, event.Fork(), "missing", "owner"); !errors.Is(err, ErrLockNotFound) {
		t.Fatalf("unlock=%v", err)
	}
	if err := missing.tryRUnlock(ctx, event, "missing", "reader"); !errors.Is(err, ErrLockNotFound) {
		t.Fatalf("runlock=%v", err)
	}

	expiredToken, expiredTokenStore := newFaultLock(t, "expired-token")
	if err := expiredToken.Lock(ctx, "owner"); err != nil {
		t.Fatal(err)
	}
	expiredInfo, err := expiredToken.getLockInformation(ctx, event, "expired-token")
	if err != nil {
		t.Fatal(err)
	}
	expiredInfo.LeaseExpireTime = time.Now().Add(-time.Second)
	expiredTokenStore.mockSubscribeStorage.mu.Lock()
	expiredTokenStore.mockSubscribeStorage.data["expired-token"] = expiredInfo
	expiredTokenStore.mockSubscribeStorage.mu.Unlock()
	expiredTokenStore.timeValue = time.Now()
	if _, err := expiredToken.GetFencingToken(ctx, "owner"); !errors.Is(err, ErrLeaseExpired) {
		raw, _ := expiredTokenStore.Get(ctx, "expired-token")
		t.Fatalf("err=%v raw=%s", err, raw)
	}
	canceledToken, _ := newFaultLock(t, "canceled-token")
	canceledLock, cancelLock := context.WithCancel(ctx)
	cancelLock()
	if err := canceledToken.WithLock(canceledLock, func(context.Context, storage.Version) error { return nil }); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}

	busyRead, busyReadStore := newFaultLock(t, "busy-read")
	if err := busyRead.Lock(ctx, "writer"); err != nil {
		t.Fatal(err)
	}
	busyReadStore.updateErr = ErrLockBusy
	busyCtx, busyCancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer busyCancel()
	if err := busyRead.RLock(busyCtx, "reader"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}

	versionRead, versionReadStore := newFaultLock(t, "version-read")
	versionRead.options.VersionMissRetryInterval = time.Millisecond
	if err := versionRead.RLock(ctx, "reader"); err != nil {
		t.Fatal(err)
	}
	versionReadStore.mu.Lock()
	versionReadStore.updateErr = ErrVersionMiss
	versionReadStore.updates = 0
	versionReadStore.mu.Unlock()
	go func() {
		for {
			versionReadStore.mu.Lock()
			ready := versionReadStore.updates > 0
			if ready {
				versionReadStore.updateErr = nil
			}
			versionReadStore.mu.Unlock()
			if ready {
				return
			}
			time.Sleep(time.Millisecond)
		}
	}()
	if err := versionRead.RUnLock(ctx, "reader"); err != nil {
		t.Fatal(err)
	}

	clockLock, _ := newFaultLock(t, "clock-provider")
	if err := clockLock.Lock(ctx, "owner"); err != nil {
		t.Fatal(err)
	}
	clockLock.options.TimeProvider = failingTimeProvider{errors.New("provider clock")}
	clockInfo, err := clockLock.getLockInformation(ctx, event, "clock-provider")
	if err != nil {
		t.Fatal(err)
	}
	clockInfo.ReadLock = true
	if err := clockLock.rlockReentry(ctx, event, "clock-provider", "owner", clockInfo); err == nil {
		t.Fatal("read reentry ignored provider clock")
	}
	dog.lockId = "clock-provider"
	dog.storageLock = clockLock
	if err := dog.refreshLeaseExpiredTime(); err == nil {
		t.Fatal("watchdog ignored provider clock")
	}
}

func TestCoverageLastBranches(t *testing.T) {
	ctx := context.Background()
	retry, store := newFaultLock(t, "read-retry-event")
	retry.options.VersionMissRetryInterval = time.Millisecond
	store.mu.Lock()
	store.createErr = ErrVersionMiss
	store.mu.Unlock()
	go func() {
		time.Sleep(5 * time.Millisecond)
		store.mu.Lock()
		store.createErr = nil
		store.mu.Unlock()
	}()
	if err := retry.RLock(ctx, "reader"); err != nil {
		t.Fatal(err)
	}
	if err := retry.RUnLock(ctx, "reader"); err != nil {
		t.Fatal(err)
	}

	fullCh := make(chan struct{}, 1)
	fullCh <- struct{}{}
	fair, err := NewFairStorageLockWithOptions(newMockSubscribeStorage(), testOptions("fair-full"))
	if err != nil {
		t.Fatal(err)
	}
	fair.queue = []chan struct{}{fullCh}
	if err := fair.Close(ctx); err != nil {
		t.Fatal(err)
	}

	gate := make(chan struct{})
	releaseCh := make(chan struct{})
	slow := &slowLockStorage{mockSubscribeStorage: newMockSubscribeStorage(), gate: gate, release: releaseCh}
	closing, err := NewFairStorageLockWithOptions(slow, testOptions("fair-closing"))
	if err != nil {
		t.Fatal(err)
	}
	doneCh := make(chan error, 1)
	go func() { doneCh <- closing.Lock(ctx, "owner") }()
	<-gate
	if err := closing.Close(ctx); err != nil {
		t.Fatal(err)
	}
	close(releaseCh)
	if err := <-doneCh; err == nil || !strings.Contains(err.Error(), "closed") {
		t.Fatalf("close during acquire=%v", err)
	}

	blocked := &slowLockStorage{mockSubscribeStorage: newMockSubscribeStorage(), gate: make(chan struct{}), release: make(chan struct{}), fail: true}
	failed, err := NewFairStorageLockWithOptions(blocked, testOptions("fair-closing-failed"))
	if err != nil {
		t.Fatal(err)
	}
	failedDone := make(chan error, 1)
	go func() { failedDone <- failed.Lock(ctx, "owner") }()
	<-blocked.gate
	if err := failed.Close(ctx); err != nil {
		t.Fatal(err)
	}
	close(blocked.release)
	if err := <-failedDone; err == nil || !strings.Contains(err.Error(), "closed") {
		t.Fatalf("close during failed acquire=%v", err)
	}

	acquired, err := NewStorageLockWithOptions(newMockSubscribeStorage(), testOptions("with-after-acquire"))
	if err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(ctx)
	if err := acquired.Lock(canceled, "preheld"); err != nil {
		t.Fatal(err)
	}
	cancel()
	if err := acquired.WithLock(canceled, func(context.Context, storage.Version) error { return nil }); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}

	tokenStore := &scriptedStorage{mockSubscribeStorage: newMockSubscribeStorage()}
	tokenFail, err := NewStorageLockWithOptions(tokenStore, testOptions("token-read-fails"))
	if err != nil {
		t.Fatal(err)
	}
	tokenStore.gets = []func() (string, error){
		func() (string, error) { return "", ErrLockNotFound },
		func() (string, error) { return "", errors.New("token read failed") },
		func() (string, error) { return "", errors.New("token read failed") },
	}
	if err := tokenFail.WithLock(ctx, func(context.Context, storage.Version) error { return nil }); err == nil || !strings.Contains(err.Error(), "token read failed") {
		t.Fatalf("token read=%v", err)
	}
	cancelCtx, cancelBetween := context.WithCancel(ctx)
	cancelStore := &scriptedStorage{mockSubscribeStorage: newMockSubscribeStorage()}
	cancelLock, err := NewStorageLockWithOptions(cancelStore, testOptions("cancel-between"))
	if err != nil {
		t.Fatal(err)
	}
	cancelStore.gets = []func() (string, error){
		func() (string, error) { return "", ErrLockNotFound },
		func() (string, error) {
			cancelBetween()
			return cancelStore.mockSubscribeStorage.Get(context.Background(), "cancel-between")
		},
	}
	if err := cancelLock.WithLock(cancelCtx, func(context.Context, storage.Version) error { return errors.New("callback ran") }); !errors.Is(err, context.Canceled) {
		t.Fatalf("between cancel=%v", err)
	}

	base := time.Now()
	watchClock := &sequencedTimeProvider{times: []time.Time{base, base.Add(time.Second)}, errAt: 3}
	watchLock, err := NewStorageLockWithOptions(newMockSubscribeStorage(), testOptions("watch-clock").SetTimeProvider(watchClock))
	if err != nil {
		t.Fatal(err)
	}
	if err := watchLock.Lock(ctx, "owner"); err != nil {
		t.Fatal(err)
	}
	dog := NewWatchDogCommonsImpl(ctx, events.NewEvent("watch-clock"), watchLock, "owner")
	dog.runCtx = ctx
	if err := dog.refreshLeaseExpiredTime(); err == nil || !strings.Contains(err.Error(), "late clock") {
		t.Fatalf("watchdog clock=%v", err)
	}

	snapshot, err := NewStorageLockWithOptions(newMockSubscribeStorage(), testOptions("subscriber-snapshot"))
	if err != nil {
		t.Fatal(err)
	}
	if err := snapshot.Lock(ctx, "owner"); err != nil {
		t.Fatal(err)
	}
	subscription, err := snapshot.Subscribe(ctx, "subscriber-snapshot")
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-subscription:
	case <-time.After(time.Second):
		t.Fatal("initial snapshot missing")
	}
	snapshot.subscribeMu.Lock()
	var target chan *storage.LockInformation
	if len(snapshot.subscribers["subscriber-snapshot"]) != 1 {
		t.Fatalf("subscribers=%d", len(snapshot.subscribers["subscriber-snapshot"]))
	}
	for _, candidate := range snapshot.subscribers["subscriber-snapshot"] {
		if fmt.Sprintf("%p", candidate) == fmt.Sprintf("%p", subscription) {
			target = candidate
		}
	}
	for len(target) < cap(target) {
		target <- &storage.LockInformation{LockId: "subscriber-snapshot"}
	}
	snapshot.subscribeMu.Unlock()
	time.Sleep(20 * time.Millisecond)
	doneSnapshot := make(chan struct{})
	go func() {
		snapshot.notifySubscribers("subscriber-snapshot", &storage.LockInformation{LockId: "extra"})
		close(doneSnapshot)
	}()
	select {
	case <-doneSnapshot:
	case <-time.After(time.Second):
		t.Fatal("full subscriber notification blocked")
	}
	snapshot.Unsubscribe("subscriber-snapshot", subscription)
}

type slowLockStorage struct {
	*mockSubscribeStorage
	gate    chan struct{}
	release chan struct{}
	once    sync.Once
	fail    bool
}

func (x *slowLockStorage) CreateWithVersion(ctx context.Context, id string, version storage.Version, info *storage.LockInformation) error {
	x.once.Do(func() { close(x.gate) })
	<-x.release
	if x.fail {
		return errors.New("acquire failed")
	}
	return x.mockSubscribeStorage.CreateWithVersion(ctx, id, version, info)
}

type scriptedStorage struct {
	*mockSubscribeStorage
	gets []func() (string, error)
}

func (x *scriptedStorage) Get(ctx context.Context, id string) (string, error) {
	if len(x.gets) == 0 {
		return x.mockSubscribeStorage.Get(ctx, id)
	}
	next := x.gets[0]
	x.gets = x.gets[1:]
	return next()
}

func canceled(parent context.Context) context.Context {
	ctx, cancel := context.WithCancel(parent)
	cancel()
	return ctx
}
