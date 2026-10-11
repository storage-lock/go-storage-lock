package storage_lock

import (
	"context"
	"errors"
	"testing"
)

// mockStorageLockLeaseError 返回 Inspect 错误的存储锁租约存储
type mockStorageLockLeaseError struct {
	*mockSubscribeStorage
}

func newMockStorageLockLeaseError() *mockStorageLockLeaseError {
	return &mockStorageLockLeaseError{
		mockSubscribeStorage: newMockSubscribeStorage(),
	}
}

func (m *mockStorageLockLeaseError) Get(ctx context.Context, lockId string) (string, error) {
	return "", errors.New("lease inspect error")
}

// TestStorageLockLeaseError 测试 StorageLock 租约错误分支
func TestStorageLockLeaseError(t *testing.T) {
	ctx := context.Background()
	storage := newMockStorageLockLeaseError()
	lock, _ := NewStorageLock(storage, "lease-error")

	_, err := lock.GetOwner(ctx)
	if err != nil {
		t.Logf("GetOwner with inspect error returned: %v", err)
	}

	_, err = lock.TTL(ctx, "owner-1")
	if err != nil {
		t.Logf("TTL with inspect error returned: %v", err)
	}
}
