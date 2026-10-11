package storage_lock

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/golang-infrastructure/go-iterator"
	"github.com/storage-lock/go-events"
	"github.com/storage-lock/go-storage"
)

type failingTimeStorage struct{ *mockSubscribeStorage }

func (x *failingTimeStorage) GetTime(context.Context) (time.Time, error) {
	return time.Time{}, errors.New("clock unavailable")
}

type missingCapabilityStorage struct{}

func (missingCapabilityStorage) GetName() string                           { return "missing" }
func (missingCapabilityStorage) Capabilities() []storage.StorageCapability { return nil }
func (missingCapabilityStorage) Init(context.Context) error                { return nil }
func (missingCapabilityStorage) Get(context.Context, string) (string, error) {
	return "", ErrLockNotFound
}
func (missingCapabilityStorage) GetTime(context.Context) (time.Time, error) {
	return time.Now(), nil
}
func (missingCapabilityStorage) Close(context.Context) error { return nil }
func (missingCapabilityStorage) CreateWithVersion(context.Context, string, storage.Version, *storage.LockInformation) error {
	return nil
}
func (missingCapabilityStorage) UpdateWithVersion(context.Context, string, storage.Version, storage.Version, *storage.LockInformation) error {
	return nil
}
func (missingCapabilityStorage) DeleteWithVersion(context.Context, string, storage.Version, *storage.LockInformation) error {
	return nil
}
func (missingCapabilityStorage) List(context.Context) (iterator.Iterator[*storage.LockInformation], error) {
	return &emptyLockIterator{}, nil
}

type brokenWatchDog struct{ events.Eventable }

func (brokenWatchDog) Name() string                { return "broken" }
func (brokenWatchDog) GetID() string               { return "broken-id" }
func (brokenWatchDog) Start(context.Context) error { return errors.New("watchdog start failed") }
func (brokenWatchDog) Stop(context.Context) error  { return errors.New("watchdog stop failed") }

type brokenFactory struct{}

func (brokenFactory) Name() string { return "broken-factory" }
func (brokenFactory) NewWatchDog(context.Context, *events.Event, *StorageLock, string) (WatchDog, error) {
	return nil, errors.New("watchdog create failed")
}

func testOptions(id string) *StorageLockOptions {
	return NewStorageLockOptionsWithLockId(id).SetLeaseExpireAfter(3 * time.Second).SetLeaseRefreshInterval(time.Second).SetDisableWatchDog(true)
}

func TestCoverageOptionSettersAndValidation(t *testing.T) {
	options := NewStorageLockOptionsWithLockId("coverage").
		SetEventListeners(nil).
		AddEventListeners(nil).
		SetWatchDogFactory(NewWatchDogFactoryCommonsImpl()).
		SetSkipCapabilityCheck(true).
		SetSkipLeaseMarginCheck(true).
		SetDisableWatchDog(true).
		SetUnlockTimeout(time.Second).
		SetMaxReentrantDepth(3)
	if options.WatchDogFactory.Name() == "" || options.MaxReentrantDepth != 3 || !options.DisableWatchDog {
		t.Fatal("option setters did not retain values")
	}
	for _, invalid := range []*StorageLockOptions{
		nil,
		testOptions("x").SetVersionMissRetryInterval(-time.Millisecond),
		testOptions("x").SetUnlockTimeout(-time.Millisecond),
		{LeaseExpireAfter: 3 * time.Second, LeaseRefreshInterval: time.Second},
		testOptions("x").SetLeaseExpireAfter(time.Second),
	} {
		if err := checkStorageLockOptions(invalid); err == nil {
			t.Fatalf("invalid options accepted: %+v", invalid)
		}
	}
	if _, err := NewStorageLock(nil, "x"); !errors.Is(err, ErrStorageNil) {
		t.Fatal(err)
	}
	if _, err := NewStorageLockWithOptions(newMockSubscribeStorage(), nil); !errors.Is(err, ErrInvalidOptions) {
		t.Fatal(err)
	}
	if _, err := NewStorageLockWithOptions(missingCapabilityStorage{}, testOptions("missing")); !errors.Is(err, ErrStorageCapabilityMissing) {
		t.Fatal(err)
	}
	if _, err := NewStorageLockWithOptions(missingCapabilityStorage{}, testOptions("missing").SetTimeProvider(&mockSubscribeTimeProvider{now: time.Now()})); !errors.Is(err, ErrStorageCapabilityMissing) {
		t.Fatal(err)
	}
}

func TestCoverageAliasesFencingAndWithLock(t *testing.T) {
	ctx := context.Background()
	lock, err := NewStorageLockWithOptions(newMockSubscribeStorage(), testOptions("aliases"))
	if err != nil {
		t.Fatal(err)
	}
	if err := lock.Unlock(ctx, ""); !errors.Is(err, ErrOwnerIdEmpty) {
		t.Fatal(err)
	}
	if err := lock.TryWLock(ctx, ""); !errors.Is(err, ErrOwnerIdEmpty) {
		t.Fatal(err)
	}
	if err := lock.TryRLock(ctx, ""); !errors.Is(err, ErrOwnerIdEmpty) {
		t.Fatal(err)
	}
	if err := lock.Lock(ctx, "owner"); err != nil {
		t.Fatal(err)
	}
	if err := lock.TryWLock(ctx, "owner"); err != nil || lock.Unlock(ctx, "owner") != nil {
		t.Fatal("write alias failed")
	}
	if err := lock.TryRLock(ctx, "reader"); !errors.Is(err, ErrLockBusy) {
		t.Fatalf("read alias=%v", err)
	}
	if err := lock.Unlock(ctx, "owner"); err != nil || lock.TryRLock(ctx, "reader") != nil || lock.RUnLock(ctx, "reader") != nil {
		t.Fatal("read alias after release failed")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := lock.GetFencingToken(canceled, "owner"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := lock.GetFencingToken(ctx, "missing"); !errors.Is(err, ErrLockNotFound) {
		t.Fatal(err)
	}
	if err := lock.Lock(ctx, "owner"); err != nil {
		t.Fatal(err)
	}
	if _, err := lock.GetFencingToken(ctx, "other"); !errors.Is(err, ErrLockNotBelongYou) {
		t.Fatal(err)
	}
	if err := lock.WithLock(ctx, nil); !errors.Is(err, ErrCallbackNil) {
		t.Fatal(err)
	}
	canceledLock, cancelLock := context.WithCancel(ctx)
	cancelLock()
	if err := lock.WithLock(canceledLock, func(context.Context, storage.Version) error { return nil }); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	var seen storage.Version
	if err := lock.WithLock(ctx, func(_ context.Context, token storage.Version) error {
		seen = token
		return errors.New("business")
	}); err == nil || seen == 0 {
		t.Fatal(err)
	}
}

func TestCoverageLeaseQueryAndRenewalFailures(t *testing.T) {
	ctx := context.Background()
	lock, err := NewStorageLockWithOptions(newMockSubscribeStorage(), testOptions("lease"))
	if err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := lock.Inspect(canceled); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := lock.TTL(ctx, ""); !errors.Is(err, ErrOwnerIdEmpty) {
		t.Fatal(err)
	}
	if err := lock.Renew(ctx, ""); !errors.Is(err, ErrOwnerIdEmpty) {
		t.Fatal(err)
	}
	if err := lock.Lock(ctx, "owner"); err != nil {
		t.Fatal(err)
	}
	if _, err := lock.TTL(ctx, "other"); !errors.Is(err, ErrLockNotBelongYou) {
		t.Fatal(err)
	}
	if err := lock.Unlock(ctx, "owner"); err != nil {
		t.Fatal(err)
	}
	if _, err := lock.TTL(ctx, "owner"); !errors.Is(err, ErrLockNotFound) && !errors.Is(err, ErrLeaseExpired) {
		t.Fatal(err)
	}
	if _, err := nextVersion(math.MaxUint64); !errors.Is(err, ErrVersionExhausted) {
		t.Fatal(err)
	}
	if err := checkExclusiveOwner(&storage.LockInformation{LockCount: 0}, "owner"); !errors.Is(err, ErrLockNotFound) {
		t.Fatal(err)
	}
	done, stop := context.WithCancel(ctx)
	stop()
	if err := waitContext(done, time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestCoverageLockFailureBranches(t *testing.T) {
	ctx := context.Background()
	storage := &failingTimeStorage{newMockSubscribeStorage()}
	lock, err := NewStorageLockWithOptions(storage, testOptions("time-failure"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lock.getLeaseExpireTime(ctx, events.NewEvent("time-failure")); err == nil {
		t.Fatal("clock failure was ignored")
	}
	if err := lock.Lock(ctx, "owner"); err == nil {
		t.Fatal("lock succeeded without a clock")
	}
	if err := lock.TryLock(ctx, "owner"); err == nil {
		t.Fatal("try lock succeeded without a clock")
	}

	normal, err := NewStorageLockWithOptions(newMockSubscribeStorage(), testOptions("rollback"))
	if err != nil {
		t.Fatal(err)
	}
	normal.options.WatchDogFactory = brokenFactory{}
	normal.options.DisableWatchDog = false
	if err := normal.Lock(ctx, "owner"); err == nil {
		t.Fatal("broken watchdog factory should fail lock")
	}
	held, err := normal.IsLocked(ctx)
	if err != nil || held {
		t.Fatalf("rollback left lock held=%v err=%v", held, err)
	}

	startFailure, err := NewStorageLockWithOptions(newMockSubscribeStorage(), testOptions("start-failure").SetDisableWatchDog(false))
	if err != nil {
		t.Fatal(err)
	}
	startFailure.options.WatchDogFactory = NewWatchDogFactoryFuncWrapper("wrapper", func(ctx context.Context, e *events.Event, lock *StorageLock, owner string) (WatchDog, error) {
		dog := NewWatchDogCommonsImpl(ctx, e, lock, owner)
		return &brokenWatchDog{Eventable: dog}, nil
	})
	if startFailure.options.WatchDogFactory.Name() != "wrapper" {
		t.Fatal("factory wrapper name lost")
	}
	if err := startFailure.Lock(ctx, "owner"); err == nil {
		t.Fatal("watchdog start failure should roll back")
	}
}

func TestCoverageFairLockAndWatchDogHelpers(t *testing.T) {
	ctx := context.Background()
	storage := newMockSubscribeStorage()
	fair, err := NewFairStorageLock(storage, "fair")
	if err != nil {
		t.Fatal(err)
	}
	if fair.GetStorageLock() == nil {
		t.Fatal("missing inner lock")
	}
	if _, err := NewFairStorageLock(nil, "bad"); err == nil {
		t.Fatal("nil storage accepted")
	}
	if _, err := NewFairStorageLockWithOptions(storage, nil); err == nil {
		t.Fatal("nil options accepted")
	}
	if err := fair.Lock(ctx, "owner"); err != nil || fair.UnLock(ctx, "owner") != nil {
		t.Fatal(err)
	}
	if err := fair.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if err := fair.Lock(ctx, "owner"); err == nil {
		t.Fatal("closed fair lock accepted acquisition")
	}

	options := testOptions("dog").SetDisableWatchDog(false)
	lock, err := NewStorageLockWithOptions(newMockSubscribeStorage(), options)
	if err != nil {
		t.Fatal(err)
	}
	dog := NewWatchDogCommonsImpl(ctx, events.NewEvent("dog"), lock, "owner")
	if dog.Name() == "" {
		t.Fatal("watchdog has no name")
	}
	dog.SetEvent(events.NewEvent("replacement"))
	dog.stop = make(chan struct{})
	dog.runCtx, dog.runCancel = context.WithCancel(ctx)
	dog.stopInternal()
	if dog.isRunning.Load() {
		t.Fatal("internal stop did not clear running state")
	}
	if NewWatchDogFactoryCommonsImpl().Name() == "" {
		t.Fatal("factory name empty")
	}
	prefix := generateMachineDefaultLockIdPrefix()
	if prefix == "" {
		t.Fatal("machine prefix empty")
	}
	getTestLockInformation(t)
	testEnsureLockNotExists(t, newMockSubscribeStorage(), "absent")
}
