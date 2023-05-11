package maria_db_storage

import (
	"context"
	"database/sql"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// ------------------------------------------------- --------------------------------------------------------------------

// NewMariaDBStorageLock 高层API，使用默认配置快速创建基于MariaDB的分布式锁
func NewMariaDBStorageLock(ctx context.Context, lockId string, dsn string) (*StorageLock, error) {
	connectionGetter := NewMariaStorageConnectionGetterFromDSN(dsn)
	storageOptions := &MariaStorageOptions{
		ConnectionGetter: connectionGetter,
		TableName:        DefaultStorageTableName,
	}

	storage, err := NewMariaDbStorage(ctx, storageOptions)
	if err != nil {
		return nil, err
	}

	lockOptions := &StorageLockOptions{
		LockId:                lockId,
		LeaseExpireAfter:      DefaultLeaseExpireAfter,
		LeaseRefreshInterval:  DefaultLeaseRefreshInterval,
		VersionMissRetryTimes: DefaultVersionMissRetryTimes,
	}
	return NewStorageLock(storage, lockOptions), nil
}

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
func NewMariaStorageConnectionGetter(host string, port uint, user, passwd, database string) *MariaStorageConnectionGetter {
	return &MariaStorageConnectionGetter{
		MySQLStorageConnectionGetter: NewMySQLStorageConnectionGetter(host, port, user, passwd, database),
	}
}

// Get 获取到数据库的连接
func (x *MariaStorageConnectionGetter) Get(ctx context.Context) (*sql.DB, error) {
	return x.MySQLStorageConnectionGetter.Get(ctx)
}

// ------------------------------------------------- --------------------------------------------------------------------

const DefaultMariaStorageTableName = "storage_lock"

type MariaStorageOptions struct {

	// 存放锁的表的名字
	TableName string

	// 用于获取数据库连接
	ConnectionGetter ConnectionGetter[*sql.DB]
}

func (x *MariaStorageOptions) ToMySQLStorageOptions() *MySQLStorageOptions {
	return &MySQLStorageOptions{
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

func NewMariaDbStorage(ctx context.Context, options *MariaStorageOptions) (*MariaDbStorage, error) {

	mysqlStorage, err := NewMySQLStorage(ctx, options.ToMySQLStorageOptions())
	if err != nil {
		return nil, err
	}

	storage := &MariaDbStorage{
		options:      options,
		MySQLStorage: mysqlStorage,
	}

	err = storage.Init(ctx)
	if err != nil {
		return nil, err
	}

	return storage, nil
}

func (x *MariaDbStorage) Init(ctx context.Context) error {
	return x.MySQLStorage.Init(ctx)
}

func (x *MariaDbStorage) UpdateWithVersion(ctx context.Context, lockId string, exceptedVersion, newVersion Version, lockInformation *LockInformation) error {
	return x.MySQLStorage.UpdateWithVersion(ctx, lockId, exceptedVersion, newVersion, lockInformation)
}

func (x *MariaDbStorage) InsertWithVersion(ctx context.Context, lockId string, version Version, lockInformation *LockInformation) error {
	return x.MySQLStorage.InsertWithVersion(ctx, lockId, version, lockInformation)
}

func (x *MariaDbStorage) DeleteWithVersion(ctx context.Context, lockId string, exceptedVersion Version, lockInformation *LockInformation) error {
	return x.MySQLStorage.DeleteWithVersion(ctx, lockId, exceptedVersion, lockInformation)
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
