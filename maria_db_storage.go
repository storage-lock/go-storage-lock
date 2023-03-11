package storage_lock

import (
	"context"
	"database/sql"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// ------------------------------------------------- --------------------------------------------------------------------

// MariaStorageConnectionGetter 创建一个Maria的连接
type MariaStorageConnectionGetter struct {
	*MySQLStorageConnectionGetter
}

var _ ConnectionGetter[*sql.DB] = &MariaStorageConnectionGetter{}

// NewMariaStorageConnectionGetterFromDSN 从DSN创建Maria连接
func NewMariaStorageConnectionGetterFromDSN(dsn string) *MariaStorageConnectionGetter {
	return &MariaStorageConnectionGetter{
		MySQLStorageConnectionGetter: NewMySQLStorageConnectionGetterFromDSN(dsn),
	}
}

// NewMariaStorageConnectionGetter 从服务器属性创建数据库连接
func NewMariaStorageConnectionGetter(host string, port uint, user, passwd string) *MariaStorageConnectionGetter {
	return &MariaStorageConnectionGetter{
		MySQLStorageConnectionGetter: NewMySQLStorageConnectionGetter(host, port, user, passwd),
	}
}

// Get 获取到数据库的连接
func (x *MariaStorageConnectionGetter) Get(ctx context.Context) (*sql.DB, error) {
	return x.MySQLStorageConnectionGetter.Get(ctx)
}

// ------------------------------------------------- --------------------------------------------------------------------

const DefaultMariaStorageTableName = "storage_lock"

type MariaStorageOptions struct {

	// 锁存放在哪个数据库下
	DatabaseName string

	// 存放锁的表的名字
	TableName string

	// 用于获取数据库连接
	ConnectionGetter ConnectionGetter[*sql.DB]
}

func (x *MariaStorageOptions) ToMySQLStorageOptions() *MySQLStorageOptions {
	return &MySQLStorageOptions{
		DatabaseName:     x.DatabaseName,
		TableName:        x.TableName,
		ConnectionGetter: x.ConnectionGetter,
	}
}

// ------------------------------------------------- --------------------------------------------------------------------

type MariaDbStorage struct {
	*MySQLStorage

	options *MariaStorageOptions
}

var _ Storage = &MariaDbStorage{}

func NewMariaDbStorage(options *MariaStorageOptions) *MariaDbStorage {
	return &MariaDbStorage{
		options:      options,
		MySQLStorage: NewMySQLStorage(options.ToMySQLStorageOptions()),
	}
}

func (x *MariaDbStorage) Init(ctx context.Context) error {
	return x.MySQLStorage.Init(ctx)
}

func (x *MariaDbStorage) UpdateWithVersion(ctx context.Context, lockId string, exceptedVersion, newVersion Version, lockInformationJsonString string) error {
	return x.MySQLStorage.UpdateWithVersion(ctx, lockId, exceptedVersion, newVersion, lockInformationJsonString)
}

func (x *MariaDbStorage) InsertWithVersion(ctx context.Context, lockId string, version Version, lockInformationJsonString string) error {
	return x.MySQLStorage.InsertWithVersion(ctx, lockId, version, lockInformationJsonString)
}

func (x *MariaDbStorage) DeleteWithVersion(ctx context.Context, lockId string, exceptedVersion Version) error {
	return x.MySQLStorage.DeleteWithVersion(ctx, lockId, exceptedVersion)
}

func (x *MariaDbStorage) Get(ctx context.Context, lockId string) (string, error) {
	return x.MySQLStorage.Get(ctx, lockId)
}

func (x *MariaDbStorage) GetTime(ctx context.Context) (time.Time, error) {
	return x.MySQLStorage.GetTime(ctx)
}

func (x *MariaDbStorage) Close(ctx context.Context) error {
	return x.MySQLStorage.Close(ctx)
}
