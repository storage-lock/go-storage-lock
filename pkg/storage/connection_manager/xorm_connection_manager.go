package connection_manager

import (
	"context"
	"database/sql"
	"xorm.io/xorm"
)

// XormConnectionManager 用来复用xorm（https://github.com/go-xorm/xorm）的数据库连接
// TODO 2023-8-4 01:28:15 单元测试
type XormConnectionManager struct {
	engine *xorm.Engine
}

var _ ConnectionManager[*sql.DB] = &XormConnectionManager{}

func NewXormConnectionManager(engine *xorm.Engine) *XormConnectionManager {
	return &XormConnectionManager{
		engine: engine,
	}
}

func (x *XormConnectionManager) Name() string {
	return "xorm-connection-manager"
}

func (x *XormConnectionManager) Take(ctx context.Context) (*sql.DB, error) {
	return x.engine.DB().DB, nil
}

func (x *XormConnectionManager) Return(ctx context.Context, db *sql.DB) error {
	return nil
}

func (x *XormConnectionManager) Shutdown(ctx context.Context) error {
	return nil
}
