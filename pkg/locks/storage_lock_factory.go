package locks

import (
	"github.com/storage-lock/go-storage-lock/pkg/storage"
)

// StorageLockFactory 锁的工厂方法
type StorageLockFactory[Connection any] struct {
	ConnectionProvider storage.ConnectionProvider[Connection]
}
