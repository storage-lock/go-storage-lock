package storage_lock

import (
	"context"
	"database/sql"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// ------------------------------------------------- --------------------------------------------------------------------

// TidbStorageConnectionGetter 创建一个TIDB的连接
type TidbStorageConnectionGetter struct {
	*MySQLStorageConnectionGetter
}

var _ ConnectionGetter[*sql.DB] = &TidbStorageConnectionGetter{}

// NewTidbStorageConnectionGetterFromDSN 从DSN创建TIDB连接
func NewTidbStorageConnectionGetterFromDSN(dsn string) *TidbStorageConnectionGetter {
	return &TidbStorageConnectionGetter{
		MySQLStorageConnectionGetter: NewMySQLStorageConnectionGetterFromDSN(dsn),
	}
}

// NewTidbStorageConnectionGetter 从服务器属性创建数据库连接
func NewTidbStorageConnectionGetter(host string, port uint, user, passwd string) *TidbStorageConnectionGetter {
	return &TidbStorageConnectionGetter{
		MySQLStorageConnectionGetter: NewMySQLStorageConnectionGetter(host, port, user, passwd),
	}
}

// Get 获取到数据库的连接
func (x *TidbStorageConnectionGetter) Get(ctx context.Context) (*sql.DB, error) {
	return x.MySQLStorageConnectionGetter.Get(ctx)
}

// ------------------------------------------------- --------------------------------------------------------------------

const DefaultTidbStorageTableName = "storage_lock"

type TidbStorageOptions struct {

	// 锁存放在哪个数据库下
	DatabaseName string

	// 存放锁的表的名字
	TableName string

	// 用于获取数据库连接
	ConnectionGetter ConnectionGetter[*sql.DB]
}

func (x *TidbStorageOptions) ToMySQLStorageOptions() *MySQLStorageOptions {
	return &MySQLStorageOptions{
		DatabaseName:     x.DatabaseName,
		TableName:        x.TableName,
		ConnectionGetter: x.ConnectionGetter,
	}
}

// ------------------------------------------------- --------------------------------------------------------------------

// TidbStorage 把锁存储在Tidb数据库中
type TidbStorage struct {
	*MySQLStorage

	options *TidbStorageOptions
}

var _ Storage = &TidbStorage{}

func NewTidbStorage(options *TidbStorageOptions) *TidbStorage {
	return &TidbStorage{
		options:      options,
		MySQLStorage: NewMySQLStorage(options.ToMySQLStorageOptions()),
	}
}

func (x *TidbStorage) Init(ctx context.Context) error {
	return x.MySQLStorage.Init(ctx)
}

func (x *TidbStorage) UpdateWithVersion(ctx context.Context, lockId string, exceptedVersion, newVersion Version, lockInformationJsonString string) error {
	return x.MySQLStorage.UpdateWithVersion(ctx, lockId, exceptedVersion, newVersion, lockInformationJsonString)
}

func (x *TidbStorage) InsertWithVersion(ctx context.Context, lockId string, version Version, lockInformationJsonString string) error {
	return x.MySQLStorage.InsertWithVersion(ctx, lockId, version, lockInformationJsonString)
}

func (x *TidbStorage) DeleteWithVersion(ctx context.Context, lockId string, exceptedVersion Version) error {
	return x.MySQLStorage.DeleteWithVersion(ctx, lockId, exceptedVersion)
}

func (x *TidbStorage) Get(ctx context.Context, lockId string) (string, error) {
	return x.MySQLStorage.Get(ctx, lockId)
}

func (x *TidbStorage) GetTime(ctx context.Context) (time.Time, error) {
	return x.MySQLStorage.GetTime(ctx)
}

func (x *TidbStorage) Close(ctx context.Context) error {
	return x.MySQLStorage.Close(ctx)
}

// ------------------------------------------------- --------------------------------------------------------------------
