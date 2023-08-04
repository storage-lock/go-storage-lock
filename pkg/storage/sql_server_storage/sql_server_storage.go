package sql_server_storage

import (
	"context"
	"errors"
	"fmt"
	"github.com/golang-infrastructure/go-iterator"
	"github.com/storage-lock/go-storage-lock/pkg/storage"
	"github.com/storage-lock/go-storage-lock/pkg/storage/storage_base"
	"github.com/storage-lock/go-storage-lock/pkg/storage_lock"
	"time"

	_ "github.com/denisenkom/go-mssqldb"
)

type SqlServerStorage struct {
	options       *SqlServerStorageOptions
	tableFullName string
}

var _ storage.Storage = &SqlServerStorage{}

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

const StorageName = "sql-server-storage"

func (x *SqlServerStorage) GetName() string {
	return StorageName
}

func (x *SqlServerStorage) Init(ctx context.Context) error {
	db, err := x.options.ConnectionManager.Take(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = x.options.ConnectionManager.Return(ctx, db)
	}()

	// 创建存储锁信息需要的表
	tableFullName := x.options.TableName
	if tableFullName == "" {
		tableFullName = storage.DefaultStorageTableName
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

	return nil
}

func (x *SqlServerStorage) UpdateWithVersion(ctx context.Context, lockId string, exceptedVersion, newVersion storage.Version, lockInformation *storage.LockInformation) error {

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

func (x *SqlServerStorage) InsertWithVersion(ctx context.Context, lockId string, version storage.Version, lockInformation *storage.LockInformation) error {

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

func (x *SqlServerStorage) DeleteWithVersion(ctx context.Context, lockId string, exceptedVersion storage.Version, lockInformation *storage.LockInformation) error {

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

func (x *SqlServerStorage) Get(ctx context.Context, lockId string) (string, error) {

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

func (x *SqlServerStorage) GetTime(ctx context.Context) (time.Time, error) {

	db, err := x.options.ConnectionManager.Take(ctx)
	if err != nil {
		return time.Time{}, err
	}
	defer func() {
		_ = x.options.ConnectionManager.Return(ctx, db)
	}()

	var zero time.Time
	rs, err := db.Query("SELECT CURRENT_TIMESTAMP")
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
	return nil
}

func (x *SqlServerStorage) List(ctx context.Context) (iterator.Iterator[*storage.LockInformation], error) {

	db, err := x.options.ConnectionManager.Take(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = x.options.ConnectionManager.Return(ctx, db)
	}()

	rows, err := db.Query(fmt.Sprintf("SELECT * FROM %s", x.tableFullName))
	if err != nil {
		return nil, err
	}
	return storage_base.NewSqlRowsIterator(rows), nil
}
