package tidb_storage

import (
	"database/sql"
	"github.com/storage-lock/go-storage-lock/pkg/storage/connection_manager"
	"github.com/storage-lock/go-storage-lock/pkg/storage/mysql_storage"
)

// TidbConnectionGetter 创建一个TIDB的连接
type TidbConnectionGetter struct {
	// tidb底层实际上都是跟mysql通用的
	*mysql_storage.MySQLConnectionManager
}

var _ connection_manager.ConnectionManager[*sql.DB] = &TidbConnectionGetter{}

// NewTidbConnectionProviderFromDSN 从DSN创建tidb连接
func NewTidbConnectionProviderFromDSN(dsn string) *TidbConnectionGetter {
	return &TidbConnectionGetter{
		MySQLConnectionManager: mysql_storage.NewMySQLConnectionProviderFromDSN(dsn),
	}
}

// NewTidbStorageConnectionProvider 从服务器属性创建数据库连接
func NewTidbStorageConnectionProvider(host string, port uint, user, passwd, databaseName string) *TidbConnectionGetter {
	return &TidbConnectionGetter{
		MySQLConnectionManager: mysql_storage.NewMySQLConnectionProvider(host, port, user, passwd, databaseName),
	}
}
