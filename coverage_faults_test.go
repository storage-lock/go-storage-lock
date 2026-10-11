package storage_lock

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/storage-lock/go-events"
	"github.com/storage-lock/go-storage"
)

type faultStorage struct {
	*mockSubscribeStorage
	mu        sync.Mutex
	getErr    error
	updateErr error
	createErr error
	timeErr   error
	timeValue time.Time
	updates   int
}

func (x *faultStorage) Get(ctx context.Context, id string) (string, error) {
	x.mu.Lock()
	err := x.getErr
	x.mu.Unlock()
	if err != nil {
		return "", err
	}
	return x.mockSubscribeStorage.Get(ctx, id)
}

func (x *faultStorage) GetTime(ctx context.Context) (time.Time, error) {
	x.mu.Lock()
	err, value := x.timeErr, x.timeValue
	x.mu.Unlock()
	if err != nil {
		return time.Time{}, err
	}
	if !value.IsZero() {
		return value, nil
	}
	return x.mockSubscribeStorage.GetTime(ctx)
}

func (x *faultStorage) CreateWithVersion(ctx context.Context, id string, version storage.Version, info *storage.LockInformation) error {
	x.mu.Lock()
	err := x.createErr
	x.mu.Unlock()
	if err != nil {
		return err
	}
	return x.mockSubscribeStorage.CreateWithVersion(ctx, id, version, info)
}

func (x *faultStorage) UpdateWithVersion(ctx context.Context, id string, oldVersion, newVersion storage.Version, info *storage.LockInformation) error {
	x.mu.Lock()
	x.updates++
	err := x.updateErr
	x.mu.Unlock()
	if err != nil {
		return err
	}
	return x.mockSubscribeStorage.UpdateWithVersion(ctx, id, oldVersion, newVersion, info)
}

func newFaultLock(t *testing.T, id string) (*StorageLock, *faultStorage) {
	t.Helper()
	store := &faultStorage{mockSubscribeStorage: newMockSubscribeStorage()}
	lock, err := NewStorageLockWithOptions(store, testOptions(id))
	if err != nil {
		t.Fatal(err)
	}
	return lock, store
}

func TestCoverageStorageFaultBranches(t *testing.T) {
	ctx := context.Background()
	lock, store := newFaultLock(t, "faults")
	store.getErr = errors.New("get failed")
	if err := lock.Lock(ctx, "owner"); err == nil {
		t.Fatal("get failure ignored")
	}
	if _, err := lock.Inspect(ctx); err == nil {
		t.Fatal("inspect ignored get failure")
	}
	held, err := lock.IsLocked(ctx)
	if held || err == nil {
		t.Fatalf("islocked held=%v err=%v", held, err)
	}
	if _, err := lock.GetOwner(ctx); err == nil {
		t.Fatal("owner ignored get failure")
	}
	store.getErr = nil
	if _, err := store.Get(ctx, "faults"); !errors.Is(err, ErrLockNotFound) {
		t.Fatalf("leftover record=%v", err)
	}

	if err := lock.Lock(ctx, "owner"); err != nil {
		t.Fatal(err)
	}
	store.updateErr = errors.New("update failed")
	if err := lock.Lock(ctx, "owner"); err == nil {
		t.Fatal("reentry update failure ignored")
	}
	if err := lock.Renew(ctx, "owner"); err == nil {
		t.Fatal("renew update failure ignored")
	}
	store.updateErr = nil
	if err := lock.Unlock(ctx, "owner"); err != nil {
		t.Fatal(err)
	}

	readLock, readStore := newFaultLock(t, "read-create")
	createErr := errors.New("create failed")
	readStore.createErr = createErr
	if err := readLock.RLock(ctx, "reader"); !errors.Is(err, createErr) {
		t.Fatalf("read create failure=%v", err)
	}
	store.createErr = nil
	if err := lock.RLock(ctx, "reader-a"); err != nil || lock.RLock(ctx, "reader-b") != nil {
		t.Fatal("shared read failed")
	}
	store.updateErr = ErrVersionMiss
	canceled, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()
	if err := lock.RUnLock(canceled, "reader-a"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("read unlock retry=%v", err)
	}
	store.updateErr = errors.New("read update failed")
	if err := lock.RUnLock(ctx, "reader-b"); err == nil {
		t.Fatal("read unlock ignored update failure")
	}
	store.updateErr = nil
	if err := lock.RUnLock(ctx, "reader-a"); err != nil || lock.RUnLock(ctx, "reader-b") != nil {
		t.Fatal("cleanup read locks failed")
	}

	if err := lock.Lock(ctx, "writer"); err != nil {
		t.Fatal(err)
	}
	store.updateErr = ErrVersionMiss
	writeCanceled, writeCancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer writeCancel()
	if err := lock.Unlock(writeCanceled, "writer"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("write unlock retry=%v", err)
	}
	store.updateErr = nil
	if err := lock.Unlock(ctx, "writer"); err != nil {
		t.Fatal(err)
	}
}

func TestCoverageReadReclaimAndFairQueue(t *testing.T) {
	ctx := context.Background()
	lock, store := newFaultLock(t, "reclaim")
	expired := time.Now().Add(-time.Minute)
	store.timeValue = expired
	info := &storage.LockInformation{LockId: "reclaim", OwnerId: "old", Version: 4, LockCount: 1, LeaseExpireTime: expired}
	if err := store.CreateWithVersion(ctx, "reclaim", info.Version, info); err != nil {
		t.Fatal(err)
	}
	store.timeValue = time.Now()
	if err := lock.RLock(ctx, "reader"); err != nil {
		t.Fatal(err)
	}
	if err := lock.RUnLock(ctx, "reader"); err != nil {
		t.Fatal(err)
	}

	fair, err := NewFairStorageLockWithOptions(newMockSubscribeStorage(), testOptions("fair-queue"))
	if err != nil {
		t.Fatal(err)
	}
	if err := fair.Lock(ctx, "first"); err != nil {
		t.Fatal(err)
	}
	waiting, waitCancel := context.WithTimeout(ctx, 30*time.Millisecond)
	defer waitCancel()
	if err := fair.Lock(waiting, "second"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("queued lock=%v", err)
	}
	if err := fair.Lock(ctx, "first"); err != nil || fair.UnLock(ctx, "first") != nil || fair.UnLock(ctx, "first") != nil {
		t.Fatal("fair reentry failed")
	}
	if err := fair.UnLock(ctx, "missing"); err == nil {
		t.Fatal("unknown fair owner unlocked")
	}
	fair.Close(ctx)
	if fair.queue != nil {
		t.Fatal("close leaked queue")
	}
}

func TestCoverageWatchdogRefreshFailures(t *testing.T) {
	ctx := context.Background()
	lock, store := newFaultLock(t, "refresh")
	dog := NewWatchDogCommonsImpl(ctx, events.NewEvent("refresh"), lock, "owner")
	dog.runCtx = ctx
	dog.stop = make(chan struct{})
	dog.notifyLockLost(nil)
	lock.options.OnLockLost = func(error) { panic("listener failed") }
	func() {
		defer func() { _ = recover() }()
		dog.notifyLockLost(errors.New("lost"))
	}()

	if err := lock.Lock(ctx, "owner"); err != nil {
		t.Fatal(err)
	}
	store.getErr = ErrLockNotFound
	if err := dog.refreshLeaseExpiredTime(); !errors.Is(err, ErrLockNotFound) {
		t.Fatal(err)
	}
	store.getErr = errors.New("temporary")
	if err := dog.refreshLeaseExpiredTime(); err == nil {
		t.Fatal("temporary refresh error ignored")
	}
	store.getErr = nil
	store.updateErr = errors.New("refresh update")
	if err := dog.refreshLeaseExpiredTime(); err == nil {
		t.Fatal("refresh update failure ignored")
	}
	stopCtx, cancel := context.WithCancel(ctx)
	cancel()
	dog.done = make(chan struct{})
	if err := dog.Stop(stopCtx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestCoverageExistingRecordBranches(t *testing.T) {
	ctx := context.Background()
	store := newMockSubscribeStorage()
	info := getTestLockInformation(t, 7)
	info.LockId = "existing"
	if err := store.CreateWithVersion(ctx, info.LockId, info.Version, info); err != nil {
		t.Fatal(err)
	}
	testEnsureLockNotExists(t, store, info.LockId)
	if _, err := store.Get(ctx, info.LockId); !errors.Is(err, ErrLockNotFound) {
		t.Fatal(err)
	}
}
