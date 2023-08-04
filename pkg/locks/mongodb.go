package locks

import (
	"context"
	"github.com/storage-lock/go-storage-lock/pkg/storage"
	"github.com/storage-lock/go-storage-lock/pkg/storage/mongodb_storage"
	"github.com/storage-lock/go-storage-lock/pkg/storage_lock"
)

// NewMongoStorageLock 高层API，使用默认配置快速创建基于Mongo的分布式锁
func NewMongoStorageLock(ctx context.Context, lockId string, uri string) (*storage_lock.StorageLock, error) {
	connectionGetter := mongodb_storage.NewMongoConnectionManager(uri)
	storageOptions := &mongodb_storage.MongoStorageOptions{
		ConnectionProvider: connectionGetter,
		DatabaseName:       storage.DefaultStorageTableName,
		CollectionName:     storage.DefaultStorageTableName,
	}

	s, err := mongodb_storage.NewMongoStorage(ctx, storageOptions)
	if err != nil {
		return nil, err
	}

	lockOptions := &storage_lock.StorageLockOptions{
		LockId:                lockId,
		LeaseExpireAfter:      storage_lock.DefaultLeaseExpireAfter,
		LeaseRefreshInterval:  storage_lock.DefaultLeaseRefreshInterval,
		VersionMissRetryTimes: storage_lock.DefaultVersionMissRetryTimes,
	}
	return storage_lock.NewStorageLock(s, lockOptions), nil
}
