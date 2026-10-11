package storage_lock

import (
	"context"
	"fmt"
	"time"

	go_storage "github.com/storage-lock/go-storage"
	"github.com/golang-infrastructure/go-iterator"
)

// ListLocks 列出所有锁的信息。
// 支持按前缀过滤、按状态过滤。
// 注意：底层存储必须实现 Storage.List() 方法。
func ListLocks(ctx context.Context, storage go_storage.Storage, prefix string) (iterator.Iterator[*go_storage.LockInformation], error) {
	if storage == nil {
		return nil, ErrStorageNil
	}

	// 直接调用 Storage 的 List 方法
	iter, err := storage.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list locks: %w", err)
	}

	// 如果指定了前缀，进行过滤
	if prefix != "" {
		return &prefixFilterIterator{iter: iter, prefix: prefix}, nil
	}

	return iter, nil
}

// prefixFilterIterator 按前缀过滤的迭代器包装
type prefixFilterIterator struct {
	iter   iterator.Iterator[*go_storage.LockInformation]
	prefix string
}

func (p *prefixFilterIterator) Next() bool {
	for p.iter.Next() {
		info := p.iter.Value()
		if info != nil && len(info.LockId) >= len(p.prefix) && info.LockId[:len(p.prefix)] == p.prefix {
			return true
		}
	}
	return false
}

func (p *prefixFilterIterator) Value() *go_storage.LockInformation {
	return p.iter.Value()
}

// ListHeldLocks 列出当前被有效持有的锁。
func ListHeldLocks(ctx context.Context, storage go_storage.Storage, prefix string) (iterator.Iterator[*go_storage.LockInformation], error) {
	iter, err := ListLocks(ctx, storage, prefix)
	if err != nil {
		return nil, err
	}

	return &heldFilterIterator{iter: iter}, nil
}

// heldFilterIterator 过滤出被持有的锁
type heldFilterIterator struct {
	iter iterator.Iterator[*go_storage.LockInformation]
}

func (h *heldFilterIterator) Next() bool {
	now := time.Now()
	for h.iter.Next() {
		info := h.iter.Value()
		if info != nil && info.LockCount > 0 && now.Before(info.LeaseExpireTime) {
			return true
		}
	}
	return false
}

func (h *heldFilterIterator) Value() *go_storage.LockInformation {
	return h.iter.Value()
}

// CountLocks 统计锁的数量（需要遍历迭代器）。
func CountLocks(ctx context.Context, storage go_storage.Storage, prefix string) (int, error) {
	iter, err := ListLocks(ctx, storage, prefix)
	if err != nil {
		return 0, err
	}

	count := 0
	for iter.Next() {
		count++
	}
	return count, nil
}
