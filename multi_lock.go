package storage_lock

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/storage-lock/go-events"
	go_storage "github.com/storage-lock/go-storage"
	storage_events "github.com/storage-lock/go-storage-events"
)

// ErrAtomicMultiLockUnsupported 表示后端没有把多个 key 放进同一次线性一致提交的能力。
// 这时不能退回逐把加锁：中途失败前，调用方已经持有子集，不是全有全无。
var ErrAtomicMultiLockUnsupported = errors.New("storage does not support atomic multi-key lock")

// MultiLock 在同一次存储提交里获取或释放多把独占锁。
// 后端必须实现 AtomicBatchStorage 并声明 CapabilityAtomicBatch。
// 不满足时 NewMultiLock 直接拒绝，而不是给出顺序加锁的 best-effort。
type MultiLock struct {
	storage  go_storage.AtomicBatchStorage
	executor *storage_events.WithEventSafeExecutor
	locks    []*StorageLock
	ownerID  string

	mu   sync.Mutex
	held bool
}

// NewMultiLock 创建原子联锁。所有锁必须来自同一个支持原子批次的存储，且 lock id 互不相同。
// ownerId 是整组锁的持有者；释放时必须仍是这个持有者。
func NewMultiLock(ownerId string, locks ...*StorageLock) (*MultiLock, error) {
	if ownerId == "" {
		return nil, ErrOwnerIdEmpty
	}
	if len(locks) == 0 {
		return nil, fmt.Errorf("MultiLock requires at least one lock")
	}
	var batch go_storage.AtomicBatchStorage
	seen := make(map[string]bool, len(locks))
	ids := make([]string, 0, len(locks))
	for _, lock := range locks {
		if lock == nil {
			return nil, fmt.Errorf("MultiLock contains nil lock")
		}
		candidate, ok := lock.storage.(go_storage.AtomicBatchStorage)
		if !ok || !go_storage.HasCapability(lock.storage, go_storage.CapabilityAtomicBatch) {
			return nil, fmt.Errorf("%w: %s", ErrAtomicMultiLockUnsupported, lock.storage.GetName())
		}
		if batch == nil {
			batch = candidate
		} else if batch != candidate {
			return nil, fmt.Errorf("MultiLock requires all locks to share one storage")
		}
		if seen[lock.options.LockId] {
			return nil, fmt.Errorf("MultiLock contains duplicate lock: %s", lock.options.LockId)
		}
		seen[lock.options.LockId] = true
		ids = append(ids, lock.options.LockId)
	}
	sort.SliceStable(locks, func(i, j int) bool {
		return locks[i].options.LockId < locks[j].options.LockId
	})
	return &MultiLock{
		storage:  batch,
		executor: storage_events.NewWithEventSafeExecutor(batch),
		locks:    locks,
		ownerID:  ownerId,
	}, nil
}

// Lock 一次提交获取全部锁。任一 key 的条件不成立时，一个 key 都不会被改成持有状态。
func (ml *MultiLock) Lock(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	ml.mu.Lock()
	defer ml.mu.Unlock()
	if ml.held {
		return nil
	}
	ops := make([]go_storage.CASOp, 0, len(ml.locks))
	for _, lock := range ml.locks {
		op, err := ml.acquireOp(ctx, lock)
		if err != nil {
			return err
		}
		ops = append(ops, op)
	}
	if err := ml.storage.CompareAndSwapBatch(ctx, ops); err != nil {
		return err
	}
	for i, lock := range ml.locks {
		lock.notifySubscribers(lock.options.LockId, ops[i].Information)
		if err := lock.startWatchDog(ctx, events.NewEvent(lock.options.LockId), lock.options.LockId, ml.ownerID, ops[i].Information); err != nil {
			ml.releaseHeld(ctx)
			return err
		}
	}
	ml.held = true
	return nil
}

// TryLock 与 Lock 相同：批次本身不会阻塞等待别人释放。
// 条件不成立时返回存储的版本冲突或已存在错误，不会留下部分持有。
func (ml *MultiLock) TryLock(ctx context.Context) error {
	return ml.Lock(ctx)
}

func (ml *MultiLock) acquireOp(ctx context.Context, lock *StorageLock) (go_storage.CASOp, error) {
	e := events.NewEvent(lock.options.LockId).SetOwnerId(ml.ownerID)
	info, err := lock.getLockInformation(ctx, e, lock.options.LockId)
	if err != nil && !errors.Is(err, ErrLockNotFound) {
		return go_storage.CASOp{}, err
	}
	now, err := lock.getTime(ctx, e.Fork())
	if err != nil {
		return go_storage.CASOp{}, err
	}
	if info == nil {
		created := &go_storage.LockInformation{
			LockId:          lock.options.LockId,
			OwnerId:         ml.ownerID,
			Version:         1,
			LockCount:       1,
			LockBeginTime:   now,
			LeaseExpireTime: now.Add(lock.options.LeaseExpireAfter),
		}
		return go_storage.CASOp{LockID: lock.options.LockId, NewVersion: 1, Information: created}, nil
	}
	active := info.LockCount > 0 && now.Before(info.LeaseExpireTime)
	if active && (info.ReadLock || info.OwnerId != ml.ownerID) {
		return go_storage.CASOp{}, fmt.Errorf("%w: %s", ErrLockBusy, lock.options.LockId)
	}
	// 空闲或过期只说明锁可以换人。公平队列的队首不是本次持有者时，不能组装这次条件写。
	if !active {
		if head := firstWriteWaiter(info.Waiters); head != nil {
			if head.OwnerId != ml.ownerID {
				return go_storage.CASOp{}, ErrNotYourTurn
			}
			info.Waiters = removeWaiter(info.Waiters, ml.ownerID, waiterModeWrite)
		}
	}
	if active && info.OwnerId == ml.ownerID {
		if lock.options.MaxReentrantDepth > 0 && info.LockCount >= lock.options.MaxReentrantDepth {
			return go_storage.CASOp{}, ErrMaxReentrantDepth
		}
	}
	old := info.Version
	next, err := nextVersion(old)
	if err != nil {
		return go_storage.CASOp{}, err
	}
	info.OwnerId = ml.ownerID
	info.ReadLock = false
	info.ReadOwners = nil
	if active {
		info.LockCount++
	} else {
		info.LockCount = 1
		info.LockBeginTime = now
	}
	info.Version = next
	info.LeaseExpireTime = now.Add(lock.options.LeaseExpireAfter)
	return go_storage.CASOp{LockID: lock.options.LockId, ExpectedVersion: old, NewVersion: next, Information: info}, nil
}

// UnLock 一次提交把全部已持有的锁写成墓碑。任一版本已经变化时整批不动。
func (ml *MultiLock) UnLock(ctx context.Context) error {
	ml.mu.Lock()
	defer ml.mu.Unlock()
	if !ml.held {
		return nil
	}
	if err := ml.releaseHeld(ctx); err != nil {
		return err
	}
	ml.held = false
	return nil
}

func (ml *MultiLock) releaseHeld(ctx context.Context) error {
	ops := make([]go_storage.CASOp, 0, len(ml.locks))
	for _, lock := range ml.locks {
		e := events.NewEvent(lock.options.LockId).SetOwnerId(ml.ownerID)
		info, err := lock.getLockInformation(ctx, e, lock.options.LockId)
		if err != nil {
			return err
		}
		if info.ReadLock || info.OwnerId != ml.ownerID || info.LockCount == 0 {
			return fmt.Errorf("%w: %s", ErrLockNotBelongYou, lock.options.LockId)
		}
		next, err := nextVersion(info.Version)
		if err != nil {
			return err
		}
		old := info.Version
		info.Version = next
		info.LockCount = 0
		info.OwnerId = ""
		ops = append(ops, go_storage.CASOp{
			LockID: lock.options.LockId, ExpectedVersion: old, NewVersion: next, Information: info,
		})
	}
	if err := ml.storage.CompareAndSwapBatch(ctx, ops); err != nil {
		return err
	}
	for _, lock := range ml.locks {
		lock.stopWatchDog(ctx, events.NewEvent(lock.options.LockId), nil)
	}
	return nil
}

// HeldCount 返回当前持有的锁数量。未持有时为 0。
func (ml *MultiLock) HeldCount() int {
	ml.mu.Lock()
	defer ml.mu.Unlock()
	if !ml.held {
		return 0
	}
	return len(ml.locks)
}

// LockIds 按获取顺序返回 lock id。顺序是 lock id 的字典序，用来避免调用方传入顺序造成死锁。
func (ml *MultiLock) LockIds() []string {
	ids := make([]string, len(ml.locks))
	for i, lock := range ml.locks {
		ids[i] = lock.options.LockId
	}
	return ids
}
