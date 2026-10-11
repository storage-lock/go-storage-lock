package storage_lock

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/storage-lock/go-events"
	"github.com/storage-lock/go-storage"
)

func TestCoverageDirectEdgeBranches(t *testing.T) {
	ctx := context.Background()
	lock, store := newFaultLock(t, "edges")
	event := events.NewEvent("edges")
	if err := lock.Lock(ctx, "owner"); err != nil {
		t.Fatal(err)
	}
	info, err := lock.getLockInformation(ctx, event, "edges")
	if err != nil {
		t.Fatal(err)
	}
	info.Version = math.MaxUint64
	if _, err := lock.activeLeaseExpiry(ctx, event, info); err != nil {
		t.Fatal(err)
	}
	store.timeValue = info.LeaseExpireTime.Add(time.Second)
	if _, err := lock.activeLeaseExpiry(ctx, event, info); !errors.Is(err, ErrLeaseExpired) {
		t.Fatal(err)
	}
	store.timeErr = errors.New("clock")
	if _, err := lock.activeLeaseExpiry(ctx, event, info); err == nil {
		t.Fatal("clock failure ignored")
	}
	store.timeErr = nil
	store.timeValue = time.Time{}

	if err := lock.renewOnce(ctx, event, "owner", false); err != nil && !errors.Is(err, ErrVersionMiss) {
		t.Fatal(err)
	}
	store.getErr = errors.New("read")
	if err := lock.renewOnce(ctx, event, "owner", false); err == nil {
		t.Fatal("renew ignored read failure")
	}
	store.getErr = nil
	if err := lock.renewOnce(ctx, event, "other", false); !errors.Is(err, ErrLockNotBelongYou) {
		t.Fatal(err)
	}
	info.LockCount = 0
	if err := lock.unlockReleaseByTombstone(ctx, event, "edges", "owner", info, math.MaxUint64); !errors.Is(err, ErrVersionExhausted) {
		t.Fatal(err)
	}
	info.Version = math.MaxUint64
	info.LockCount = 1
	if err := lock.lockExpired(ctx, event, "edges", "owner", time.Now(), info); !errors.Is(err, ErrVersionExhausted) {
		t.Fatal(err)
	}
	lock.lockRollback(ctx, event, "edges", "owner", info)
	if err := lock.lockReentry(ctx, event, "edges", "owner", info); !errors.Is(err, ErrVersionExhausted) {
		t.Fatal(err)
	}

	store.createErr = ErrVersionMiss
	if err := lock.lockNotExists(ctx, event, "missing-edge", "owner", nil); !errors.Is(err, ErrVersionMiss) {
		t.Fatal(err)
	}
	store.createErr = nil
	store.timeErr = errors.New("clock")
	if err := lock.lockNotExists(ctx, event, "missing-edge", "owner", nil); err == nil {
		t.Fatal("create ignored clock failure")
	}
	if err := lock.tryLock(ctx, event, "edges", ""); !errors.Is(err, ErrOwnerIdEmpty) {
		t.Fatal(err)
	}
	store.timeErr = nil

	read, readStore := newFaultLock(t, "read-edges")
	if err := read.RLock(ctx, "reader"); err != nil {
		t.Fatal(err)
	}
	readInfo, err := read.getLockInformation(ctx, event, "read-edges")
	if err != nil {
		t.Fatal(err)
	}
	readInfo.Version = math.MaxUint64
	if err := read.rlockReentry(ctx, event, "read-edges", "reader", readInfo); !errors.Is(err, ErrVersionExhausted) {
		t.Fatal(err)
	}
	readStore.timeErr = errors.New("clock")
	if err := read.tryRLock(ctx, event, "read-edges", "reader"); err == nil {
		t.Fatal("read retry ignored clock")
	}
	readStore.timeErr = nil
	if err := read.tryRUnlock(ctx, event, "read-edges", ""); !errors.Is(err, ErrOwnerIdEmpty) && err != nil {
		t.Fatal(err)
	}
	if err := read.runlockRelease(ctx, event, "read-edges", "reader", readInfo, math.MaxUint64); !errors.Is(err, ErrVersionExhausted) {
		t.Fatal(err)
	}
	readStore.updateErr = errors.New("update")
	if err := read.runlockRelease(ctx, event, "read-edges", "reader", readInfo, readInfo.Version-1); err == nil {
		t.Fatal("read release ignored update")
	}
	readStore.updateErr = nil
	if err := read.RUnLock(ctx, "reader"); err != nil {
		t.Fatal(err)
	}

	empty := ""
	store.mockSubscribeStorage.data["blank"] = &storage.LockInformation{}
	store.mockSubscribeStorage.mu.Lock()
	delete(store.mockSubscribeStorage.data, "blank")
	store.mockSubscribeStorage.mu.Unlock()
	if _, err := lock.getLockInformation(ctx, event, "absent"); !errors.Is(err, ErrLockNotFound) {
		t.Fatal(err)
	}
	_ = empty

	lock.storageLockWatchDog = &brokenWatchDog{}
	lock.stopWatchDog(ctx, event, info)
	if lock.storageLockWatchDog != nil {
		t.Fatal("failed watchdog was retained")
	}

	first, err := lock.Subscribe(ctx, "edges")
	if err != nil {
		t.Fatal(err)
	}
	second, err := lock.Subscribe(ctx, "edges")
	if err != nil {
		t.Fatal(err)
	}
	lock.Unsubscribe("edges", second)
	lock.Unsubscribe("", first)
	lock.Unsubscribe("edges", nil)
	lock.Unsubscribe("edges", first)
	lock.subscribers = nil
	lock.Unsubscribe("edges", first)
	for i := 0; i < 20; i++ {
		lock.notifySubscribers("edges", info)
	}

	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := lock.GetFencingToken(ctx, "owner"); err != nil && !errors.Is(err, ErrLockNotFound) && !errors.Is(err, ErrLeaseExpired) && !errors.Is(err, ErrLockNotBelongYou) {
		t.Fatal(err)
	}
	if err := lock.tryUnlock(canceled, event, "edges", "owner"); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, ErrLeaseExpired) && !errors.Is(err, ErrLockNotFound) {
		t.Fatal(err)
	}
}

func TestCoverageWatchdogRemainingBranches(t *testing.T) {
	ctx := context.Background()
	lock, store := newFaultLock(t, "watch-edges")
	if err := lock.Lock(ctx, "owner"); err != nil {
		t.Fatal(err)
	}
	info, err := lock.getLockInformation(ctx, events.NewEvent("watch-edges"), "watch-edges")
	if err != nil {
		t.Fatal(err)
	}
	info.OwnerId = "other"
	store.mockSubscribeStorage.mu.Lock()
	store.mockSubscribeStorage.data["watch-edges"] = info
	store.mockSubscribeStorage.mu.Unlock()
	dog := NewWatchDogCommonsImpl(ctx, events.NewEvent("watch-edges"), lock, "owner")
	dog.runCtx = ctx
	dog.stop = make(chan struct{})
	if err := dog.refreshLeaseExpiredTime(); !errors.Is(err, ErrLockNotBelongYou) {
		t.Fatal(err)
	}
	info.OwnerId = "owner"
	info.LockCount = 0
	if err := dog.refreshLeaseExpiredTime(); !errors.Is(err, ErrLeaseExpired) {
		t.Fatal(err)
	}
	info.LockCount = 1
	info.LeaseExpireTime = time.Now().Add(-time.Second)
	if err := dog.refreshLeaseExpiredTime(); !errors.Is(err, ErrLeaseExpired) {
		t.Fatal(err)
	}
	store.timeErr = errors.New("clock")
	info.LeaseExpireTime = time.Now().Add(time.Minute)
	if err := dog.refreshLeaseExpiredTime(); err == nil {
		t.Fatal("refresh ignored clock")
	}
}

func TestCoverageFairRetryAndPrefix(t *testing.T) {
	ctx := context.Background()
	store := &faultStorage{mockSubscribeStorage: newMockSubscribeStorage()}
	fair, err := NewFairStorageLockWithOptions(store, testOptions("fair-edge").SetVersionMissRetryInterval(time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	store.getErr = errors.New("unavailable")
	failed, cancel := context.WithTimeout(ctx, 5*time.Millisecond)
	defer cancel()
	if err := fair.Lock(failed, "owner"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("fair retry=%v", err)
	}
	store.getErr = nil
	if err := fair.Lock(ctx, "owner"); err != nil {
		t.Fatal(err)
	}
	fair.queue = []chan struct{}{make(chan struct{}, 1)}
	fair.queue[0] <- struct{}{}
	if err := fair.UnLock(ctx, "owner"); err != nil {
		t.Fatal(err)
	}
	fair.queue = []chan struct{}{make(chan struct{})}
	if err := fair.Close(ctx); err != nil {
		t.Fatal(err)
	}
}
