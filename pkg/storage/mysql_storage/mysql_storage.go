package mysql_storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/golang-infrastructure/go-iterator"
	"github.com/storage-lock/go-storage-lock/pkg/storage"
	"github.com/storage-lock/go-storage-lock/pkg/storage/base"
	"github.com/storage-lock/go-storage-lock/pkg/storage_lock"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

type MySQLStorage struct {
	options *MySQLStorageOptions

	db            *sql.DB
	tableFullName string
}

var _ storage.Storage = &MySQLStorage{}

func NewMySQLStorage(ctx context.Context, options *MySQLStorageOptions) (*MySQLStorage, error) {
	s := &MySQLStorage{
		options: options,
	}

	err := s.Init(ctx)
	if err != nil {
		return nil, err
	}

	return s, nil
}

func (x *MySQLStorage) GetName() string {
	return "mysql-storage"
}

func (x *MySQLStorage) Init(ctx context.Context) error {
	db, err := x.options.ConnectionProvider.Get(ctx)
	if err != nil {
		return err
	}

	// 创建存储锁信息需要的表
	tableFullName := x.options.TableName
	if tableFullName == "" {
		tableFullName = storage.DefaultStorageTableName
	}
	createTableSql := `CREATE TABLE IF NOT EXISTS %s (
    lock_id VARCHAR(255) NOT NULL PRIMARY KEY,
    owner_id VARCHAR(255) NOT NULL,
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

func (x *MySQLStorage) UpdateWithVersion(ctx context.Context, lockId string, exceptedVersion, newVersion storage.Version, lockInformation *storage.LockInformation) error {
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
		return storage_lock.ErrVersionMiss
	}
	return nil
}

func (x *MySQLStorage) InsertWithVersion(ctx context.Context, lockId string, version storage.Version, lockInformation *storage.LockInformation) error {
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
		return storage_lock.ErrVersionMiss
	}
	return nil
}

func (x *MySQLStorage) DeleteWithVersion(ctx context.Context, lockId string, exceptedVersion storage.Version, lockInformation *storage.LockInformation) error {
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
		return storage_lock.ErrVersionMiss
	}
	return nil
}

func (x *MySQLStorage) Get(ctx context.Context, lockId string) (string, error) {
	getLockSql := fmt.Sprintf("SELECT lock_information_json_string FROM %s WHERE lock_id = ?", x.tableFullName)
	rs, err := x.db.Query(getLockSql, lockId)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = rs.Close()
	}()
	if !rs.Next() {
		return "", storage_lock.ErrLockNotFound
	}
	var lockInformationJsonString string
	err = rs.Scan(&lockInformationJsonString)
	if err != nil {
		return "", err
	}
	return lockInformationJsonString, nil
}

func (x *MySQLStorage) GetTime(ctx context.Context) (time.Time, error) {
	var zero time.Time
	rs, err := x.db.Query("SELECT UNIX_TIMESTAMP(NOW())")
	if err != nil {
		return zero, err
	}
	defer func() {
		_ = rs.Close()
	}()
	if !rs.Next() {
		return zero, errors.New("rs server time failed")
	}
	var databaseTimestamp uint64
	err = rs.Scan(&databaseTimestamp)
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

func (x *MySQLStorage) List(ctx context.Context) (iterator.Iterator[*storage.LockInformation], error) {
	rows, err := x.db.Query("SELECT * FROM %s", x.tableFullName)
	if err != nil {
		return nil, err
	}
	return base.NewRowsIterator(rows), nil
}
