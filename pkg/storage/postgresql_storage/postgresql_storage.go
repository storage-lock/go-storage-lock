package postgresql_storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	_ "github.com/lib/pq"
)

// ------------------------------------------------- --------------------------------------------------------------------

// NewPostgreSQLStorageLock 高层API，使用默认配置快速创建基于PostgreSQL的分布式锁
func NewPostgreSQLStorageLock(ctx context.Context, lockId string, dsn string, schema ...string) (*StorageLock, error) {
	connectionGetter := NewPostgreSQLStorageConnectionGetterFromDSN(dsn)
	storageOptions := &PostgreSQLStorageOptions{
		ConnectionGetter: connectionGetter,
		TableName:        DefaultStorageTableName,
	}

	if len(schema) != 0 {
		storageOptions.Schema = schema[0]
	}

	storage, err := NewPostgreSQLStorage(ctx, storageOptions)
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

type PostgreSQLStorageConnectionGetter struct {

	// 主机的名字
	Host string

	// 主机的端口
	Port uint

	// 用户名
	User string

	// 密码
	Passwd string

	DatabaseName string

	// DSN
	// Example: "host=192.168.128.206 user=postgres password=123456 port=5432 dbname=postgres sslmode=disable"
	DSN string

	// 初始化好的数据库实例
	db   *sql.DB
	err  error
	once sync.Once
}

var _ ConnectionGetter[*sql.DB] = &PostgreSQLStorageConnectionGetter{}

// NewPostgreSQLStorageConnectionGetterFromDSN 从DSN创建MySQL连接
func NewPostgreSQLStorageConnectionGetterFromDSN(dsn string) *PostgreSQLStorageConnectionGetter {
	return &PostgreSQLStorageConnectionGetter{
		DSN: dsn,
	}
}

// NewPostgreSQLStorageConnectionGetter 从服务器属性创建数据库连接
func NewPostgreSQLStorageConnectionGetter(host string, port uint, user, passwd, databaseName string) *PostgreSQLStorageConnectionGetter {
	return &PostgreSQLStorageConnectionGetter{
		Host:         host,
		Port:         port,
		User:         user,
		Passwd:       passwd,
		DatabaseName: databaseName,
	}
}

// Get 获取到数据库的连接
func (x *PostgreSQLStorageConnectionGetter) Get(ctx context.Context) (*sql.DB, error) {
	x.once.Do(func() {
		db, err := sql.Open("postgres", x.GetDSN())
		if err != nil {
			x.err = err
			return
		}
		x.db = db
	})
	return x.db, x.err
}

func (x *PostgreSQLStorageConnectionGetter) GetDSN() string {
	if x.DSN != "" {
		return x.DSN
	}
	return fmt.Sprintf("host=%s user=%s password=%s port=%d dbname=%s sslmode=disable", x.Host, x.User, x.Passwd, x.Port, x.DatabaseName)
}

// ------------------------------------------------- --------------------------------------------------------------------

const DefaultPostgreSQLStorageSchema = "public"

type PostgreSQLStorageOptions struct {

	// 存在哪个schema下，默认是public
	Schema string

	// 存放锁的表的名字
	TableName string

	// 用于获取数据库连接
	ConnectionGetter ConnectionGetter[*sql.DB]
}

// ------------------------------------------------- --------------------------------------------------------------------

type PostgreSQLStorage struct {
	options *PostgreSQLStorageOptions

	db            *sql.DB
	tableFullName string
}

var _ Storage = &PostgreSQLStorage{}

func NewPostgreSQLStorage(ctx context.Context, options *PostgreSQLStorageOptions) (*PostgreSQLStorage, error) {
	storage := &PostgreSQLStorage{
		options: options,
	}

	err := storage.Init(ctx)
	if err != nil {
		return nil, err
	}

	return storage, nil
}

func (x *PostgreSQLStorage) Init(ctx context.Context) error {
	db, err := x.options.ConnectionGetter.Get(ctx)
	if err != nil {
		return err
	}

	// 如果设置了数据库的话需要切换数据库
	if x.options.Schema != "" {
		// 切换到schema，如果需要的话，但是schema不会自动创建，需要使用者自己创建，会自动创建的只有存放锁信息的表
		_, err = db.ExecContext(ctx, fmt.Sprintf("SET search_path TO %s ", x.options.Schema))
		if err != nil {
			return err
		}
	}

	// 创建存储锁信息需要的表
	tableFullName := x.options.TableName
	if tableFullName == "" {
		tableFullName = DefaultPostgreSQLStorageSchema
	}
	if x.options.Schema != "" {
		tableFullName = fmt.Sprintf("%s.%s", x.options.Schema, tableFullName)
	} else {
		tableFullName = fmt.Sprintf("%s", tableFullName)
	}
	createTableSql := `CREATE TABLE IF NOT EXISTS %s (
    lock_id VARCHAR(255) NOT NULL PRIMARY KEY,
    owner_id VARCHAR(255) NOT NULL, 
    version BIGINT NOT NULL,
    lock_information_json_string VARCHAR(255) NOT NULL
)`
	_, err = db.ExecContext(ctx, fmt.Sprintf(createTableSql, tableFullName))
	if err != nil {
		return err
	}

	x.tableFullName = tableFullName
	x.db = db

	return nil
}

func (x *PostgreSQLStorage) UpdateWithVersion(ctx context.Context, lockId string, exceptedVersion, newVersion Version, lockInformation *LockInformation) error {
	insertSql := fmt.Sprintf(`UPDATE %s SET version = $1, lock_information_json_string = $2 WHERE lock_id = $3 AND owner_id = $4 AND version = $5`, x.tableFullName)
	execContext, err := x.db.ExecContext(ctx, insertSql, newVersion, lockInformation.ToJsonString(), lockId, lockInformation.OwnerId, exceptedVersion)
	if err != nil {
		return err
	}
	affected, err := execContext.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrVersionMiss
	}
	return nil
}

func (x *PostgreSQLStorage) InsertWithVersion(ctx context.Context, lockId string, version Version, lockInformation *LockInformation) error {
	insertSql := fmt.Sprintf(`INSERT INTO %s (lock_id, owner_id, version, lock_information_json_string) VALUES ($1, $2, $3, $4)`, x.tableFullName)
	execContext, err := x.db.ExecContext(ctx, insertSql, lockId, lockInformation.OwnerId, version, lockInformation.ToJsonString())
	if err != nil {
		return err
	}
	affected, err := execContext.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrVersionMiss
	}
	return nil
}

func (x *PostgreSQLStorage) DeleteWithVersion(ctx context.Context, lockId string, exceptedVersion Version, lockInformation *LockInformation) error {
	deleteSql := fmt.Sprintf(`DELETE FROM %s WHERE lock_id = $1 AND owner_id = $2 AND version = $3`, x.tableFullName)
	execContext, err := x.db.ExecContext(ctx, deleteSql, lockId, lockInformation.OwnerId, exceptedVersion)
	if err != nil {
		return err
	}
	affected, err := execContext.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrVersionMiss
	}
	return nil
}

func (x *PostgreSQLStorage) Get(ctx context.Context, lockId string) (string, error) {
	getLockSql := fmt.Sprintf("SELECT lock_information_json_string FROM %s WHERE lock_id = $1", x.tableFullName)
	rs, err := x.db.Query(getLockSql, lockId)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = rs.Close()
	}()
	if !rs.Next() {
		return "", ErrLockNotFound
	}
	var lockInformationJsonString string
	err = rs.Scan(&lockInformationJsonString)
	if err != nil {
		return "", err
	}
	return lockInformationJsonString, nil
}

func (x *PostgreSQLStorage) GetTime(ctx context.Context) (time.Time, error) {
	var zero time.Time
	rs, err := x.db.Query("SELECT CURRENT_TIMESTAMP")
	if err != nil {
		return zero, err
	}
	defer func() {
		_ = rs.Close()
	}()
	if !rs.Next() {
		return zero, errors.New("rs server time failed")
	}
	var databaseTime time.Time
	err = rs.Scan(&databaseTime)
	if err != nil {
		return zero, err
	}

	return databaseTime, nil
}

func (x *PostgreSQLStorage) Close(ctx context.Context) error {
	if x.db == nil {
		return nil
	}
	return x.db.Close()
}
