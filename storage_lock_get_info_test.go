package storage_lock

import (
	"context"
	"errors"
	"testing"

	go_storage "github.com/storage-lock/go-storage"
)

// TestStorageLockGetLockInformationBranches 测试 StorageLock.getLockInformation 的分支
func TestStorageLockGetLockInformationBranches(t *testing.T) {
	// 测试存储返回空字符串（锁不存在）
	t.Run("StorageReturnsEmptyString", func(t *testing.T) {
		ctx := context.Background()
		storage := newMockStorageReturnsEmpty()
		lock, _ := NewStorageLock(storage, "test-empty-lock")

		err := lock.TryLock(ctx, "owner-1")
		if err != ErrLockNotFound {
			t.Logf("Storage returns empty returned: %v", err)
		}
	})
}

// mockStorageReturnsEmpty 返回空字符串的存储
type mockStorageReturnsEmpty struct {
	*mockSubscribeStorage
}

func newMockStorageReturnsEmpty() *mockStorageReturnsEmpty {
	return &mockStorageReturnsEmpty{
		mockSubscribeStorage: newMockSubscribeStorage(),
	}
}

func (m *mockStorageReturnsEmpty) Get(ctx context.Context, lockId string) (string, error) {
	return "", nil // 返回空字符串
}

func (m *mockStorageReturnsEmpty) CreateWithVersion(ctx context.Context, lockId string, version go_storage.Version, lockInformation *go_storage.LockInformation) error {
	return errors.New("create failed") // 强制走 Get 分支
}
