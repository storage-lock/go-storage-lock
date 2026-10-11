package storage_lock

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestLockQueryAndReentrantDepth(t *testing.T) {
	ctx := context.Background()
	storage := newMockSubscribeStorage()
	options := &StorageLockOptions{
		LockId:               "query-depth",
		LeaseExpireAfter:     3 * time.Second,
		LeaseRefreshInterval: time.Second,
		DisableWatchDog:      true,
		MaxReentrantDepth:    2,
	}
	lock, err := NewStorageLockWithOptions(storage, options)
	if err != nil {
		t.Fatal(err)
	}

	held, err := lock.IsLocked(ctx)
	if err != nil || held {
		t.Fatalf("empty lock held=%v err=%v", held, err)
	}
	if _, err := lock.GetOwner(ctx); !errors.Is(err, ErrLockNotFound) {
		t.Fatalf("empty owner error=%v", err)
	}

	if err := lock.Lock(ctx, "owner"); err != nil {
		t.Fatal(err)
	}
	if err := lock.Lock(ctx, "owner"); err != nil {
		t.Fatal(err)
	}
	if err := lock.Lock(ctx, "owner"); !errors.Is(err, ErrMaxReentrantDepth) {
		t.Fatalf("third lock error=%v", err)
	}

	held, err = lock.IsLocked(ctx)
	if err != nil || !held {
		t.Fatalf("held=%v err=%v", held, err)
	}
	owner, err := lock.GetOwner(ctx)
	if err != nil || owner != "owner" {
		t.Fatalf("owner=%q err=%v", owner, err)
	}
	status, err := lock.Inspect(ctx)
	if err != nil || status.Information.LockCount != 2 {
		t.Fatalf("status=%+v err=%v", status, err)
	}
}

func TestNegativeReentrantDepthRejected(t *testing.T) {
	options := NewStorageLockOptionsWithLockId("negative-depth")
	options.MaxReentrantDepth = -1
	if err := checkStorageLockOptions(options); !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("error=%v", err)
	}
}
