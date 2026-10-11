package storage_lock

import (
	"context"
	"errors"
	"testing"

	go_storage "github.com/storage-lock/go-storage"
)

// mockStorageForSemaphoreReduceError 专门用于触发 Semaphore.ReducePermits 错误
type mockStorageForSemaphoreReduceError struct {
	*mockSubscribeStorage
}

func newMockStorageForSemaphoreReduceError() *mockStorageForSemaphoreReduceError {
	return &mockStorageForSemaphoreReduceError{
		mockSubscribeStorage: newMockSubscribeStorage(),
	}
}

func (m *mockStorageForSemaphoreReduceError) CreateWithVersion(ctx context.Context, lockId string, version go_storage.Version, lockInformation *go_storage.LockInformation) error {
	return errors.New("semaphore reduce error")
}

// TestSemaphoreReducePermitsError 测试 Semaphore.ReducePermits 错误分支

// mockStorageForLeaderElectionCloseError 专门用于触发 LeaderElection.Close 错误
type mockStorageForLeaderElectionCloseError struct {
	*mockSubscribeStorage
}

func newMockStorageForLeaderElectionCloseError() *mockStorageForLeaderElectionCloseError {
	return &mockStorageForLeaderElectionCloseError{
		mockSubscribeStorage: newMockSubscribeStorage(),
	}
}

func (m *mockStorageForLeaderElectionCloseError) CreateWithVersion(ctx context.Context, lockId string, version go_storage.Version, lockInformation *go_storage.LockInformation) error {
	return errors.New("election close error")
}

// TestLeaderElectionCloseError 测试 LeaderElection.Close 错误分支
func TestLeaderElectionCloseError(t *testing.T) {
	storage := newMockStorageForLeaderElectionCloseError()
	le, _ := NewLeaderElection(storage, "election-close-error", "p1")

	err := le.Close()
	if err != nil {
		t.Logf("Close error: %v", err)
	}
}

// mockStorageForMultiLockRollbackError 专门用于触发 MultiLock.rollback 错误
type mockStorageForMultiLockRollbackError struct {
	*mockSubscribeStorage
}

func newMockStorageForMultiLockRollbackError() *mockStorageForMultiLockRollbackError {
	return &mockStorageForMultiLockRollbackError{
		mockSubscribeStorage: newMockSubscribeStorage(),
	}
}

func (m *mockStorageForMultiLockRollbackError) CreateWithVersion(ctx context.Context, lockId string, version go_storage.Version, lockInformation *go_storage.LockInformation) error {
	return errors.New("multilock rollback error")
}

// TestMultiLockRollbackError 测试 MultiLock.rollback 错误分支
func TestMultiLockRollbackError(t *testing.T) {
	ctx := context.Background()
	storage := newMockStorageForMultiLockRollbackError()
	lock1, _ := NewStorageLock(storage, "lock1")
	lock2, _ := NewStorageLock(storage, "lock2")

	ml, _ := NewMultiLock(lock1, lock2)

	// 尝试获取锁会失败，触发 rollback
	err := ml.Lock(ctx)
	if err != nil {
		t.Logf("MultiLock Lock with rollback error: %v", err)
	}
}
