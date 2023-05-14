package mysql_storage

import (
	"database/sql"
	"github.com/storage-lock/go-storage-lock/pkg/storage"
)

// MySQLStorageOptions 基于MySQL为存储引擎时的选项
type MySQLStorageOptions struct {

	// 存放锁的表的名字，如果未指定的话则使用默认的表
	TableName string

	// 用于获取数据库连接
	ConnectionProvider storage.ConnectionProvider[*sql.DB]
}

func NewMySQLStorageOptions() *MySQLStorageOptions {
	return &MySQLStorageOptions{
		TableName: storage.DefaultStorageTableName,
	}
}

func (x *MySQLStorageOptions) WithConnectionProvider(connProvider storage.ConnectionProvider[*sql.DB]) *MySQLStorageOptions {
	x.ConnectionProvider = connProvider
	return x
}

func (x *MySQLStorageOptions) WithTableName(tableName string) *MySQLStorageOptions {
	x.TableName = tableName
	return x
}
