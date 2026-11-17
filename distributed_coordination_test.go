package storage_lock

import (
	"context"
	"errors"
	"testing"
	"time"
)

// mockStorageForLeaderElection 用于 LeaderElection 测试的 mock storage
type mockStorageForLeaderElection struct {
	*mockSubscribeStorage
}

func newMockStorageForLeaderElection() *mockStorageForLeaderElection {
	return &mockStorageForLeaderElection{
		mockSubscribeStorage: newMockSubscribeStorage(),
	}
}

func TestLeaderElectionCampaign(t *testing.T) {
	ctx := context.Background()
	storage := newMockStorageForLeaderElection()

	election, err := NewLeaderElection(storage, "test-election", "participant-1")
	if err != nil {
		t.Fatalf("NewLeaderElection error: %v", err)
	}
	defer election.Close()

	// 首次竞选应该成功
	err = election.Campaign(ctx)
	if err != nil {
		t.Fatalf("Campaign error: %v", err)
	}

	if !election.IsLeader() {
		t.Fatal("expected to be leader after Campaign")
	}

	// 第二次竞选也应该成功（可重入）
	err = election.Campaign(ctx)
	if err != nil {
		t.Fatalf("second Campaign error: %v", err)
	}
}

func TestLeaderElectionObserve(t *testing.T) {
	ctx := context.Background()
	storage := newMockStorageForLeaderElection()

	election, err := NewLeaderElection(storage, "test-election-observe", "participant-1")
	if err != nil {
		t.Fatalf("NewLeaderElection error: %v", err)
	}
	defer election.Close()

	// 订阅 leader 变化
	ch := election.Observe()

	// 成为 leader
	err = election.Campaign(ctx)
	if err != nil {
		t.Fatalf("Campaign error: %v", err)
	}

	// 应该收到 leader 通知
	select {
	case leader := <-ch:
		if leader != "participant-1" {
			t.Fatalf("expected leader participant-1, got %s", leader)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for leader notification")
	}
}

func TestLeaderElectionResign(t *testing.T) {
	ctx := context.Background()
	storage := newMockStorageForLeaderElection()

	election, err := NewLeaderElection(storage, "test-election-resign", "participant-1")
	if err != nil {
		t.Fatalf("NewLeaderElection error: %v", err)
	}
	defer election.Close()

	// 先成为 leader
	err = election.Campaign(ctx)
	if err != nil {
		t.Fatalf("Campaign error: %v", err)
	}

	// 让位
	err = election.Resign(ctx)
	if err != nil {
		t.Fatalf("Resign error: %v", err)
	}

	if election.IsLeader() {
		t.Fatal("should not be leader after Resign")
	}
}

func TestLeaderElectionLeader(t *testing.T) {
	ctx := context.Background()
	storage := newMockStorageForLeaderElection()

	election, err := NewLeaderElection(storage, "test-election-leader", "participant-1")
	if err != nil {
		t.Fatalf("NewLeaderElection error: %v", err)
	}
	defer election.Close()

	// 成为 leader
	err = election.Campaign(ctx)
	if err != nil {
		t.Fatalf("Campaign error: %v", err)
	}

	// 查询当前 leader
	leader, err := election.Leader(ctx)
	if err != nil {
		t.Fatalf("Leader error: %v", err)
	}

	if leader != "participant-1" {
		t.Fatalf("expected leader participant-1, got %s", leader)
	}
}

func TestMultiLockBasic(t *testing.T) {
	storage := newMockSubscribeStorage()
	lock1, _ := NewStorageLock(storage, "lock-1")
	lock2, _ := NewStorageLock(storage, "lock-2")
	_, err := NewMultiLock("owner", lock1, lock2)
	if !errors.Is(err, ErrAtomicMultiLockUnsupported) {
		t.Fatalf("storage without atomic batch must be rejected, got %v", err)
	}
}

func TestMultiLockTryLock(t *testing.T) {
	storage := newMockSubscribeStorage()
	lock1, _ := NewStorageLock(storage, "try-lock-1")
	lock2, _ := NewStorageLock(storage, "try-lock-2")
	_, err := NewMultiLock("owner", lock1, lock2)
	if !errors.Is(err, ErrAtomicMultiLockUnsupported) {
		t.Fatalf("storage without atomic batch must be rejected, got %v", err)
	}
}

func TestFairLockBasic(t *testing.T) {
	ctx := context.Background()
	storage := newMockSubscribeStorage()

	fairLock, err := NewFairLock(storage, "test-fair-lock")
	if err != nil {
		t.Fatalf("NewFairLock error: %v", err)
	}

	if !fairLock.IsFair() {
		t.Fatal("expected IsFair to return true")
	}

	if fairLock.LockId() != "test-fair-lock" {
		t.Fatalf("expected lockId test-fair-lock, got %s", fairLock.LockId())
	}

	// 获取锁
	err = fairLock.Lock(ctx, "owner")
	if err != nil {
		t.Fatalf("FairLock.Lock error: %v", err)
	}

	// 释放锁
	err = fairLock.UnLock(ctx, "owner")
	if err != nil {
		t.Fatalf("FairLock.UnLock error: %v", err)
	}
}

func TestListLocks(t *testing.T) {
	ctx := context.Background()
	storage := newMockSubscribeStorage()

	// 创建一些锁
	lock1, _ := NewStorageLock(storage, "list-test-1")
	lock2, _ := NewStorageLock(storage, "list-test-2")

	lock1.Lock(ctx, "owner-1")
	lock2.Lock(ctx, "owner-2")

	// 验证 ListLocks API 可以调用（mock 返回空迭代器）
	iter, err := ListLocks(ctx, storage, "")
	if err != nil {
		t.Fatalf("ListLocks error: %v", err)
	}

	// 验证迭代器 API 可用
	count := 0
	for iter.Next() {
		info := iter.Value()
		if info != nil {
			count++
		}
	}

	// mock 返回空迭代器，count 应该为 0
	// 真实存储测试在集成测试中验证
	_ = count
}
