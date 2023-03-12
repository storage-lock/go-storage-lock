package storage_lock

import (
	"context"
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
)

func TestNewSqlServerStorage(t *testing.T) {
	storage := getTestSqlServerStorage(t)
	assert.NotNil(t, storage)
}

func TestNewSqlServerStorageConnectionGetter(t *testing.T) {
}

func TestNewSqlServerStorageConnectionGetterFromDSN(t *testing.T) {
}

func TestSqlServerStorageConnectionGetter_Get(t *testing.T) {
}

func TestSqlServerStorage_Close(t *testing.T) {
	storage := getTestSqlServerStorage(t)
	assert.NotNil(t, storage)

	err := storage.Init(context.Background())
	assert.Nil(t, err)

	err = storage.Close(context.Background())
	assert.Nil(t, err)
}

func TestSqlServerStorage_DeleteWithVersion(t *testing.T) {
	storage := getTestSqlServerStorage(t)
	assert.NotNil(t, storage)

	err := storage.Init(context.Background())
	assert.Nil(t, err)

	testEnsureLockNotExists(t, storage, testStorageLockId)

	// 先插入一条
	err = storage.InsertWithVersion(context.Background(), testStorageLockId, testStorageVersion, getTestLockInformationJsonString(t))
	assert.Nil(t, err)

	// 确认能够查询得到
	lockInformationJsonString, err := storage.Get(context.Background(), testStorageLockId)
	assert.Nil(t, err)
	assert.NotEmpty(t, lockInformationJsonString)

	// 再尝试将这一条删除
	err = storage.DeleteWithVersion(context.Background(), testStorageLockId, testStorageVersion)
	assert.Nil(t, err)

	// 然后再查询，应该就查不到了
	lockInformationJsonString, err = storage.Get(context.Background(), testStorageLockId)
	assert.ErrorIs(t, err, ErrLockNotFound)
	assert.Empty(t, lockInformationJsonString)

}

func TestSqlServerStorage_Get(t *testing.T) {

	storage := getTestSqlServerStorage(t)
	assert.NotNil(t, storage)

	err := storage.Init(context.Background())
	assert.Nil(t, err)

	testEnsureLockNotExists(t, storage, testStorageLockId)

	lockInformationJsonString := getTestLockInformationJsonString(t)
	err = storage.InsertWithVersion(context.Background(), testStorageLockId, testStorageVersion, lockInformationJsonString)
	assert.Nil(t, err)

	lockInformationJsonStringRs, err := storage.Get(context.Background(), testStorageLockId)
	assert.Nil(t, err)
	assert.Equal(t, lockInformationJsonString, lockInformationJsonStringRs)

}

func TestSqlServerStorage_GetTime(t *testing.T) {

	storage := getTestSqlServerStorage(t)
	assert.NotNil(t, storage)

	err := storage.Init(context.Background())
	assert.Nil(t, err)

	time, err := storage.GetTime(context.Background())
	assert.Nil(t, err)
	assert.False(t, time.IsZero())

}

func TestSqlServerStorage_Init(t *testing.T) {
	storage := getTestSqlServerStorage(t)
	assert.NotNil(t, storage)

	err := storage.Init(context.Background())
	assert.Nil(t, err)
}

func TestSqlServerStorage_InsertWithVersion(t *testing.T) {
	storage := getTestSqlServerStorage(t)
	assert.NotNil(t, storage)

	err := storage.Init(context.Background())
	assert.Nil(t, err)

	testEnsureLockNotExists(t, storage, testStorageLockId)

	err = storage.InsertWithVersion(context.Background(), testStorageLockId, testStorageVersion, getTestLockInformationJsonString(t))
	assert.Nil(t, err)
}

func TestSqlServerStorage_UpdateWithVersion(t *testing.T) {
	storage := getTestSqlServerStorage(t)
	assert.NotNil(t, storage)

	err := storage.Init(context.Background())
	assert.Nil(t, err)

	testEnsureLockNotExists(t, storage, testStorageLockId)

	err = storage.InsertWithVersion(context.Background(), testStorageLockId, testStorageVersion, getTestLockInformationJsonString(t))
	assert.Nil(t, err)

	newVersion := Version(testStorageVersion + 1)
	err = storage.UpdateWithVersion(context.Background(), testStorageLockId, testStorageVersion, newVersion, getTestLockInformationJsonString(t, newVersion))
	assert.Nil(t, err)

}

// 创建测试用的SqlServerStorage
func getTestSqlServerStorage(t *testing.T) *SqlServerStorage {
	envName := "STORAGE_LOCK_SQLSERVER_DSN"
	dsn := os.Getenv(envName)
	assert.NotEmpty(t, dsn)
	connectionGetter := NewSqlServerStorageConnectionGetterFromDSN(dsn)
	storage := NewSqlServerStorage(&SqlServerStorageOptions{
		ConnectionGetter: connectionGetter,
		TableName:        "storage_lock_test",
	})
	return storage
}
