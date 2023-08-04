package locks

import (
	"context"
	storage2 "github.com/storage-lock/go-storage-lock/pkg/storage"
	"github.com/storage-lock/go-storage-lock/pkg/storage/postgresql_storage"
	"github.com/storage-lock/go-storage-lock/pkg/storage_lock"
)

// NewPostgreSQLStorageLock 高层API，使用默认配置快速创建基于PostgreSQL的分布式锁
func NewPostgreSQLStorageLock(ctx context.Context, lockId string, dsn string, schema ...string) (*storage_lock.StorageLock, error) {
	connectionGetter := postgresql_storage.NewPostgreSQLConnectionGetterFromDSN(dsn)
	storageOptions := &postgresql_storage.PostgreSQLStorageOptions{
		ConnectionManager: connectionGetter,
		TableName:         storage2.DefaultStorageTableName,
	}

	if len(schema) != 0 {
		storageOptions.Schema = schema[0]
	}

	storage, err := postgresql_storage.NewPostgreSQLStorage(ctx, storageOptions)
	if err != nil {
		return nil, err
	}

	lockOptions := &storage_lock.StorageLockOptions{
		LockId:                lockId,
		LeaseExpireAfter:      storage_lock.DefaultLeaseExpireAfter,
		LeaseRefreshInterval:  storage_lock.DefaultLeaseRefreshInterval,
		VersionMissRetryTimes: storage_lock.DefaultVersionMissRetryTimes,
	}
	return storage_lock.NewStorageLock(storage, lockOptions), nil
}
