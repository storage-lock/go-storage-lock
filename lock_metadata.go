package storage_lock

import (
	"context"
	"errors"
	"time"

	go_storage "github.com/storage-lock/go-storage"
)

// LockMetadata 锁的元数据快照；Extra 是持有期间随锁记录原子保存的自定义 string map。
type LockMetadata struct {
	// 内置字段
	LockId          string
	OwnerId         string
	Version         go_storage.Version
	LockCount       int
	LockBeginTime   time.Time
	LeaseExpireTime time.Time
	ReadLock        bool

	// 扩展元数据，业务可自由附加
	// 例如：{"taskId": "12345", "source": "api-server-01", "priority": "high"}
	Extra map[string]string
}

// StorageLockWithMetadata 带元数据扩展的锁包装器。
// 在获取锁时允许附加自定义元数据。
type StorageLockWithMetadata struct {
	*StorageLock
}

// NewStorageLockWithMetadata 创建带元数据扩展的锁。
func NewStorageLockWithMetadata(storage go_storage.Storage, lockId string) (*StorageLockWithMetadata, error) {
	lock, err := NewStorageLock(storage, lockId)
	if err != nil {
		return nil, err
	}
	return &StorageLockWithMetadata{StorageLock: lock}, nil
}

// LockWithMetadata 获取锁并附加元数据。
// Extra 通过 CAS 随锁记录存储；重入获取会替换为本次传入的 Extra（nil 会清空）。
// 元数据写入失败时会尽力撤销本次获取，并将写入错误与清理错误合并返回。
func (lm *StorageLockWithMetadata) LockWithMetadata(ctx context.Context, ownerId string, metadata *LockMetadata) error {
	if err := lm.Lock(ctx, ownerId); err != nil {
		return err
	}
	if metadata == nil {
		return nil
	}

	if err := lm.updateMetadata(ctx, ownerId, metadata.Extra); err != nil {
		cleanupCtx, cancel := lm.cleanupContext(ctx)
		defer cancel()
		return errors.Join(err, lm.UnLock(cleanupCtx, ownerId))
	}
	return nil
}

func (lm *StorageLockWithMetadata) updateMetadata(ctx context.Context, ownerId string, extra map[string]string) error {
	e := lm.operationEvent(ownerId)
	for {
		info, err := lm.getLockInformation(ctx, e.Fork(), lm.options.LockId)
		if err != nil {
			return err
		}
		if err := checkExclusiveOwner(info, ownerId); err != nil {
			return err
		}
		expires, err := lm.activeLeaseExpiry(ctx, e.Fork(), info)
		if err != nil {
			return err
		}

		previous := info.Version
		next, err := nextVersion(previous)
		if err != nil {
			return err
		}
		info.Version = next
		info.LeaseExpireTime = expires
		info.Extra = cloneMetadata(extra)
		if err := lm.storageExecutor.UpdateWithVersion(ctx, e.Fork(), info.LockId, previous, next, info); err != nil {
			if !errors.Is(err, ErrVersionMiss) {
				return err
			}
			if err := waitContext(ctx, lm.options.VersionMissRetryInterval+lm.retryIntervalRandomBase()); err != nil {
				return err
			}
			continue
		}
		lm.notifySubscribers(info.LockId, info)
		return nil
	}
}

func cloneMetadata(extra map[string]string) map[string]string {
	if extra == nil {
		return nil
	}
	clone := make(map[string]string, len(extra))
	for key, value := range extra {
		clone[key] = value
	}
	return clone
}

// GetMetadata 获取锁的元数据（包括扩展字段）。
// GetMetadata 返回当前记录的快照；已释放的墓碑记录仍可读，是否 Held 应以 Inspect 为准。
func (lm *StorageLockWithMetadata) GetMetadata(ctx context.Context) (*LockMetadata, error) {
	status, err := lm.Inspect(ctx)
	if err != nil {
		return nil, err
	}

	return &LockMetadata{
		LockId:          status.Information.LockId,
		OwnerId:         status.Information.OwnerId,
		Version:         status.Information.Version,
		LockCount:       int(status.Information.LockCount),
		LockBeginTime:   status.Information.LockBeginTime,
		LeaseExpireTime: status.Information.LeaseExpireTime,
		ReadLock:        status.Information.ReadLock,
		Extra:           cloneMetadata(status.Information.Extra),
	}, nil
}
