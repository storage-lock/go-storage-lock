package storage_lock

import (
	"context"
	"errors"
	"testing"

	go_storage "github.com/storage-lock/go-storage"
)

// mockStorageLockRWUnLockError 返回 RUnLock 错误的存储
type mockStorageLockRWUnLockError struct {
	*mockSubscribeStorage
}

func newMockStorageLockRWUnLockError() *mockStorageLockRWUnLockError {
	return &mockStorageLockRWUnLockError{
		mockSubscribeStorage: newMockSubscribeStorage(),
	}
}

func (m *mockStorageLockRWUnLockError) CreateWithVersion(ctx context.Context, lockId string, version go_storage.Version, lockInformation *go_storage.LockInformation) error {
	return nil
}

func (m *mockStorageLockRWUnLockError) DeleteWithVersion(ctx context.Context, lockId string, version go_storage.Version, lockInformation *go_storage.LockInformation) error {
	return errors.New("runlock error")
}

// TestStorageLockRWUnLockError 测试 StorageLock RUnLock/WUnLock 错误分支
func TestStorageLockRWUnLockError(t *testing.T) {
	ctx := context.Background()
	storage := newMockStorageLockRWUnLockError()
	lock, _ := NewStorageLock(storage, "rw-unlock-error")

	// 先获取读锁
	_ = lock.RLock(ctx, "owner-1")

	// 释放读锁时会出错
	err := lock.RUnLock(ctx, "owner-1")
	if err != nil {
		t.Logf("RUnLock error returned: %v", err)
	}

	// 先获取写锁
	_ = lock.WLock(ctx, "owner-2")

	// 释放写锁时会出错
	err = lock.WUnLock(ctx, "owner-2")
	if err != nil {
		t.Logf("WUnLock error returned: %v", err)
	}
}

// mockStorageLockUnsubscribeNilChannel 返回 nil channel 的 Unsubscribe
type mockStorageLockUnsubscribeNilChannel struct {
	*mockSubscribeStorage
}

func newMockStorageLockUnsubscribeNilChannel() *mockStorageLockUnsubscribeNilChannel {
	return &mockStorageLockUnsubscribeNilChannel{
		mockSubscribeStorage: newMockSubscribeStorage(),
	}
}

func (m *mockStorageLockUnsubscribeNilChannel) CreateWithVersion(ctx context.Context, lockId string, version go_storage.Version, lockInformation *go_storage.LockInformation) error {
	return errors.New("unsubscribe nil channel error")
}

// TestStorageLockUnsubscribeNilChannel 测试 StorageLock Unsubscribe nil channel 分支
func TestStorageLockUnsubscribeNilChannel(t *testing.T) {
	storage := newMockStorageLockUnsubscribeNilChannel()
	lock, _ := NewStorageLock(storage, "unsubscribe-nil-channel")

	// 传入 nil channel
	lock.Unsubscribe("test-lock", nil)
}

// mockStorageLockOptionsError 返回选项错误的存储
type mockStorageLockOptionsError struct {
	*mockSubscribeStorage
}

func newMockStorageLockOptionsError() *mockStorageLockOptionsError {
	return &mockStorageLockOptionsError{
		mockSubscribeStorage: newMockSubscribeStorage(),
	}
}

func (m *mockStorageLockOptionsError) CreateWithVersion(ctx context.Context, lockId string, version go_storage.Version, lockInformation *go_storage.LockInformation) error {
	return errors.New("options error")
}

// TestStorageLockOptionsError 测试 StorageLock 选项错误分支
func TestStorageLockOptionsError(t *testing.T) {
	storage := newMockStorageLockOptionsError()
	options := NewStorageLockOptionsWithLockId("options-error")
	options.LeaseExpireAfter = -1 // 无效选项

	_, err := NewStorageLockWithOptions(storage, options)
	if err != nil {
		t.Logf("NewStorageLockWithOptions with invalid options returned: %v", err)
	}
}
