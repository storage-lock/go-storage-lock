package main

import (
	"context"
	"fmt"
	"github.com/storage-lock/go-storage-lock/pkg/storage/mysql_storage"
	"github.com/storage-lock/go-storage-lock/pkg/storage/tidb_storage"
	"github.com/storage-lock/go-storage-lock/pkg/storage_lock"
	"time"
)

func main() {

	connectionGetter := tidb_storage.NewTidbConnectionProviderFromDSN("")
	storageOptions := &tidb_storage.TidbStorageOptions{
		MySQLStorageOptions: &mysql_storage.MySQLStorageOptions{
			ConnectionProvider: connectionGetter,
			TableName:          "storage_lock_table",
		},
	}
	storage, err := tidb_storage.NewTidbStorage(context.Background(), storageOptions)
	if err != nil {
		panic(err)
	}
	lockOptions := &storage_lock.StorageLockOptions{
		LockId:                "must-serial-operation-resource-foo",
		LeaseExpireAfter:      time.Second * 30,
		LeaseRefreshInterval:  time.Second * 5,
		VersionMissRetryTimes: 3,
	}
	lock := storage_lock.NewStorageLock(storage, lockOptions)

	err = lock.Lock(context.Background())
	if err != nil {
		fmt.Println("获取锁失败: " + err.Error())
		return
	}

	fmt.Println("获取锁成功")

}
