package locks

import (
	"github.com/storage-lock/go-storage-lock/pkg/storage"
	"gorm.io/gorm/clause"
)

// StorageLockFactory 锁的工厂方法
type StorageLockFactory[Connection, Options any] struct {
	ConnectionProvider storage.ConnectionManager[Connection]
}

func NewStorageLockFactory() *StorageLockFactory[] {}

