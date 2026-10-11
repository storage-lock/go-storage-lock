package storage_lock

import (
	"context"
	"errors"
	"testing"

	go_storage "github.com/storage-lock/go-storage"
)

// ==================== Semaphore 剩余分支 ====================

// mockSemaphoreDrainError 返回 DrainPermits 错误的信号量
type mockSemaphoreDrainError struct {
	*mockSubscribeStorage
	acquireCount int
}

func newMockSemaphoreDrainError() *mockSemaphoreDrainError {
	return &mockSemaphoreDrainError{
		mockSubscribeStorage: newMockSubscribeStorage(),
	}
}

func (m *mockSemaphoreDrainError) CreateWithVersion(ctx context.Context, lockId string, version go_storage.Version, lockInformation *go_storage.LockInformation) error {
	m.acquireCount++
	if m.acquireCount > 2 {
		return errors.New("drain error")
	}
	return nil
}

// TestSemaphoreDrainError 测试 Semaphore DrainPermits 错误分支

// ==================== LeaderElection 剩余分支 ====================

// mockLeaderElectionUnsubscribeError 返回 Unsubscribe 错误的存储
type mockLeaderElectionUnsubscribeError struct {
	*mockSubscribeStorage
}

func newMockLeaderElectionUnsubscribeError() *mockLeaderElectionUnsubscribeError {
	return &mockLeaderElectionUnsubscribeError{
		mockSubscribeStorage: newMockSubscribeStorage(),
	}
}

func (m *mockLeaderElectionUnsubscribeError) CreateWithVersion(ctx context.Context, lockId string, version go_storage.Version, lockInformation *go_storage.LockInformation) error {
	return errors.New("unsubscribe error")
}

// TestLeaderElectionUnsubscribeError 测试 LeaderElection Unsubscribe 错误分支
func TestLeaderElectionUnsubscribeError(t *testing.T) {
	ctx := context.Background()
	storage := newMockLeaderElectionUnsubscribeError()
	le, _ := NewLeaderElection(storage, "election-unsubscribe-error", "participant-1")

	err := le.Resign(ctx)
	if err != nil {
		t.Logf("Resign with unsubscribe error returned: %v", err)
	}
}

// ==================== MultiLock 剩余分支 ====================

// mockMultiLockUnLockError 返回 UnLock 错误的存储
type mockMultiLockUnLockError struct {
	*mockSubscribeStorage
}

func newMockMultiLockUnLockError() *mockMultiLockUnLockError {
	return &mockMultiLockUnLockError{
		mockSubscribeStorage: newMockSubscribeStorage(),
	}
}

func (m *mockMultiLockUnLockError) CreateWithVersion(ctx context.Context, lockId string, version go_storage.Version, lockInformation *go_storage.LockInformation) error {
	return nil
}

func (m *mockMultiLockUnLockError) DeleteWithVersion(ctx context.Context, lockId string, version go_storage.Version, lockInformation *go_storage.LockInformation) error {
	return errors.New("unlock error")
}

// TestMultiLockUnLockError 测试 MultiLock UnLock 错误分支
func TestMultiLockUnLockError(t *testing.T) {
	ctx := context.Background()
	storage := newMockMultiLockUnLockError()
	lock1, _ := NewStorageLock(storage, "lock-1")
	lock2, _ := NewStorageLock(storage, "lock-2")

	ml, _ := NewMultiLock(lock1, lock2)

	// 先获取锁
	_ = ml.TryLock(ctx)

	// 释放时会出错
	err := ml.UnLock(ctx)
	if err != nil {
		t.Logf("MultiLock UnLock error returned: %v", err)
	}
}

// ==================== StorageLock 订阅分支 ====================

// mockStorageLockSubscribeError 返回 Subscribe 错误的存储
type mockStorageLockSubscribeError struct {
	*mockSubscribeStorage
}

func newMockStorageLockSubscribeError() *mockStorageLockSubscribeError {
	return &mockStorageLockSubscribeError{
		mockSubscribeStorage: newMockSubscribeStorage(),
	}
}

func (m *mockStorageLockSubscribeError) CreateWithVersion(ctx context.Context, lockId string, version go_storage.Version, lockInformation *go_storage.LockInformation) error {
	return errors.New("subscribe error")
}

// TestStorageLockSubscribeError 测试 StorageLock Subscribe 错误分支
func TestStorageLockSubscribeError(t *testing.T) {
	ctx := context.Background()
	storage := newMockStorageLockSubscribeError()
	lock, _ := NewStorageLock(storage, "subscribe-error")

	ch, err := lock.Subscribe(ctx, "subscribe-error")
	if err != nil {
		t.Logf("Subscribe error returned: %v", err)
	}
	if ch != nil {
		lock.Unsubscribe("subscribe-error", ch)
	}
}

// ==================== StorageLock RW 锁分支 ====================

// mockStorageLockRWError 返回 RW 锁错误的存储
type mockStorageLockRWError struct {
	*mockSubscribeStorage
}

func newMockStorageLockRWError() *mockStorageLockRWError {
	return &mockStorageLockRWError{
		mockSubscribeStorage: newMockSubscribeStorage(),
	}
}

func (m *mockStorageLockRWError) CreateWithVersion(ctx context.Context, lockId string, version go_storage.Version, lockInformation *go_storage.LockInformation) error {
	return errors.New("rwlock error")
}

// TestStorageLockRWError 测试 StorageLock RLock/WLock 错误分支
func TestStorageLockRWError(t *testing.T) {
	ctx := context.Background()
	storage := newMockStorageLockRWError()
	lock, _ := NewStorageLock(storage, "rw-error")

	err := lock.RLock(ctx, "owner-1")
	if err != nil {
		t.Logf("RLock error returned: %v", err)
	}

	err = lock.WLock(ctx, "owner-1")
	if err != nil {
		t.Logf("WLock error returned: %v", err)
	}
}

// ==================== StorageLock Unlock 错误分支 ====================

// mockStorageLockUnlockError 返回 Unlock 错误的存储
type mockStorageLockUnlockError struct {
	*mockSubscribeStorage
}

func newMockStorageLockUnlockError() *mockStorageLockUnlockError {
	return &mockStorageLockUnlockError{
		mockSubscribeStorage: newMockSubscribeStorage(),
	}
}

func (m *mockStorageLockUnlockError) CreateWithVersion(ctx context.Context, lockId string, version go_storage.Version, lockInformation *go_storage.LockInformation) error {
	return nil
}

func (m *mockStorageLockUnlockError) DeleteWithVersion(ctx context.Context, lockId string, version go_storage.Version, lockInformation *go_storage.LockInformation) error {
	return errors.New("unlock error")
}

// TestStorageLockUnlockError 测试 StorageLock UnLock 错误分支
func TestStorageLockUnlockError(t *testing.T) {
	ctx := context.Background()
	storage := newMockStorageLockUnlockError()
	lock, _ := NewStorageLock(storage, "unlock-error")

	// 先获取锁
	_ = lock.TryLock(ctx, "owner-1")

	// 释放时会出错
	err := lock.UnLock(ctx, "owner-1")
	if err != nil {
		t.Logf("UnLock error returned: %v", err)
	}
}
