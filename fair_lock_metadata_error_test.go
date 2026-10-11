package storage_lock

import (
	"context"
	"errors"
	"testing"

	go_storage "github.com/storage-lock/go-storage"
)

// mockFairLockError 返回存储错误的 FairLock 存储
type mockFairLockError struct {
	*mockSubscribeStorage
}

func newMockFairLockError() *mockFairLockError {
	return &mockFairLockError{
		mockSubscribeStorage: newMockSubscribeStorage(),
	}
}

func (m *mockFairLockError) CreateWithVersion(ctx context.Context, lockId string, version go_storage.Version, lockInformation *go_storage.LockInformation) error {
	return errors.New("fairlock error")
}

// TestFairLockError 测试 FairLock 存储错误分支
func TestFairLockError(t *testing.T) {
	storage := newMockFairLockError()

	_, err := NewFairLock(storage, "fairlock-error")
	if err != nil {
		t.Logf("NewFairLock with storage error returned: %v", err)
	}
}

