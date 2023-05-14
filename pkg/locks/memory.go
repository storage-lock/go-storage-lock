package locks

import (
	"github.com/storage-lock/go-storage-lock/pkg/storage/memory_storage"
	"github.com/storage-lock/go-storage-lock/pkg/storage_lock"
)

func NewStorageLockUseMemory(lockId string) *storage_lock.StorageLock {
	return storage_lock.NewStorageLock(memory_storage.NewMemoryStorage(), &storage_lock.StorageLockOptions{
		LockId:                lockId,
		LeaseExpireAfter:      storage_lock.DefaultLeaseExpireAfter,
		LeaseRefreshInterval:  storage_lock.DefaultLeaseRefreshInterval,
		VersionMissRetryTimes: storage_lock.DefaultVersionMissRetryTimes,
	})
}
