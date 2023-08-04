package connection_manager

import (
	"context"
	"database/sql"
)

// FixedSqlDBConnectionManager 每次创建连接都返回固定的 *sql.DB 实例，用于在其他地方已经创建了一个*sql.DB的时候与其共享连接资源
// TODO 2023-8-4 01:35:23 单元测试，虽然感觉没啥必要这要都能出错我跑去厕所大吃特吃！
type FixedSqlDBConnectionManager struct {
	db *sql.DB
}

var _ ConnectionManager[*sql.DB] = &FixedSqlDBConnectionManager{}

func NewFixedSqlDBConnectionManager(db *sql.DB) *FixedSqlDBConnectionManager {
	return &FixedSqlDBConnectionManager{
		db: db,
	}
}

func (x *FixedSqlDBConnectionManager) Name() string {
	return "fixed-sql-db-connection-manager"
}

func (x *FixedSqlDBConnectionManager) Take(ctx context.Context) (*sql.DB, error) {
	return x.db, nil
}

func (x *FixedSqlDBConnectionManager) Return(ctx context.Context, db *sql.DB) error {
	return nil
}

func (x *FixedSqlDBConnectionManager) Shutdown(ctx context.Context) error {
	if x.db != nil {
		return x.db.Close()
	}
	return nil
}

