package storage_lock

import (
	"context"
	"testing"
	"time"

	go_storage "github.com/storage-lock/go-storage"
	"github.com/golang-infrastructure/go-iterator"
)

// mockIteratorForQuery 用于测试 lock_query 的 mock 迭代器
type mockIteratorForQuery struct {
	items []*go_storage.LockInformation
	index int
}

func (m *mockIteratorForQuery) Next() bool {
	if m.index < len(m.items) {
		m.index++
		return true
	}
	return false
}

func (m *mockIteratorForQuery) Value() *go_storage.LockInformation {
	if m.index > 0 && m.index <= len(m.items) {
		return m.items[m.index-1]
	}
	return nil
}

// mockStorageWithList 返回带有自定义 List 实现的 mock storage
type mockStorageWithList struct {
	*mockSubscribeStorage
	listFunc func(ctx context.Context) (iterator.Iterator[*go_storage.LockInformation], error)
}

func (m *mockStorageWithList) List(ctx context.Context) (iterator.Iterator[*go_storage.LockInformation], error) {
	if m.listFunc != nil {
		return m.listFunc(ctx)
	}
	return m.mockSubscribeStorage.List(ctx)
}

// TestListLocksWithCustomIterator 测试带自定义迭代器的 ListLocks
func TestListLocksWithCustomIterator(t *testing.T) {
	ctx := context.Background()

	// 创建带自定义 List 的 storage
	storage := &mockStorageWithList{
		mockSubscribeStorage: newMockSubscribeStorage(),
		listFunc: func(ctx context.Context) (iterator.Iterator[*go_storage.LockInformation], error) {
			return &mockIteratorForQuery{
				items: []*go_storage.LockInformation{
					{LockId: "test-lock-1", LockCount: 1},
					{LockId: "test-lock-2", LockCount: 0},
				},
				index: 0,
			}, nil
		},
	}

	iter, err := ListLocks(ctx, storage, "")
	if err != nil {
		t.Fatalf("ListLocks error: %v", err)
	}

	count := 0
	for iter.Next() {
		info := iter.Value()
		if info != nil {
			count++
		}
	}

	if count != 2 {
		t.Fatalf("expected 2 locks, got %d", count)
	}
}

// TestListLocksPrefixFilter 测试前缀过滤逻辑
func TestListLocksPrefixFilter(t *testing.T) {
	ctx := context.Background()

	storage := &mockStorageWithList{
		mockSubscribeStorage: newMockSubscribeStorage(),
		listFunc: func(ctx context.Context) (iterator.Iterator[*go_storage.LockInformation], error) {
			return &mockIteratorForQuery{
				items: []*go_storage.LockInformation{
					{LockId: "app-lock-1"},
					{LockId: "app-lock-2"},
					{LockId: "db-lock-1"},
				},
				index: 0,
			}, nil
		},
	}

	// 按 "app" 前缀过滤
	iter, err := ListLocks(ctx, storage, "app")
	if err != nil {
		t.Fatalf("ListLocks error: %v", err)
	}

	count := 0
	for iter.Next() {
		info := iter.Value()
		if info != nil && len(info.LockId) >= 3 && info.LockId[:3] == "app" {
			count++
		}
	}

	if count != 2 {
		t.Fatalf("expected 2 locks with app prefix, got %d", count)
	}
}

// TestListHeldLocksWithHeldItems 测试 ListHeldLocks 过滤持有的锁
func TestListHeldLocksWithHeldItems(t *testing.T) {
	ctx := context.Background()

	now := time.Now()
	storage := &mockStorageWithList{
		mockSubscribeStorage: newMockSubscribeStorage(),
		listFunc: func(ctx context.Context) (iterator.Iterator[*go_storage.LockInformation], error) {
			return &mockIteratorForQuery{
				items: []*go_storage.LockInformation{
					{LockId: "held-1", LockCount: 1, LeaseExpireTime: now.Add(time.Hour)},
					{LockId: "held-2", LockCount: 0, LeaseExpireTime: now.Add(time.Hour)},
					{LockId: "held-3", LockCount: 1, LeaseExpireTime: now.Add(-time.Hour)}, // 已过期
				},
				index: 0,
			}, nil
		},
	}

	iter, err := ListHeldLocks(ctx, storage, "")
	if err != nil {
		t.Fatalf("ListHeldLocks error: %v", err)
	}

	count := 0
	for iter.Next() {
		info := iter.Value()
		if info != nil {
			count++
		}
	}

	// 只有 held-1 满足条件
	if count != 1 {
		t.Fatalf("expected 1 held lock, got %d", count)
	}
}

// TestCountLocksWithItems 测试 CountLocks 计数
func TestCountLocksWithItems(t *testing.T) {
	ctx := context.Background()

	storage := &mockStorageWithList{
		mockSubscribeStorage: newMockSubscribeStorage(),
		listFunc: func(ctx context.Context) (iterator.Iterator[*go_storage.LockInformation], error) {
			return &mockIteratorForQuery{
				items: []*go_storage.LockInformation{
					{LockId: "count-1"},
					{LockId: "count-2"},
					{LockId: "count-3"},
				},
				index: 0,
			}, nil
		},
	}

	count, err := CountLocks(ctx, storage, "")
	if err != nil {
		t.Fatalf("CountLocks error: %v", err)
	}

	if count != 3 {
		t.Fatalf("expected 3 locks, got %d", count)
	}
}

// TestListLocksStorageError 测试存储层返回错误
func TestListLocksStorageError(t *testing.T) {
	ctx := context.Background()

	storage := &mockStorageWithList{
		mockSubscribeStorage: newMockSubscribeStorage(),
		listFunc: func(ctx context.Context) (iterator.Iterator[*go_storage.LockInformation], error) {
			return nil, ErrLockNotFound
		},
	}

	_, err := ListLocks(ctx, storage, "")
	if err == nil {
		t.Fatal("expected error from storage")
	}
}
