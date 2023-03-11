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

// MySQLStorageConnectionGetter 创建一个MySQL的连接
type MySQLStorageConnectionGetter struct {

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

var _ ConnectionGetter[*sql.DB] = &MySQLStorageConnectionGetter{}

// NewMySQLStorageConnectionGetterFromDSN 从DSN创建MySQL连接
func NewMySQLStorageConnectionGetterFromDSN(dsn string) *MySQLStorageConnectionGetter {
	return &MySQLStorageConnectionGetter{
		DSN: dsn,
	}
}

// NewMySQLStorageConnectionGetter 从服务器属性创建数据库连接
func NewMySQLStorageConnectionGetter(host string, port uint, user, passwd string) *MySQLStorageConnectionGetter {
	return &MySQLStorageConnectionGetter{
		Host:   host,
		Port:   port,
		User:   user,
		Passwd: passwd,
	}
}

// Get 获取到数据库的连接
func (x *MySQLStorageConnectionGetter) Get(ctx context.Context) (*sql.DB, error) {
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

const DefaultMySQLStorageTableName = "storage_lock"

type MySQLStorageOptions struct {

	// 锁存放在哪个数据库下
	DatabaseName string

	// 存放锁的表的名字
	TableName string

	// 用于获取数据库连接
	ConnectionGetter ConnectionGetter[*sql.DB]
}

// ------------------------------------------------- --------------------------------------------------------------------

type MySQLStorage struct {
	options *MySQLStorageOptions

	db            *sql.DB
	tableFullName string
}

var _ Storage = &MySQLStorage{}

func NewMySQLStorage(options *MySQLStorageOptions) *MySQLStorage {
	return &MySQLStorage{
		options: options,
	}
}

func (x *MySQLStorage) Init(ctx context.Context) error {
	db, err := x.options.ConnectionGetter.Get(ctx)
	if err != nil {
		return err
	}

	// 如果设置了数据库的话需要切换数据库
	if x.options.DatabaseName != "" {

		// 如果数据库不存在的话则创建
		createDatabaseSql := fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s`", x.options.DatabaseName)
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

func (x *MySQLStorage) UpdateWithVersion(ctx context.Context, lockId string, exceptedVersion, newVersion Version, lockInformationJsonString string) error {
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

func (x *MySQLStorage) InsertWithVersion(ctx context.Context, lockId string, version Version, lockInformationJsonString string) error {
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

func (x *MySQLStorage) DeleteWithVersion(ctx context.Context, lockId string, exceptedVersion Version) error {
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

func (x *MySQLStorage) Get(ctx context.Context, lockId string) (string, error) {
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

func (x *MySQLStorage) GetTime(ctx context.Context) (time.Time, error) {
	var zero time.Time
	query, err := x.db.Query("SELECT UNIX_TIMESTAMP(NOW())")
	if err != nil {
		return zero, err
	}
	if !query.Next() {
		return zero, errors.New("query tidb server time failed")
	}
	var databaseTimestamp uint64
	err = query.Scan(&databaseTimestamp)
	if err != nil {
		return zero, err
	}

	return time.Unix(int64(databaseTimestamp), 0), nil
}

func (x *MySQLStorage) Close(ctx context.Context) error {
	if x.db == nil {
		return nil
	}
	return x.db.Close()
}
