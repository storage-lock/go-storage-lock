package locks

import (
	"context"
	"github.com/storage-lock/go-storage-lock/pkg/storage/mariadb_storage"
	"github.com/storage-lock/go-storage-lock/pkg/storage_lock"
)

// NewMariaDBStorageLock 高层API，使用默认配置快速创建基于MariaDB的分布式锁
func NewMariaDBStorageLock(ctx context.Context, lockId string, dsn string) (*storage_lock.StorageLock, error) {

	connectionProvider := mariadb_storage.NewMariaStorageConnectionProviderFromDSN(dsn)
	storageOptions := mariadb_storage.NewMariaStorageOptions()
	storageOptions.WithConnectionProvider(connectionProvider)

	storage, err := mariadb_storage.NewMariaDbStorage(ctx, storageOptions)
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
