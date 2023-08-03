package postgresql_storage

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

	_ "github.com/lib/pq"
)

// PostgreSQLStorage 基于Postgresql作为存储引擎
type PostgreSQLStorage struct {
	options *PostgreSQLStorageOptions

	db            *sql.DB
	tableFullName string
}

var _ storage.Storage = &PostgreSQLStorage{}

func NewPostgreSQLStorage(ctx context.Context, options *PostgreSQLStorageOptions) (*PostgreSQLStorage, error) {
	s := &PostgreSQLStorage{
		options: options,
	}

	err := s.Init(ctx)
	if err != nil {
		return nil, err
	}

	return s, nil
}

func (x *PostgreSQLStorage) GetName() string {
	return "postgresql-storage"
}

func (x *PostgreSQLStorage) Init(ctx context.Context) error {
	db, err := x.options.ConnectionProvider.Get(ctx)
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

func (x *PostgreSQLStorage) UpdateWithVersion(ctx context.Context, lockId string, exceptedVersion, newVersion storage.Version, lockInformation *storage.LockInformation) error {
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
		return storage_lock.ErrVersionMiss
	}
	return nil
}

func (x *PostgreSQLStorage) InsertWithVersion(ctx context.Context, lockId string, version storage.Version, lockInformation *storage.LockInformation) error {
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
		return storage_lock.ErrVersionMiss
	}
	return nil
}

func (x *PostgreSQLStorage) DeleteWithVersion(ctx context.Context, lockId string, exceptedVersion storage.Version, lockInformation *storage.LockInformation) error {
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
		return storage_lock.ErrVersionMiss
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
		return "", storage_lock.ErrLockNotFound
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
		return zero, errors.New("query postgresql server time failed")
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

func (x *PostgreSQLStorage) List(ctx context.Context) (iterator.Iterator[*storage.LockInformation], error) {
	rows, err := x.db.Query("SELECT * FROM %s", x.tableFullName)
	if err != nil {
		return nil, err
	}
	return storage_base.NewRowsIterator(rows), nil
}
