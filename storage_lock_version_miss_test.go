package storage_lock

import (
	"context"
	"errors"
	"testing"

	go_storage "github.com/storage-lock/go-storage"
)

// mockStorageLockVersionMiss 返回 VersionMiss 错误的存储
type mockStorageLockVersionMiss struct {
	*mockSubscribeStorage
	createCount int
}

func newMockStorageLockVersionMiss() *mockStorageLockVersionMiss {
	return &mockStorageLockVersionMiss{
		mockSubscribeStorage: newMockSubscribeStorage(),
	}
}

func (m *mockStorageLockVersionMiss) CreateWithVersion(ctx context.Context, lockId string, version go_storage.Version, lockInformation *go_storage.LockInformation) error {
	m.createCount++
	if m.createCount == 1 {
		return ErrVersionMiss
	}
	return nil
}

// TestStorageLockVersionMiss 测试 VersionMiss 分支
func TestStorageLockVersionMiss(t *testing.T) {
	ctx := context.Background()
	storage := newMockStorageLockVersionMiss()
	lock, _ := NewStorageLock(storage, "version-miss-test")

	err := lock.TryLock(ctx, "owner-1")
	if err != nil {
		t.Logf("VersionMiss returned: %v", err)
	}
}

// mockStorageLockErrVersionMiss 返回 ErrVersionMiss 的存储
type mockStorageLockErrVersionMiss struct {
	*mockSubscribeStorage
	updateCount int
}

func newMockStorageLockErrVersionMiss() *mockStorageLockErrVersionMiss {
	return &mockStorageLockErrVersionMiss{
		mockSubscribeStorage: newMockSubscribeStorage(),
	}
}

func (m *mockStorageLockErrVersionMiss) CreateWithVersion(ctx context.Context, lockId string, version go_storage.Version, lockInformation *go_storage.LockInformation) error {
	return errors.New("create failed")
}

func (m *mockStorageLockErrVersionMiss) Get(ctx context.Context, lockId string) (string, error) {
	return `{"LockId":"test","OwnerId":"owner-1","Version":1,"LockCount":1}`, nil
}

func (m *mockStorageLockErrVersionMiss) UpdateWithVersion(ctx context.Context, lockId string, oldVersion, newVersion go_storage.Version, lockInformation *go_storage.LockInformation) error {
	m.updateCount++
	return ErrVersionMiss
}

// TestStorageLockErrVersionMiss 测试 ErrVersionMiss 返回分支
func TestStorageLockErrVersionMiss(t *testing.T) {
	ctx := context.Background()
	storage := newMockStorageLockErrVersionMiss()
	lock, _ := NewStorageLock(storage, "err-version-miss-test")

	err := lock.TryLock(ctx, "owner-1")
	if err != nil {
		t.Logf("ErrVersionMiss returned: %v", err)
	}
}
