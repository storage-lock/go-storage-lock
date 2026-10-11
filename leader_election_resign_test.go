package storage_lock

import (
	"context"
	"errors"
	"testing"

	go_storage "github.com/storage-lock/go-storage"
)

// mockStorageForLeaderElectionResignError 专门用于触发 LeaderElection.Resign 错误
type mockStorageForLeaderElectionResignError struct {
	*mockSubscribeStorage
}

func newMockStorageForLeaderElectionResignError() *mockStorageForLeaderElectionResignError {
	return &mockStorageForLeaderElectionResignError{
		mockSubscribeStorage: newMockSubscribeStorage(),
	}
}

func (m *mockStorageForLeaderElectionResignError) CreateWithVersion(ctx context.Context, lockId string, version go_storage.Version, lockInformation *go_storage.LockInformation) error {
	return nil
}

func (m *mockStorageForLeaderElectionResignError) DeleteWithVersion(ctx context.Context, lockId string, version go_storage.Version, lockInformation *go_storage.LockInformation) error {
	return errors.New("election resign error")
}

// TestLeaderElectionResignStorageError 测试 LeaderElection.Resign 存储错误分支
func TestLeaderElectionResignStorageError(t *testing.T) {
	ctx := context.Background()
	storage := newMockStorageForLeaderElectionResignError()
	le, _ := NewLeaderElection(storage, "election-resign-storage-error", "p1")

	// 先成为 leader
	_ = le.Campaign(ctx)

	// 让位时出错
	err := le.Resign(ctx)
	if err != nil {
		t.Logf("LeaderElection Resign storage error: %v", err)
	}
}
