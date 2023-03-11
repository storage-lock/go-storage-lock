package main

import (
	"context"
	"fmt"
	storage_lock "github.com/golang-infrastructure/go-storage-lock"
	"time"
)

func main() {

	connectionGetter := storage_lock.NewTidbStorageConnectionGetterFromDSN("")
	storageOptions := &storage_lock.TidbStorageOptions{
		ConnectionGetter: connectionGetter,
		DatabaseName:     "storage_lock_database",
		TableName:        "storage_lock_table",
	}
	storage := storage_lock.NewTidbStorage(storageOptions)
	lockOptions := &storage_lock.StorageLockOptions{
		LockId:                "must-serial-operation-resource-foo",
		LeaseExpireAfter:      time.Second * 30,
		LeaseRefreshInterval:  time.Second * 5,
		VersionMissRetryTimes: 3,
	}
	lock := storage_lock.NewStorageLock(storage, lockOptions)

	err := lock.Lock(context.Background())
	if err != nil {
		fmt.Println("获取锁失败: " + err.Error())
		return
	}

	fmt.Println("获取锁成功")

}
