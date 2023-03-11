package storage_lock

import "database/sql"

const DefaultSqlStorageTableName = "storage_lock"

type SqlStorageOptions struct {

	// 锁存放在哪个数据库下
	DatabaseName string

	// 存放锁的表的名字
	TableName string

	// 用于获取数据库连接
	ConnectionGetter ConnectionGetter[*sql.DB]
}
