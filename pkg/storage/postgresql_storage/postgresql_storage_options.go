package postgresql_storage

import (
	"database/sql"
	"github.com/storage-lock/go-storage-lock/pkg/storage"
)

type PostgreSQLStorageOptions struct {

	// 存在哪个schema下，默认是public
	Schema string

	// 存放锁的表的名字
	TableName string

	// 用于获取数据库连接
	ConnectionProvider storage.ConnectionProvider[*sql.DB]
}

func NewPostgreSQLStorageOptions() *PostgreSQLStorageOptions {
	return &PostgreSQLStorageOptions{
		Schema: DefaultPostgreSQLStorageSchema,
	}
}

func (x *PostgreSQLStorageOptions) WithSchema(schema string) *PostgreSQLStorageOptions {
	x.Schema = schema
	return x
}

func (x *PostgreSQLStorageOptions) WithTableName(tableName string) *PostgreSQLStorageOptions {
	x.TableName = tableName
	return x
}

func (x *PostgreSQLStorageOptions) WithConnectionProvider(connectionProvider storage.ConnectionProvider[*sql.DB]) *PostgreSQLStorageOptions {
	x.ConnectionProvider = connectionProvider
	return x
}
