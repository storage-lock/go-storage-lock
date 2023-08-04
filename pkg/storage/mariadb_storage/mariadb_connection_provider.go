package mariadb_storage

import (
	"database/sql"
	"github.com/storage-lock/go-storage-lock/pkg/storage/connection_manager"
	"github.com/storage-lock/go-storage-lock/pkg/storage/mysql_storage"
)

// MariaDBConnectionManager 创建一个Maria的连接
type MariaDBConnectionManager struct {
	// 其实底层都是基于mysql的
	*mysql_storage.MySQLConnectionManager
}

var _ connection_manager.ConnectionManager[*sql.DB] = &MariaDBConnectionManager{}

// NewMariaStorageConnectionProviderFromDSN 从DSN创建Maria连接
func NewMariaStorageConnectionProviderFromDSN(dsn string) *MariaDBConnectionManager {
	return &MariaDBConnectionManager{
		MySQLConnectionManager: mysql_storage.NewMySQLConnectionProviderFromDSN(dsn),
	}
}

// NewMariaStorageConnectionProvider 从服务器属性创建数据库连接
func NewMariaStorageConnectionProvider(host string, port uint, user, passwd, database string) *MariaDBConnectionManager {
	return &MariaDBConnectionManager{
		MySQLConnectionManager: mysql_storage.NewMySQLConnectionProvider(host, port, user, passwd, database),
	}
}
