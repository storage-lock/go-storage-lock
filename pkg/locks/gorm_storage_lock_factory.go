package locks

import (
	"context"
	"github.com/storage-lock/go-storage-lock/pkg/storage_lock"
	"gorm.io/gorm"
)

// GormStorageLockFactory 基于GORM创建分布式锁
type GormStorageLockFactory struct {
	db *gorm.DB
}

func NewGormStorageLockFactory(db *gorm.DB) *GormStorageLockFactory {
	return &GormStorageLockFactory{
		db: db,
	}
}

func (x *GormStorageLockFactory) CreateLock(ctx context.Context) (*storage_lock.StorageLock, error) {
	name := x.db.Dialector.Name()
	switch name {
	case "mysql":

	}
}
