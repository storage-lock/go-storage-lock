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
func NewTidbStorageConnectionGetter(host string, port uint, user, passwd, databaseName string) *TidbStorageConnectionGetter {
	return &TidbStorageConnectionGetter{
		MySQLStorageConnectionGetter: NewMySQLStorageConnectionGetter(host, port, user, passwd, databaseName),
	}
}

// Get 获取到数据库的连接
func (x *TidbStorageConnectionGetter) Get(ctx context.Context) (*sql.DB, error) {
	return x.MySQLStorageConnectionGetter.Get(ctx)
}

// ------------------------------------------------- --------------------------------------------------------------------

const DefaultStorageTableName = "storage_lock"

type TidbStorageOptions struct {

	// 存放锁的表的名字
	TableName string

	// 用于获取数据库连接
	ConnectionGetter ConnectionGetter[*sql.DB]
}

func (x *TidbStorageOptions) ToMySQLStorageOptions() *MySQLStorageOptions {
	return &MySQLStorageOptions{
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

func NewTidbStorage(ctx context.Context, options *TidbStorageOptions) (*TidbStorage, error) {

	mysqlStorage, err := NewMySQLStorage(ctx, options.ToMySQLStorageOptions())
	if err != nil {
		return nil, err
	}

	storage := &TidbStorage{
		options:      options,
		MySQLStorage: mysqlStorage,
	}

	err = storage.Init(ctx)
	if err != nil {
		return nil, err
	}

	return storage, nil
}

func (x *TidbStorage) Init(ctx context.Context) error {
	return x.MySQLStorage.Init(ctx)
}

func (x *TidbStorage) UpdateWithVersion(ctx context.Context, lockId string, exceptedVersion, newVersion Version, lockInformation *LockInformation) error {
	return x.MySQLStorage.UpdateWithVersion(ctx, lockId, exceptedVersion, newVersion, lockInformation)
}

func (x *TidbStorage) InsertWithVersion(ctx context.Context, lockId string, version Version, lockInformation *LockInformation) error {
	return x.MySQLStorage.InsertWithVersion(ctx, lockId, version, lockInformation)
}

func (x *TidbStorage) DeleteWithVersion(ctx context.Context, lockId string, exceptedVersion Version, lockInformation *LockInformation) error {
	return x.MySQLStorage.DeleteWithVersion(ctx, lockId, exceptedVersion, lockInformation)
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
