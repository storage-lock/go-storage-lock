package storage_lock

import (
	"context"
	"errors"
	"testing"

	go_storage "github.com/storage-lock/go-storage"
)

// mockStorageLockInfoError 返回 LockInformation 错误的存储
type mockStorageLockInfoError struct {
	*mockSubscribeStorage
}

func newMockStorageLockInfoError() *mockStorageLockInfoError {
	return &mockStorageLockInfoError{
		mockSubscribeStorage: newMockSubscribeStorage(),
	}
}

func (m *mockStorageLockInfoError) Get(ctx context.Context, lockId string) (string, error) {
	return "invalid json {{{", nil // 返回无效 JSON
}

func (m *mockStorageLockInfoError) CreateWithVersion(ctx context.Context, lockId string, version go_storage.Version, lockInformation *go_storage.LockInformation) error {
	return errors.New("create failed") // 强制走 Get 分支
}

// TestStorageLockLockInfoError 测试 LockInformation 解析错误分支
func TestStorageLockLockInfoError(t *testing.T) {
	ctx := context.Background()
	storage := newMockStorageLockInfoError()
	lock, _ := NewStorageLock(storage, "test-lock-info-error")

	err := lock.TryLock(ctx, "owner-1")
	if err != nil {
		t.Logf("LockInfo error returned: %v", err)
	}
}
