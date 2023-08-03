package locks

import (
	"context"
	"github.com/storage-lock/go-storage-lock/pkg/storage"
	"github.com/storage-lock/go-storage-lock/pkg/storage/mysql_storage"
	"github.com/storage-lock/go-storage-lock/pkg/storage/tidb_storage"
	"github.com/storage-lock/go-storage-lock/pkg/storage_lock"
)

// NewTidbStorageLock 高层API，使用默认配置快速创建基于TiDB的分布式锁
// lockId: 要锁住的资源的唯一ID
// dsn: 用来存储锁的TiDB数据库连接方式
func NewTidbStorageLock(ctx context.Context, lockId string, dsn string) (*storage_lock.StorageLock, error) {
	connectionProvider := tidb_storage.NewTidbConnectionProviderFromDSN(dsn)
	storageOptions := &tidb_storage.TidbStorageOptions{
		MySQLStorageOptions: &mysql_storage.MySQLStorageOptions{
			ConnectionProvider: connectionProvider,
			TableName:          storage.DefaultStorageTableName,
		},
	}

	s, err := tidb_storage.NewTidbStorage(ctx, storageOptions)
	if err != nil {
		return nil, err
	}

	lockOptions := &storage_lock.StorageLockOptions{
		LockId:                lockId,
		LeaseExpireAfter:      storage_lock.DefaultLeaseExpireAfter,
		LeaseRefreshInterval:  storage_lock.DefaultLeaseRefreshInterval,
		//VersionMissRetryTimes: storage_lock.DefaultVersionMissRetryTimes,
	}
	return storage_lock.NewStorageLock(s, lockOptions), nil
}
