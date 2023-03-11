package storage_lock

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// ------------------------------------------------- --------------------------------------------------------------------

// TidbStorageConnectionGetter 创建一个TIDB的连接
type TidbStorageConnectionGetter struct {

	// 主机的名字
	Host string

	// 主机的端口
	Port uint

	// 用户名
	User string

	// 密码
	Passwd string

	// DSN
	// "root:@tcp(127.0.0.1:4000)/test?charset=utf8mb4"
	DSN string

	// 初始化好的数据库实例
	db   *sql.DB
	err  error
	once sync.Once
}

var _ ConnectionGetter[*sql.DB] = &TidbStorageConnectionGetter{}

// NewTidbStorageConnectionGetterFromDSN 从DSN创建TIDB连接
func NewTidbStorageConnectionGetterFromDSN(dsn string) *TidbStorageConnectionGetter {
	return &TidbStorageConnectionGetter{
		DSN: dsn,
	}
}

// NewTidbStorageConnectionGetter 从服务器属性创建数据库连接
func NewTidbStorageConnectionGetter(host string, port uint, user, passwd string) *TidbStorageConnectionGetter {
	return &TidbStorageConnectionGetter{
		Host:   host,
		Port:   port,
		User:   user,
		Passwd: passwd,
	}
}

// Get 获取到数据库的连接
func (x *TidbStorageConnectionGetter) Get(ctx context.Context) (*sql.DB, error) {
	x.once.Do(func() {
		db, err := sql.Open("mysql", x.DSN)
		if err != nil {
			x.err = err
			return
		}
		x.db = db
	})
	return x.db, x.err
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

// ------------------------------------------------- --------------------------------------------------------------------

// TidbStorage 把锁存储在Tidb数据库中
type TidbStorage struct {
	options *TidbStorageOptions

	db            *sql.DB
	tableFullName string
}

var _ Storage = &TidbStorage{}

func NewTidbStorage(options *TidbStorageOptions) *TidbStorage {
	return &TidbStorage{
		options: options,
	}
}

func (x *TidbStorage) Init(ctx context.Context) error {
	db, err := x.options.ConnectionGetter.Get(ctx)
	if err != nil {
		return err
	}

	// 如果设置了数据库的话需要切换数据库
	if x.options.DatabaseName != "" {

		// 如果数据库不存在的话则创建
		createDatabaseSql := fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s", x.options.DatabaseName)
		_, err := db.ExecContext(ctx, createDatabaseSql)
		if err != nil {
			return err
		}

		// 切换到数据库
		_, err = db.ExecContext(ctx, "USE "+x.options.DatabaseName)
		if err != nil {
			return err
		}
	}

	// 创建存储锁信息需要的表
	tableFullName := x.options.TableName
	if tableFullName == "" {
		tableFullName = DefaultTidbStorageTableName
	}
	if x.options.DatabaseName != "" {
		tableFullName = fmt.Sprintf("`%s`.`%s`", x.options.DatabaseName, tableFullName)
	} else {
		tableFullName = fmt.Sprintf("`%s`", tableFullName)
	}
	createTableSql := `CREATE TABLE IF NOT EXISTS %s (
    lock_id VARCHAR(255) NOT NULL PRIMARY KEY,
    version BIGINT NOT NULL,
    lock_information_json_string VARCHAR(255) NOT NULL
)`
	_, err = db.Exec(fmt.Sprintf(createTableSql, tableFullName))
	if err != nil {
		return err
	}

	x.tableFullName = tableFullName
	x.db = db

	return nil
}

func (x *TidbStorage) UpdateWithVersion(ctx context.Context, lockId string, exceptedVersion, newVersion Version, lockInformationJsonString string) error {
	insertSql := fmt.Sprintf(`UPDATE %s SET version = ?, lock_information_json_string = ? WHERE lock_id = ? AND version = ?`, x.tableFullName)
	execContext, err := x.db.ExecContext(ctx, insertSql, newVersion, lockInformationJsonString, lockId, exceptedVersion)
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

func (x *TidbStorage) InsertWithVersion(ctx context.Context, lockId string, version Version, lockInformationJsonString string) error {
	insertSql := fmt.Sprintf(`INSERT INTO %s (lock_id, version, lock_information_json_string) VALUES (?, ?, ?)`, x.tableFullName)
	execContext, err := x.db.ExecContext(ctx, insertSql, lockId, version, lockInformationJsonString)
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

func (x *TidbStorage) DeleteWithVersion(ctx context.Context, lockId string, exceptedVersion Version) error {
	deleteSql := fmt.Sprintf(`DELETE FROM %s WHERE lock_id = ? AND version = ?`, x.tableFullName)
	execContext, err := x.db.ExecContext(ctx, deleteSql, lockId, exceptedVersion)
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

func (x *TidbStorage) Get(ctx context.Context, lockId string) (string, error) {
	getLockSql := fmt.Sprintf("SELECT lock_information_json_string FROM %s WHERE lock_id = ?", x.tableFullName)
	query, err := x.db.Query(getLockSql, lockId)
	if err != nil {
		return "", err
	}
	if !query.Next() {
		return "", ErrLockNotFound
	}
	var lockInformationJsonString string
	err = query.Scan(&lockInformationJsonString)
	if err != nil {
		return "", err
	}
	return lockInformationJsonString, nil
}

func (x *TidbStorage) GetTime(ctx context.Context) (time.Time, error) {
	var zero time.Time
	// TODO 这种方式获取到的时间是不是靠谱
	query, err := x.db.Query("SELECT NOW()")
	if err != nil {
		return zero, err
	}
	if !query.Next() {
		return zero, errors.New("query tidb server time failed")
	}
	var tidbServerTime time.Time
	err = query.Scan(&tidbServerTime)
	if err != nil {
		return zero, err
	}
	return tidbServerTime, nil
}

func (x *TidbStorage) Close(ctx context.Context) error {
	if x.db == nil {
		return nil
	}
	return x.db.Close()
}

// ------------------------------------------------- --------------------------------------------------------------------
