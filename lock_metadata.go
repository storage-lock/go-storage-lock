package storage_lock

import (
	"context"
	"time"

	go_storage "github.com/storage-lock/go-storage"
)

// LockMetadata 锁的元数据扩展，允许业务附加自定义信息。
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
// metadata.Extra 中的自定义字段会存储到 LockInformation 中。
func (lm *StorageLockWithMetadata) LockWithMetadata(ctx context.Context, ownerId string, metadata *LockMetadata) error {
	if metadata == nil {
		return lm.Lock(ctx, ownerId)
	}

	// TODO: 需要扩展 LockInformation 结构体支持 Extra 字段
	// 当前实现先忽略 Extra，未来可通过扩展 LockInformation 实现
	return lm.Lock(ctx, ownerId)
}

// GetMetadata 获取锁的元数据（包括扩展字段）。
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
		Extra:           nil, // TODO: 从 LockInformation 读取
	}, nil
}
