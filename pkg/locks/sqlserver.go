package locks

import (
	"context"
	"github.com/storage-lock/go-storage-lock/pkg/storage"
	"github.com/storage-lock/go-storage-lock/pkg/storage/sql_server_storage"
	"github.com/storage-lock/go-storage-lock/pkg/storage_lock"
)

// NewSqlServerStorageLock 高层API，使用默认配置快速创建基于SQLServer的分布式锁
func NewSqlServerStorageLock(ctx context.Context, lockId string, dsn string) (*storage_lock.StorageLock, error) {
	connectionGetter := sql_server_storage.NewSqlServerStorageConnectionGetterFromDSN(dsn)
	storageOptions := &sql_server_storage.SqlServerStorageOptions{
		ConnectionProvider: connectionGetter,
		TableName:          storage.DefaultStorageTableName,
	}

	s, err := sql_server_storage.NewSqlServerStorage(ctx, storageOptions)
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
