package sql_server_storage

import (
	"database/sql"
	"github.com/storage-lock/go-storage-lock/pkg/storage"
)

type SqlServerStorageOptions struct {

	// 存放锁的表的名字
	TableName string

	// 用于获取数据库连接
	ConnectionProvider storage.ConnectionProvider[*sql.DB]
}

func NewSqlServerStorageOptions() *SqlServerStorageOptions {
	return &SqlServerStorageOptions{
		TableName: storage.DefaultStorageTableName,
	}
}

func (x *SqlServerStorageOptions) WithTableName(tableName string) *SqlServerStorageOptions {
	x.TableName = tableName
	return x
}

func (x *SqlServerStorageOptions) WithConnectionProvider(connectionProvider storage.ConnectionProvider[*sql.DB]) *SqlServerStorageOptions {
	x.ConnectionProvider = connectionProvider
	return x
}
