package storage_lock

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/storage-lock/go-events"
	"github.com/storage-lock/go-storage"
	"github.com/stretchr/testify/require"
)

func newBranchClock() (*mockSubscribeStorage, *mockSubscribeTimeProvider) {
	return newMockSubscribeStorage(), newMockSubscribeTimeProvider(time.Now())
}

func openBranchLock(t *testing.T, store storage.Storage, clock *mockSubscribeTimeProvider, id string, watch bool, maxDepth int, lost *atomic.Bool) *StorageLock {
	t.Helper()
	options := NewStorageLockOptionsWithLockId(id).
		SetTimeProvider(clock).
		SetVersionMissRetryInterval(5 * time.Millisecond).
		SetDisableWatchDog(!watch)
	if watch {
		options.SetLeaseExpireAfter(3 * time.Second).SetLeaseRefreshInterval(time.Second)
	} else {
		options.SetLeaseExpireAfter(5 * time.Minute)
	}
	if maxDepth > 0 {
		options.SetMaxReentrantDepth(maxDepth)
	}
	if lost != nil {
		options.SetOnLockLost(func(error) { lost.Store(true) })
	}
	lock, err := NewStorageLockWithOptions(store, options)
	require.NoError(t, err)
	if watch {
		t.Cleanup(func() {
			lock.stopWatchDog(context.Background(), events.NewEvent(id), nil)
		})
	}
	return lock
}

func lockSnap(t *testing.T, lock *StorageLock) storage.LockInformation {
	t.Helper()
	status, err := lock.Inspect(context.Background())
	require.NoError(t, err)
	return status.Information
}

func requireStable(t *testing.T, lock *StorageLock, before storage.LockInformation, owners ...string) {
	t.Helper()
	after := lockSnap(t, lock)
	require.Equal(t, before.Version, after.Version)
	require.Equal(t, before.OwnerId, after.OwnerId)
	require.Equal(t, before.LockCount, after.LockCount)
	require.True(t, before.LeaseExpireTime.Equal(after.LeaseExpireTime), "lease moved from %s to %s", before.LeaseExpireTime, after.LeaseExpireTime)
	require.Equal(t, before.ReadLock, after.ReadLock)
	require.Equal(t, before.WriteIntentOwner, after.WriteIntentOwner)
	require.Equal(t, len(before.Waiters), len(after.Waiters))
	for _, owner := range owners {
		require.Equal(t, before.ReadDepth(owner), after.ReadDepth(owner), owner)
	}
}

func requireAbsent(t *testing.T, store *mockSubscribeStorage, id string) {
	t.Helper()
	_, err := store.Get(context.Background(), id)
	require.ErrorIs(t, err, ErrLockNotFound)
}

// scriptedCAS 先按脚本拒绝一次条件写，再把后面的写入交给真实版本检查。
type scriptedCAS struct {
	*mockSubscribeStorage
	gate        sync.Mutex
	failCreates int
	failUpdates int
	createErr   error
	updateErr   error
}

func (s *scriptedCAS) CreateWithVersion(ctx context.Context, lockID string, version storage.Version, info *storage.LockInformation) error {
	s.gate.Lock()
	if s.failCreates > 0 {
		s.failCreates--
		err := s.createErr
		s.gate.Unlock()
		return err
	}
	s.gate.Unlock()
	return s.mockSubscribeStorage.CreateWithVersion(ctx, lockID, version, info)
}

func (s *scriptedCAS) UpdateWithVersion(ctx context.Context, lockID string, oldVersion, newVersion storage.Version, info *storage.LockInformation) error {
	s.gate.Lock()
	if s.failUpdates > 0 {
		s.failUpdates--
		err := s.updateErr
		s.gate.Unlock()
		return err
	}
	s.gate.Unlock()
	return s.mockSubscribeStorage.UpdateWithVersion(ctx, lockID, oldVersion, newVersion, info)
}
