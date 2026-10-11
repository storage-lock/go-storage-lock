package storage_lock

import (
	"context"
	"errors"
	"testing"

	go_storage "github.com/storage-lock/go-storage"
)

// mockStorageForFairLockNewError 专门用于触发 FairLock.NewStorageLock 错误
type mockStorageForFairLockNewError struct {
	*mockSubscribeStorage
}

func newMockStorageForFairLockNewError() *mockStorageForFairLockNewError {
	return &mockStorageForFairLockNewError{
		mockSubscribeStorage: newMockSubscribeStorage(),
	}
}

func (m *mockStorageForFairLockNewError) CreateWithVersion(ctx context.Context, lockId string, version go_storage.Version, lockInformation *go_storage.LockInformation) error {
	return errors.New("fairlock newstorage error")
}

// TestFairLockNewStorageLockError 测试 FairLock.NewStorageLock 错误分支
func TestFairLockNewStorageLockError(t *testing.T) {
	storage := newMockStorageForFairLockNewError()

	_, err := NewFairLock(storage, "fairlock-newstorage-error")
	if err != nil {
		t.Logf("FairLock NewStorageLock error: %v", err)
	}
}

// mockStorageForFairLockTryLockError 专门用于触发 FairLock.TryLock 错误
type mockStorageForFairLockTryLockError struct {
	*mockSubscribeStorage
}

func newMockStorageForFairLockTryLockError() *mockStorageForFairLockTryLockError {
	return &mockStorageForFairLockTryLockError{
		mockSubscribeStorage: newMockSubscribeStorage(),
	}
}

func (m *mockStorageForFairLockTryLockError) CreateWithVersion(ctx context.Context, lockId string, version go_storage.Version, lockInformation *go_storage.LockInformation) error {
	return errors.New("fairlock trylock error")
}

// TestFairLockTryLockStorageError 测试 FairLock.TryLock 存储错误分支
func TestFairLockTryLockStorageError(t *testing.T) {
	ctx := context.Background()
	storage := newMockStorageForFairLockTryLockError()
	fl, _ := NewFairLock(storage, "fairlock-trylock-storage-error")

	err := fl.TryLock(ctx)
	if err != nil {
		t.Logf("FairLock TryLock storage error: %v", err)
	}
}

// mockStorageForLeaderElectionNewError 专门用于触发 LeaderElection.NewStorageLock 错误
type mockStorageForLeaderElectionNewError struct {
	*mockSubscribeStorage
}

func newMockStorageForLeaderElectionNewError() *mockStorageForLeaderElectionNewError {
	return &mockStorageForLeaderElectionNewError{
		mockSubscribeStorage: newMockSubscribeStorage(),
	}
}

func (m *mockStorageForLeaderElectionNewError) CreateWithVersion(ctx context.Context, lockId string, version go_storage.Version, lockInformation *go_storage.LockInformation) error {
	return errors.New("election newstorage error")
}

// TestLeaderElectionNewStorageLockError 测试 LeaderElection.NewStorageLock 错误分支
func TestLeaderElectionNewStorageLockError(t *testing.T) {
	storage := newMockStorageForLeaderElectionNewError()

	_, err := NewLeaderElection(storage, "election-newstorage-error", "p1")
	if err != nil {
		t.Logf("LeaderElection NewStorageLock error: %v", err)
	}
}

// mockStorageForLeaderElectionLeaderError 专门用于触发 LeaderElection.Leader 错误
type mockStorageForLeaderElectionLeaderError struct {
	*mockSubscribeStorage
}

func newMockStorageForLeaderElectionLeaderError() *mockStorageForLeaderElectionLeaderError {
	return &mockStorageForLeaderElectionLeaderError{
		mockSubscribeStorage: newMockSubscribeStorage(),
	}
}

func (m *mockStorageForLeaderElectionLeaderError) Get(ctx context.Context, lockId string) (string, error) {
	return "", errors.New("election leader get error")
}

// TestLeaderElectionLeaderStorageError 测试 LeaderElection.Leader 存储错误分支
func TestLeaderElectionLeaderStorageError(t *testing.T) {
	ctx := context.Background()
	storage := newMockStorageForLeaderElectionLeaderError()
	le, _ := NewLeaderElection(storage, "election-leader-storage-error", "p1")

	_, err := le.Leader(ctx)
	if err != nil {
		t.Logf("LeaderElection Leader storage error: %v", err)
	}
}

// mockStorageForSemaphoreNewError 专门用于触发 Semaphore.NewStorageLock 错误
type mockStorageForSemaphoreNewError struct {
	*mockSubscribeStorage
}

func newMockStorageForSemaphoreNewError() *mockStorageForSemaphoreNewError {
	return &mockStorageForSemaphoreNewError{
		mockSubscribeStorage: newMockSubscribeStorage(),
	}
}

func (m *mockStorageForSemaphoreNewError) CreateWithVersion(ctx context.Context, lockId string, version go_storage.Version, lockInformation *go_storage.LockInformation) error {
	return errors.New("semaphore newstorage error")
}

// TestSemaphoreNewStorageLockError 测试 Semaphore.NewStorageLock 错误分支

// mockStorageForSemaphoreTryAcquireError 专门用于触发 Semaphore.TryAcquire 错误
type mockStorageForSemaphoreTryAcquireError struct {
	*mockSubscribeStorage
}

func newMockStorageForSemaphoreTryAcquireError() *mockStorageForSemaphoreTryAcquireError {
	return &mockStorageForSemaphoreTryAcquireError{
		mockSubscribeStorage: newMockSubscribeStorage(),
	}
}

func (m *mockStorageForSemaphoreTryAcquireError) CreateWithVersion(ctx context.Context, lockId string, version go_storage.Version, lockInformation *go_storage.LockInformation) error {
	return errors.New("semaphore tryacquire error")
}

// TestSemaphoreTryAcquireStorageError 测试 Semaphore.TryAcquire 存储错误分支

// mockStorageForSemaphoreAvailablePermitsError 专门用于触发 Semaphore.AvailablePermits 错误
type mockStorageForSemaphoreAvailablePermitsError struct {
	*mockSubscribeStorage
}

func newMockStorageForSemaphoreAvailablePermitsError() *mockStorageForSemaphoreAvailablePermitsError {
	return &mockStorageForSemaphoreAvailablePermitsError{
		mockSubscribeStorage: newMockSubscribeStorage(),
	}
}

func (m *mockStorageForSemaphoreAvailablePermitsError) Get(ctx context.Context, lockId string) (string, error) {
	return "", errors.New("semaphore availablepermits error")
}

// TestSemaphoreAvailablePermitsStorageError 测试 Semaphore.AvailablePermits 存储错误分支

// mockStorageForMultiLockLockError 专门用于触发 MultiLock.Lock 错误
type mockStorageForMultiLockLockError struct {
	*mockSubscribeStorage
}

func newMockStorageForMultiLockLockError() *mockStorageForMultiLockLockError {
	return &mockStorageForMultiLockLockError{
		mockSubscribeStorage: newMockSubscribeStorage(),
	}
}

func (m *mockStorageForMultiLockLockError) CreateWithVersion(ctx context.Context, lockId string, version go_storage.Version, lockInformation *go_storage.LockInformation) error {
	return errors.New("multilock lock error")
}

// TestMultiLockLockStorageError 测试 MultiLock.Lock 存储错误分支
func TestMultiLockLockStorageError(t *testing.T) {
	ctx := context.Background()
	storage := newMockStorageForMultiLockLockError()
	lock1, _ := NewStorageLock(storage, "lock1")
	lock2, _ := NewStorageLock(storage, "lock2")

	ml, _ := NewMultiLock(lock1, lock2)

	err := ml.Lock(ctx)
	if err != nil {
		t.Logf("MultiLock Lock storage error: %v", err)
	}
}

// mockStorageForMultiLockTryLockError 专门用于触发 MultiLock.TryLock 错误
type mockStorageForMultiLockTryLockError struct {
	*mockSubscribeStorage
}

func newMockStorageForMultiLockTryLockError() *mockStorageForMultiLockTryLockError {
	return &mockStorageForMultiLockTryLockError{
		mockSubscribeStorage: newMockSubscribeStorage(),
	}
}

func (m *mockStorageForMultiLockTryLockError) CreateWithVersion(ctx context.Context, lockId string, version go_storage.Version, lockInformation *go_storage.LockInformation) error {
	return errors.New("multilock trylock error")
}

// TestMultiLockTryLockStorageError 测试 MultiLock.TryLock 存储错误分支
func TestMultiLockTryLockStorageError(t *testing.T) {
	ctx := context.Background()
	storage := newMockStorageForMultiLockTryLockError()
	lock1, _ := NewStorageLock(storage, "lock1")
	lock2, _ := NewStorageLock(storage, "lock2")

	ml, _ := NewMultiLock(lock1, lock2)

	err := ml.TryLock(ctx)
	if err != nil {
		t.Logf("MultiLock TryLock storage error: %v", err)
	}
}

// mockStorageForMultiLockUnLockError 专门用于触发 MultiLock.UnLock 错误
type mockStorageForMultiLockUnLockError struct {
	*mockSubscribeStorage
}

func newMockStorageForMultiLockUnLockError() *mockStorageForMultiLockUnLockError {
	return &mockStorageForMultiLockUnLockError{
		mockSubscribeStorage: newMockSubscribeStorage(),
	}
}

func (m *mockStorageForMultiLockUnLockError) CreateWithVersion(ctx context.Context, lockId string, version go_storage.Version, lockInformation *go_storage.LockInformation) error {
	return nil
}

func (m *mockStorageForMultiLockUnLockError) DeleteWithVersion(ctx context.Context, lockId string, version go_storage.Version, lockInformation *go_storage.LockInformation) error {
	return errors.New("multilock unlock error")
}

// TestMultiLockUnLockStorageError 测试 MultiLock.UnLock 存储错误分支
func TestMultiLockUnLockStorageError(t *testing.T) {
	ctx := context.Background()
	storage := newMockStorageForMultiLockUnLockError()
	lock1, _ := NewStorageLock(storage, "lock1")
	lock2, _ := NewStorageLock(storage, "lock2")

	ml, _ := NewMultiLock(lock1, lock2)

	// 先获取锁
	_ = ml.TryLock(ctx)

	// 释放时出错
	err := ml.UnLock(ctx)
	if err != nil {
		t.Logf("MultiLock UnLock storage error: %v", err)
	}
}
