package storage_lock

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	_ "github.com/denisenkom/go-mssqldb"
)

// ------------------------------------------------- --------------------------------------------------------------------

// NewSqlServerStorageLock 高层API，使用默认配置快速创建基于SQLServer的分布式锁
func NewSqlServerStorageLock(ctx context.Context, lockId string, dsn string) (*StorageLock, error) {
	connectionGetter := NewSqlServerStorageConnectionGetterFromDSN(dsn)
	storageOptions := &SqlServerStorageOptions{
		ConnectionGetter: connectionGetter,
		TableName:        DefaultStorageTableName,
	}

	storage, err := NewSqlServerStorage(ctx, storageOptions)
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

// SqlServerStorageConnectionGetter 创建一个SqlServer的连接
type SqlServerStorageConnectionGetter struct {

	// 主机的名字
	Host string

	// 主机的端口
	Port uint

	// 用户名
	User string

	// 密码
	Passwd string

	// DSN
	// Example: "sqlserver://sa:UeGqAm8CxYGldMDLoNNt@192.168.128.206:1433"
	DSN string

	// 初始化好的数据库实例
	db   *sql.DB
	err  error
	once sync.Once
}

var _ ConnectionGetter[*sql.DB] = &SqlServerStorageConnectionGetter{}

// NewSqlServerStorageConnectionGetterFromDSN 从DSN创建SqlServer连接
func NewSqlServerStorageConnectionGetterFromDSN(dsn string) *SqlServerStorageConnectionGetter {
	return &SqlServerStorageConnectionGetter{
		DSN: dsn,
	}
}

// NewSqlServerStorageConnectionGetter 从服务器属性创建数据库连接
func NewSqlServerStorageConnectionGetter(host string, port uint, user, passwd string) *SqlServerStorageConnectionGetter {
	return &SqlServerStorageConnectionGetter{
		Host:   host,
		Port:   port,
		User:   user,
		Passwd: passwd,
	}
}

func (x *SqlServerStorageConnectionGetter) GetDSN() string {
	if x.DSN != "" {
		return x.DSN
	}
	return fmt.Sprintf("sqlserver://%s:%s@%s:%d", x.User, x.Passwd, x.Host, x.Port)
}

// Get 获取到数据库的连接
func (x *SqlServerStorageConnectionGetter) Get(ctx context.Context) (*sql.DB, error) {
	x.once.Do(func() {
		//db, err := sql.Open("sqlserver", x.GetDSN())
		db, err := sql.Open("mssql", x.GetDSN())
		if err != nil {
			x.err = err
			return
		}
		x.db = db
	})
	return x.db, x.err
}

// ------------------------------------------------- --------------------------------------------------------------------

type SqlServerStorageOptions struct {

	// 存放锁的表的名字
	TableName string

	// 用于获取数据库连接
	ConnectionGetter ConnectionGetter[*sql.DB]
}

// ------------------------------------------------- --------------------------------------------------------------------

type SqlServerStorage struct {
	options *SqlServerStorageOptions

	db            *sql.DB
	tableFullName string
}

var _ Storage = &SqlServerStorage{}

func NewSqlServerStorage(ctx context.Context, options *SqlServerStorageOptions) (*SqlServerStorage, error) {

	// 创建存储介质
	storage := &SqlServerStorage{
		options: options,
	}

	// 初始化
	err := storage.Init(ctx)
	if err != nil {
		return nil, err
	}

	return storage, nil
}

func (x *SqlServerStorage) Init(ctx context.Context) error {
	db, err := x.options.ConnectionGetter.Get(ctx)
	if err != nil {
		return err
	}

	// 创建存储锁信息需要的表
	tableFullName := x.options.TableName
	if tableFullName == "" {
		tableFullName = DefaultStorageTableName
	}
	// 这个语法好像执行不过去
	//createTableSql := `IF NOT EXISTS (SELECT * FROM SYSOBJECTS WHERE NAME='%s' AND XTYPE='U')
	//   CREATE TABLE %s (
	//       lock_id VARCHAR(255) NOT NULL PRIMARY KEY,
	//  version BIGINT NOT NULL,
	//  lock_information_json_string VARCHAR(255) NOT NULL
	//   )
	//GO`
	// 这个语法是可以的
	createTableSql := `IF NOT EXISTS (
	SELECT * FROM sys.tables t
	JOIN sys.schemas s ON (t.schema_id = s.schema_id)
	WHERE s.name = 'dbo' AND t.name = '%s')
CREATE TABLE %s (
    lock_id VARCHAR(255) NOT NULL PRIMARY KEY,
    owner_id VARCHAR(255) NOT NULL, 
    version BIGINT NOT NULL,
    lock_information_json_string VARCHAR(255) NOT NULL
	   );`

	_, err = db.ExecContext(ctx, fmt.Sprintf(createTableSql, tableFullName, tableFullName))
	if err != nil {
		return err
	}

	x.tableFullName = tableFullName
	x.db = db

	return nil
}

func (x *SqlServerStorage) UpdateWithVersion(ctx context.Context, lockId string, exceptedVersion, newVersion Version, lockInformation *LockInformation) error {
	insertSql := fmt.Sprintf(`UPDATE %s SET version = ?, lock_information_json_string = ? WHERE lock_id = ? AND owner_id = ? AND version = ?`, x.tableFullName)
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

func (x *SqlServerStorage) InsertWithVersion(ctx context.Context, lockId string, version Version, lockInformation *LockInformation) error {
	insertSql := fmt.Sprintf(`INSERT INTO %s (lock_id, owner_id, version, lock_information_json_string) VALUES (?, ?, ?, ?)`, x.tableFullName)
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

func (x *SqlServerStorage) DeleteWithVersion(ctx context.Context, lockId string, exceptedVersion Version, lockInformation *LockInformation) error {
	deleteSql := fmt.Sprintf(`DELETE FROM %s WHERE lock_id = ? AND owner_id = ? AND version = ?`, x.tableFullName)
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

func (x *SqlServerStorage) Get(ctx context.Context, lockId string) (string, error) {
	getLockSql := fmt.Sprintf("SELECT lock_information_json_string FROM %s WHERE lock_id = ?", x.tableFullName)
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

func (x *SqlServerStorage) GetTime(ctx context.Context) (time.Time, error) {
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

func (x *SqlServerStorage) Close(ctx context.Context) error {
	if x.db == nil {
		return nil
	}
	return x.db.Close()
}

// ------------------------------------------------- --------------------------------------------------------------------
