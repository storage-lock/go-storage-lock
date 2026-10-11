package storage_lock

import (
	"context"
	"errors"
	go_storage "github.com/storage-lock/go-storage"
	"testing"
	"time"
)

func testCapabilitiesLock(t *testing.T, storage *mockSubscribeStorage, id string) *StorageLock {
	t.Helper()
	lock, err := NewStorageLockWithOptions(storage, &StorageLockOptions{
		LockId:                   id,
		LeaseExpireAfter:         3 * time.Second,
		LeaseRefreshInterval:     time.Second,
		DisableWatchDog:          true,
		VersionMissRetryInterval: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	return lock
}

func TestStorageLockTryLockAndCancellation(t *testing.T) {
	ctx := context.Background()
	storage := newMockSubscribeStorage()
	first := testCapabilitiesLock(t, storage, "try-lock")
	second := testCapabilitiesLock(t, storage, "try-lock")
	if err := first.Lock(ctx, "one"); err != nil {
		t.Fatal(err)
	}
	if err := second.TryLock(ctx, "two"); !errors.Is(err, ErrLockBusy) {
		t.Fatalf("TryLock error = %v", err)
	}
	cancelCtx, cancel := context.WithTimeout(ctx, 25*time.Millisecond)
	defer cancel()
	if err := second.Lock(cancelCtx, "two"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Lock cancellation error = %v", err)
	}
	_ = first.UnLock(ctx, "one")
}

func TestStorageLockRenewTTLAndFencingTombstone(t *testing.T) {
	ctx := context.Background()
	storage := newMockSubscribeStorage()
	lock := testCapabilitiesLock(t, storage, "renew-lock")
	if err := lock.Lock(ctx, "owner"); err != nil {
		t.Fatal(err)
	}
	ttl, err := lock.TTL(ctx, "owner")
	if err != nil || ttl <= 0 {
		t.Fatalf("TTL = %v, error = %v", ttl, err)
	}
	firstToken, err := lock.GetFencingToken(ctx, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if err := lock.Renew(ctx, "owner"); err != nil {
		t.Fatal(err)
	}
	if err := lock.UnLock(ctx, "owner"); err != nil {
		t.Fatal(err)
	}
	status, err := lock.Inspect(ctx)
	if err != nil || status.Held {
		t.Fatalf("Inspect after release = %+v, error = %v", status, err)
	}
	if err := lock.Lock(ctx, "next"); err != nil {
		t.Fatal(err)
	}
	secondToken, err := lock.GetFencingToken(ctx, "next")
	if err != nil || secondToken <= firstToken {
		t.Fatalf("fencing tokens = %d, %d; error = %v", firstToken, secondToken, err)
	}
}

func TestStorageLockWithLockReleasesAfterCallback(t *testing.T) {
	ctx := context.Background()
	storage := newMockSubscribeStorage()
	lock := testCapabilitiesLock(t, storage, "with-lock")
	called := false
	if err := lock.WithLock(ctx, func(_ context.Context, token go_storage.Version) error {
		called = token > 0
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("callback did not receive a fencing token")
	}
	status, err := lock.Inspect(ctx)
	if err != nil || status.Held {
		t.Fatalf("lock remained held: %+v, error = %v", status, err)
	}
}
