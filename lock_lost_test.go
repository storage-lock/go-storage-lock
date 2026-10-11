package storage_lock

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/storage-lock/go-storage"
)

// takeoverStorage 在第一次续租时把记录改成其他持有者，用来复现持有权丢失。
type takeoverStorage struct {
	*mockSubscribeStorage
	armed bool
}

func (x *takeoverStorage) UpdateWithVersion(ctx context.Context, lockId string, oldVersion, newVersion storage.Version, information *storage.LockInformation) error {
	x.mu.Lock()
	if !x.armed {
		x.armed = true
		existing := x.data[lockId]
		if existing != nil {
			existing.OwnerId = "other-owner"
			existing.Version = newVersion
		}
	}
	x.mu.Unlock()
	return x.mockSubscribeStorage.UpdateWithVersion(ctx, lockId, oldVersion, newVersion, information)
}

func TestOnLockLostWhenOwnershipChanges(t *testing.T) {
	lost := make(chan error, 1)
	options := NewStorageLockOptionsWithLockId("lost-lock").
		SetLeaseExpireAfter(3 * time.Second).
		SetLeaseRefreshInterval(time.Second).
		SetOnLockLost(func(err error) { lost <- err })
	lock, err := NewStorageLockWithOptions(&takeoverStorage{mockSubscribeStorage: newMockSubscribeStorage()}, options)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	if err := lock.Lock(ctx, "owner"); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-lost:
		if !errors.Is(err, ErrLockNotBelongYou) {
			t.Fatalf("lost error=%v", err)
		}
	case <-ctx.Done():
		t.Fatal("OnLockLost was not called")
	}
}
