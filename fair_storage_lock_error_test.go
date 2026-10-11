package storage_lock

import (
	"context"
	"errors"
	"testing"

	go_storage "github.com/storage-lock/go-storage"
)

// mockFairStorageLockError 返回存储错误的 FairStorageLock 存储
type mockFairStorageLockError struct {
	*mockSubscribeStorage
}

func newMockFairStorageLockError() *mockFairStorageLockError {
	return &mockFairStorageLockError{
		mockSubscribeStorage: newMockSubscribeStorage(),
	}
}

func (m *mockFairStorageLockError) CreateWithVersion(ctx context.Context, lockId string, version go_storage.Version, lockInformation *go_storage.LockInformation) error {
	return errors.New("fairstoragelock error")
}

// TestFairStorageLockError 测试 FairStorageLock 存储错误分支
func TestFairStorageLockError(t *testing.T) {
	storage := newMockFairStorageLockError()

	_, err := NewFairStorageLock(storage, "fairstoragelock-error")
	if err != nil {
		t.Logf("NewFairStorageLock with storage error returned: %v", err)
	}
}

// TestFairStorageLockWithOptionsError 测试 FairStorageLockWithOptions 存储错误分支
func TestFairStorageLockWithOptionsError(t *testing.T) {
	storage := newMockFairStorageLockError()
	options := NewStorageLockOptionsWithLockId("fairstoragelock-options-error")

	_, err := NewFairStorageLockWithOptions(storage, options)
	if err != nil {
		t.Logf("NewFairStorageLockWithOptions with storage error returned: %v", err)
	}
}
