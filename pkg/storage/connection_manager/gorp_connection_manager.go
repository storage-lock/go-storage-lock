package connection_manager

import (
	"context"
	"database/sql"
	"gopkg.in/gorp.v1"
)

// GorpConnectionManager 复用gorp的数据库连接（https://github.com/go-gorp/gorp）
// TODO 2023-8-4 01:28:03 单元测试
type GorpConnectionManager struct {
	dbMap *gorp.DbMap
}

var _ ConnectionManager[*sql.DB] = &GorpConnectionManager{}

func NewGorpConnectionManager(dbMap *gorp.DbMap) *GorpConnectionManager {
	return &GorpConnectionManager{
		dbMap: dbMap,
	}
}

func (x *GorpConnectionManager) Name() string {
	return "gorp-connection-manager"
}

func (x *GorpConnectionManager) Take(ctx context.Context) (*sql.DB, error) {
	return x.dbMap.Db, nil
}

func (x *GorpConnectionManager) Return(ctx context.Context, db *sql.DB) error {
	return nil
}

func (x *GorpConnectionManager) Shutdown(ctx context.Context) error {
	return nil
}


