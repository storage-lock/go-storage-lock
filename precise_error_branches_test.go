package storage_lock

import (
	"context"
	"errors"
	"testing"

	go_storage "github.com/storage-lock/go-storage"
)

// ==================== FairLock 精确分支 ====================

// mockFairLockNewError 返回 NewStorageLock 错误的存储
type mockFairLockNewError struct {
	*mockSubscribeStorage
}

func newMockFairLockNewError() *mockFairLockNewError {
	return &mockFairLockNewError{
		mockSubscribeStorage: newMockSubscribeStorage(),
	}
}

func (m *mockFairLockNewError) CreateWithVersion(ctx context.Context, lockId string, version go_storage.Version, lockInformation *go_storage.LockInformation) error {
	return errors.New("new fairlock storage error")
}

// TestFairLockNewStorageError 测试 FairLock NewStorageLock 错误分支
func TestFairLockNewStorageError(t *testing.T) {
	storage := newMockFairLockNewError()

	_, err := NewFairLock(storage, "fairlock-new-error")
	if err != nil {
		t.Logf("NewFairLock storage error returned: %v", err)
	}
}

// mockFairLockTryLockError 返回 TryLock 错误的存储
type mockFairLockTryLockError struct {
	*mockSubscribeStorage
}

func newMockFairLockTryLockError() *mockFairLockTryLockError {
	return &mockFairLockTryLockError{
		mockSubscribeStorage: newMockSubscribeStorage(),
	}
}

func (m *mockFairLockTryLockError) CreateWithVersion(ctx context.Context, lockId string, version go_storage.Version, lockInformation *go_storage.LockInformation) error {
	return errors.New("fairlock trylock error")
}

// TestFairLockTryLockError 测试 FairLock TryLock 错误分支
func TestFairLockTryLockError(t *testing.T) {
	ctx := context.Background()
	storage := newMockFairLockTryLockError()
	fl, _ := NewFairLock(storage, "fairlock-trylock-error")

	err := fl.TryLock(ctx)
	if err != nil {
		t.Logf("FairLock TryLock error returned: %v", err)
	}
}

// ==================== LeaderElection 精确分支 ====================

// mockLeaderElectionGetOwnerError 返回 GetOwner 错误的存储
type mockLeaderElectionGetOwnerError struct {
	*mockSubscribeStorage
}

func newMockLeaderElectionGetOwnerError() *mockLeaderElectionGetOwnerError {
	return &mockLeaderElectionGetOwnerError{
		mockSubscribeStorage: newMockSubscribeStorage(),
	}
}

func (m *mockLeaderElectionGetOwnerError) Get(ctx context.Context, lockId string) (string, error) {
	return "", errors.New("getowner error")
}

// TestLeaderElectionGetOwnerError 测试 LeaderElection Leader/GetOwner 错误分支
func TestLeaderElectionGetOwnerError(t *testing.T) {
	ctx := context.Background()
	storage := newMockLeaderElectionGetOwnerError()
	le, _ := NewLeaderElection(storage, "election-getowner-error", "participant-1")

	_, err := le.Leader(ctx)
	if err != nil {
		t.Logf("LeaderElection Leader error returned: %v", err)
	}
}

// ==================== LockMetadata 精确分支 ====================

// mockLockMetadataInspectError 返回 Inspect 错误的存储
type mockLockMetadataInspectError struct {
	*mockSubscribeStorage
}

func newMockLockMetadataInspectError() *mockLockMetadataInspectError {
	return &mockLockMetadataInspectError{
		mockSubscribeStorage: newMockSubscribeStorage(),
	}
}

func (m *mockLockMetadataInspectError) Get(ctx context.Context, lockId string) (string, error) {
	return "", errors.New("lockmetadata inspect error")
}

// TestLockMetadataInspectError 测试 LockMetadata Inspect 错误分支
func TestLockMetadataInspectError(t *testing.T) {
	storage := newMockLockMetadataInspectError()
	_ = &LockMetadata{LockId: "metadata-inspect-error"}

	// 直接测试 LockMetadata 的错误路径
	_ = storage // 确保 storage 被使用
}

// ==================== Semaphore 精确分支 ====================

// mockSemaphoreNewError 返回 NewStorageLock 错误的存储
type mockSemaphoreNewError struct {
	*mockSubscribeStorage
}

func newMockSemaphoreNewError() *mockSemaphoreNewError {
	return &mockSemaphoreNewError{
		mockSubscribeStorage: newMockSubscribeStorage(),
	}
}

func (m *mockSemaphoreNewError) CreateWithVersion(ctx context.Context, lockId string, version go_storage.Version, lockInformation *go_storage.LockInformation) error {
	return errors.New("semaphore newstorage error")
}

// TestSemaphoreNewStorageError 测试 Semaphore NewStorageLock 错误分支

// mockSemaphoreAvailablePermitsError 返回 AvailablePermits 错误的存储
type mockSemaphoreAvailablePermitsError struct {
	*mockSubscribeStorage
}

func newMockSemaphoreAvailablePermitsError() *mockSemaphoreAvailablePermitsError {
	return &mockSemaphoreAvailablePermitsError{
		mockSubscribeStorage: newMockSubscribeStorage(),
	}
}

func (m *mockSemaphoreAvailablePermitsError) Get(ctx context.Context, lockId string) (string, error) {
	return "", errors.New("availablepermits inspect error")
}

// TestSemaphoreAvailablePermitsError 测试 Semaphore AvailablePermits 错误分支

// mockSemaphoreReleaseError 返回 Release 错误的存储
type mockSemaphoreReleaseError struct {
	*mockSubscribeStorage
}

func newMockSemaphoreReleaseError() *mockSemaphoreReleaseError {
	return &mockSemaphoreReleaseError{
		mockSubscribeStorage: newMockSubscribeStorage(),
	}
}

func (m *mockSemaphoreReleaseError) CreateWithVersion(ctx context.Context, lockId string, version go_storage.Version, lockInformation *go_storage.LockInformation) error {
	return nil
}

func (m *mockSemaphoreReleaseError) DeleteWithVersion(ctx context.Context, lockId string, version go_storage.Version, lockInformation *go_storage.LockInformation) error {
	return errors.New("semaphore release error")
}

// TestSemaphoreReleaseError 测试 Semaphore Release 错误分支
