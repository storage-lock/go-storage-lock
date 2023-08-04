package mysql_storage

import (
	"context"
	"errors"
	"fmt"
	"github.com/golang-infrastructure/go-iterator"
	"github.com/storage-lock/go-storage-lock/pkg/storage"
	"github.com/storage-lock/go-storage-lock/pkg/storage/storage_base"
	"github.com/storage-lock/go-storage-lock/pkg/storage_lock"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

type MySQLStorage struct {
	options       *MySQLStorageOptions
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

const StorageName = "mysql-storage"

func (x *MySQLStorage) GetName() string {
	return StorageName
}

func (x *MySQLStorage) Init(ctx context.Context) (err error) {
	db, err := x.options.ConnectionManager.Take(ctx)
	if err != nil {
		return err
	}
	defer func() {
		thisError := x.options.ConnectionManager.Return(ctx, db)
		if err == nil {
			err = thisError
		}
	}()

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

	return nil
}

func (x *MySQLStorage) UpdateWithVersion(ctx context.Context, lockId string, exceptedVersion, newVersion storage.Version, lockInformation *storage.LockInformation) error {

	db, err := x.options.ConnectionManager.Take(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = x.options.ConnectionManager.Return(ctx, db)
	}()

	insertSql := fmt.Sprintf(`UPDATE %s SET version = ?, lock_information_json_string = ? WHERE lock_id = ? AND owner_id = ? AND version = ?`, x.tableFullName)
	execContext, err := db.ExecContext(ctx, insertSql, newVersion, lockInformation.ToJsonString(), lockId, lockInformation.OwnerId, exceptedVersion)
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

	db, err := x.options.ConnectionManager.Take(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = x.options.ConnectionManager.Return(ctx, db)
	}()

	insertSql := fmt.Sprintf(`INSERT INTO %s (lock_id, owner_id, version, lock_information_json_string) VALUES (?, ?, ?, ?)`, x.tableFullName)
	execContext, err := db.ExecContext(ctx, insertSql, lockId, lockInformation.OwnerId, version, lockInformation.ToJsonString())
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

	db, err := x.options.ConnectionManager.Take(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = x.options.ConnectionManager.Return(ctx, db)
	}()

	deleteSql := fmt.Sprintf(`DELETE FROM %s WHERE lock_id = ? AND owner_id = ? AND version = ?`, x.tableFullName)
	execContext, err := db.ExecContext(ctx, deleteSql, lockId, lockInformation.OwnerId, exceptedVersion)
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

	db, err := x.options.ConnectionManager.Take(ctx)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = x.options.ConnectionManager.Return(ctx, db)
	}()

	getLockSql := fmt.Sprintf("SELECT lock_information_json_string FROM %s WHERE lock_id = ?", x.tableFullName)
	rs, err := db.Query(getLockSql, lockId)
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

	db, err := x.options.ConnectionManager.Take(ctx)
	if err != nil {
		return time.Time{}, err
	}
	defer func() {
		_ = x.options.ConnectionManager.Return(ctx, db)
	}()

	var zero time.Time
	rs, err := db.Query("SELECT UNIX_TIMESTAMP(NOW())")
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
	return nil
}

func (x *MySQLStorage) List(ctx context.Context) (iterator.Iterator[*storage.LockInformation], error) {

	db, err := x.options.ConnectionManager.Take(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = x.options.ConnectionManager.Return(ctx, db)
	}()

	rows, err := db.Query("SELECT * FROM %s", x.tableFullName)
	if err != nil {
		return nil, err
	}
	return storage_base.NewSqlRowsIterator(rows), nil
}
