package storage_lock

import (
	"context"
	"testing"
	"time"
)

func newTestMetadataLock(t *testing.T, lockId string) *StorageLockWithMetadata {
	t.Helper()
	options := NewStorageLockOptionsWithLockId(lockId)
	options.LeaseExpireAfter = 3 * time.Second
	options.LeaseRefreshInterval = time.Second
	options.DisableWatchDog = true
	options.VersionMissRetryInterval = time.Millisecond
	lock, err := NewStorageLockWithOptions(newMockSubscribeStorage(), options)
	if err != nil {
		t.Fatal(err)
	}
	return &StorageLockWithMetadata{StorageLock: lock}
}

func TestLockMetadataPersistsAndCopiesExtra(t *testing.T) {
	ctx := context.Background()
	lock := newTestMetadataLock(t, "metadata-lock")

	extra := map[string]string{"taskId": "task-1", "region": "west"}
	metadata := &LockMetadata{Extra: extra}
	if err := lock.LockWithMetadata(ctx, "owner-1", metadata); err != nil {
		t.Fatal(err)
	}
	extra["taskId"] = "mutated-input"

	got, err := lock.GetMetadata(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.Extra["taskId"] != "task-1" || got.Extra["region"] != "west" {
		t.Fatalf("persisted metadata = %#v", got.Extra)
	}

	got.Extra["taskId"] = "mutated-output"
	again, err := lock.GetMetadata(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if again.Extra["taskId"] != "task-1" {
		t.Fatalf("returned metadata aliases stored record: %#v", again.Extra)
	}

	if err := lock.UnLock(ctx, "owner-1"); err != nil {
		t.Fatal(err)
	}
}

func TestLockMetadataReentryUpdatesMetadataAndPreservesLease(t *testing.T) {
	ctx := context.Background()
	lock := newTestMetadataLock(t, "metadata-reentry-lock")

	if err := lock.LockWithMetadata(ctx, "owner-1", &LockMetadata{Extra: map[string]string{"revision": "1"}}); err != nil {
		t.Fatal(err)
	}
	if err := lock.LockWithMetadata(ctx, "owner-1", &LockMetadata{Extra: map[string]string{"revision": "2"}}); err != nil {
		t.Fatal(err)
	}

	got, err := lock.GetMetadata(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.LockCount != 2 || got.Extra["revision"] != "2" {
		t.Fatalf("metadata after reentry = %+v", got)
	}

	if err := lock.UnLock(ctx, "owner-1"); err != nil {
		t.Fatal(err)
	}
	got, err = lock.GetMetadata(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.LockCount != 1 || got.Extra["revision"] != "2" {
		t.Fatalf("metadata after one unlock = %+v", got)
	}
	if err := lock.UnLock(ctx, "owner-1"); err != nil {
		t.Fatal(err)
	}
}

func TestLockWithMetadataNilUsesRegularLock(t *testing.T) {
	ctx := context.Background()
	lock := newTestMetadataLock(t, "metadata-nil-lock")
	if err := lock.LockWithMetadata(ctx, "owner-1", nil); err != nil {
		t.Fatal(err)
	}
	got, err := lock.GetMetadata(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.Extra != nil {
		t.Fatalf("nil metadata Extra = %#v", got.Extra)
	}
	if err := lock.UnLock(ctx, "owner-1"); err != nil {
		t.Fatal(err)
	}
}
