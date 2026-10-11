package storage_lock

import (
	"context"
	"errors"
	"testing"

	go_storage "github.com/storage-lock/go-storage"
)

// TestStorageLockSkipCapabilityCheck 测试 SkipCapabilityCheck 分支
func TestStorageLockSkipCapabilityCheck(t *testing.T) {
	ctx := context.Background()

	// 测试 SkipCapabilityCheck = true
	t.Run("SkipCapabilityCheck_True", func(t *testing.T) {
		storage := newMockSubscribeStorage()
		options := NewStorageLockOptionsWithLockId("test-skip-cap")
		options.SkipCapabilityCheck = true

		lock, err := NewStorageLockWithOptions(storage, options)
		if err != nil {
			t.Fatalf("NewStorageLockWithOptions error: %v", err)
		}

		err = lock.TryLock(ctx, "owner-1")
		if err != nil {
			t.Logf("TryLock returned: %v", err)
		}
		_ = lock.UnLock(ctx, "owner-1")
	})

	// 测试 SkipCapabilityCheck = false（默认）
	t.Run("SkipCapabilityCheck_False", func(t *testing.T) {
		storage := newMockSubscribeStorage()
		options := NewStorageLockOptionsWithLockId("test-no-skip-cap")
		options.SkipCapabilityCheck = false

		lock, err := NewStorageLockWithOptions(storage, options)
		if err != nil {
			t.Fatalf("NewStorageLockWithOptions error: %v", err)
		}

		err = lock.TryLock(ctx, "owner-1")
		if err != nil {
			t.Logf("TryLock returned: %v", err)
		}
		_ = lock.UnLock(ctx, "owner-1")
	})
}

// mockStorageGetError 返回 Get 错误的存储
type mockStorageGetError struct {
	*mockSubscribeStorage
}

func newMockStorageGetError() *mockStorageGetError {
	return &mockStorageGetError{
		mockSubscribeStorage: newMockSubscribeStorage(),
	}
}

func (m *mockStorageGetError) Get(ctx context.Context, lockId string) (string, error) {
	return "", errors.New("storage error") // 返回存储错误
}

func (m *mockStorageGetError) CreateWithVersion(ctx context.Context, lockId string, version go_storage.Version, lockInformation *go_storage.LockInformation) error {
	return errors.New("storage error") // 强制走 Get 分支
}

// TestStorageLockGetError 测试 getLockInformation 的 Get 错误分支
func TestStorageLockGetError(t *testing.T) {
	ctx := context.Background()
	storage := newMockStorageGetError()
	lock, _ := NewStorageLock(storage, "test-get-error")

	err := lock.TryLock(ctx, "owner-1")
	if err != nil {
		t.Logf("Get error returned: %v", err)
	}
}
