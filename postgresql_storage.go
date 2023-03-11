package storage_lock

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"
)

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

	// DSN
	// "root:@tcp(127.0.0.1:4000)/test?charset=utf8mb4"
	DSN string

	// 初始化好的数据库实例
	db   *sql.DB
	err  error
	once sync.Once
}

// ------------------------------------------------- --------------------------------------------------------------------

type PostgreSQLStorage struct {
	options *MySQLStorageOptions

	db            *sql.DB
	tableFullName string
}

var _ Storage = &PostgreSQLStorage{}

func (x *PostgreSQLStorage) Init(ctx context.Context) error {
	db, err := x.options.ConnectionGetter.Get(ctx)
	if err != nil {
		return err
	}

	// 如果设置了数据库的话需要切换数据库
	if x.options.DatabaseName != "" {
		// 切换到数据库
		_, err = db.ExecContext(ctx, "USE "+x.options.DatabaseName)
		if err != nil {
			return err
		}
	}

	// 创建存储锁信息需要的表
	tableFullName := x.options.TableName
	if tableFullName == "" {
		tableFullName = DefaultStorageTableName
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

func (x *PostgreSQLStorage) UpdateWithVersion(ctx context.Context, lockId string, exceptedVersion, newVersion Version, lockInformationJsonString string) error {
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

func (x *PostgreSQLStorage) InsertWithVersion(ctx context.Context, lockId string, version Version, lockInformationJsonString string) error {
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

func (x *PostgreSQLStorage) DeleteWithVersion(ctx context.Context, lockId string, exceptedVersion Version) error {
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

func (x *PostgreSQLStorage) Get(ctx context.Context, lockId string) (string, error) {
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

func (x *PostgreSQLStorage) GetTime(ctx context.Context) (time.Time, error) {
	var zero time.Time
	query, err := x.db.Query("SELECT UNIX_TIMESTAMP(NOW())")
	if err != nil {
		return zero, err
	}
	if !query.Next() {
		return zero, errors.New("query server time failed")
	}
	var databaseTimestamp uint64
	err = query.Scan(&databaseTimestamp)
	if err != nil {
		return zero, err
	}

	return time.Unix(int64(databaseTimestamp), 0), nil
}

func (x *PostgreSQLStorage) Close(ctx context.Context) error {
	if x.db == nil {
		return nil
	}
	return x.db.Close()
}
