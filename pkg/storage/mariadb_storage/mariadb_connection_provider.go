package mariadb_storage

import (
	"database/sql"
	"github.com/storage-lock/go-storage-lock/pkg/storage"
	"github.com/storage-lock/go-storage-lock/pkg/storage/mysql_storage"
)

// MariaDBConnectionGetter 创建一个Maria的连接
type MariaDBConnectionGetter struct {
	*mysql_storage.MySQLConnectionProvider
}

var _ storage.ConnectionProvider[*sql.DB] = &MariaDBConnectionGetter{}

// NewMariaStorageConnectionProviderFromDSN 从DSN创建Maria连接
func NewMariaStorageConnectionProviderFromDSN(dsn string) *MariaDBConnectionGetter {
	return &MariaDBConnectionGetter{
		MySQLConnectionProvider: mysql_storage.NewMySQLConnectionProviderFromDSN(dsn),
	}
}

// NewMariaStorageConnectionProvider 从服务器属性创建数据库连接
func NewMariaStorageConnectionProvider(host string, port uint, user, passwd, database string) *MariaDBConnectionGetter {
	return &MariaDBConnectionGetter{
		MySQLConnectionProvider: mysql_storage.NewMySQLConnectionProvider(host, port, user, passwd, database),
	}
}
