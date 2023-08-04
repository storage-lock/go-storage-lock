package connection_manager

import (
	"context"
	"database/sql"
	"gorm.io/gorm"
)

// GormConnectionManager 从Gorm（https://github.com/go-gorm/gorm）获取数据库连接，如果当前项目是引入的gorm的话则可以与其复用数据库连接资源
// TODO 2023-8-4 01:34:49 单元测试
type GormConnectionManager struct {
	db *gorm.DB
}

var _ ConnectionManager[*sql.DB] = &GormConnectionManager{}

func NewGormConnectionManager(db *gorm.DB) *GormConnectionManager {
	return &GormConnectionManager{
		db: db,
	}
}

func (x *GormConnectionManager) Name() string {
	return "gorm-connection-manager"
}

func (x *GormConnectionManager) Take(ctx context.Context) (*sql.DB, error) {
	// TODO 2023-8-4 00:18:11 从gorm的连接池中获取的连接如果不手动调用close的话会不会发生资源泄露？
	//  如果会的话是不是在Return的时候Close一下就行？
	return x.db.DB()
}

func (x *GormConnectionManager) Return(ctx context.Context, db *sql.DB) error {
	return nil
}

func (x *GormConnectionManager) Shutdown(ctx context.Context) error {
	return nil
}
