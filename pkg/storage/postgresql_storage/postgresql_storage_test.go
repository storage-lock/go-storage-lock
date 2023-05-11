package postgresql_storage

import (
	"context"
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
)

func TestNewPostgreSQLStorage(t *testing.T) {
	storage := getTestPostgreSQLStorage(t)
	assert.NotNil(t, storage)
}

func TestNewPostgreSQLStorageConnectionGetter(t *testing.T) {
}

func TestNewPostgreSQLStorageConnectionGetterFromDSN(t *testing.T) {
}

func TestPostgreSQLStorageConnectionGetter_Get(t *testing.T) {
}

func TestPostgreSQLStorage_Close(t *testing.T) {
	storage := getTestPostgreSQLStorage(t)
	assert.NotNil(t, storage)

	err := storage.Init(context.Background())
	assert.Nil(t, err)

	err = storage.Close(context.Background())
	assert.Nil(t, err)
}

func TestPostgreSQLStorage_DeleteWithVersion(t *testing.T) {
	storage := getTestPostgreSQLStorage(t)
	assert.NotNil(t, storage)

	err := storage.Init(context.Background())
	assert.Nil(t, err)

	testEnsureLockNotExists(t, storage, testStorageLockId)

	// 先插入一条
	lockInformation := getTestLockInformation(t)
	err = storage.InsertWithVersion(context.Background(), testStorageLockId, testStorageVersion, lockInformation)
	assert.Nil(t, err)

	// 确认能够查询得到
	lockInformationJsonString, err := storage.Get(context.Background(), testStorageLockId)
	assert.Nil(t, err)
	assert.NotEmpty(t, lockInformationJsonString)

	// 再尝试将这一条删除
	err = storage.DeleteWithVersion(context.Background(), testStorageLockId, testStorageVersion, lockInformation)
	assert.Nil(t, err)

	// 然后再查询，应该就查不到了
	lockInformationJsonString, err = storage.Get(context.Background(), testStorageLockId)
	assert.ErrorIs(t, err, ErrLockNotFound)
	assert.Empty(t, lockInformationJsonString)

}

func TestPostgreSQLStorage_Get(t *testing.T) {

	storage := getTestPostgreSQLStorage(t)
	assert.NotNil(t, storage)

	err := storage.Init(context.Background())
	assert.Nil(t, err)

	testEnsureLockNotExists(t, storage, testStorageLockId)

	lockInformation := getTestLockInformation(t)
	err = storage.InsertWithVersion(context.Background(), testStorageLockId, testStorageVersion, lockInformation)
	assert.Nil(t, err)

	lockInformationJsonStringRs, err := storage.Get(context.Background(), testStorageLockId)
	assert.Nil(t, err)
	assert.Equal(t, lockInformation.ToJsonString(), lockInformationJsonStringRs)

}

func TestPostgreSQLStorage_GetTime(t *testing.T) {

	storage := getTestPostgreSQLStorage(t)
	assert.NotNil(t, storage)

	err := storage.Init(context.Background())
	assert.Nil(t, err)

	time, err := storage.GetTime(context.Background())
	assert.Nil(t, err)
	assert.False(t, time.IsZero())

}

func TestPostgreSQLStorage_Init(t *testing.T) {
	storage := getTestPostgreSQLStorage(t)
	assert.NotNil(t, storage)

	err := storage.Init(context.Background())
	assert.Nil(t, err)
}

func TestPostgreSQLStorage_InsertWithVersion(t *testing.T) {
	storage := getTestPostgreSQLStorage(t)
	assert.NotNil(t, storage)

	err := storage.Init(context.Background())
	assert.Nil(t, err)

	testEnsureLockNotExists(t, storage, testStorageLockId)

	err = storage.InsertWithVersion(context.Background(), testStorageLockId, testStorageVersion, getTestLockInformation(t))
	assert.Nil(t, err)
}

func TestPostgreSQLStorage_UpdateWithVersion(t *testing.T) {
	storage := getTestPostgreSQLStorage(t)
	assert.NotNil(t, storage)

	err := storage.Init(context.Background())
	assert.Nil(t, err)

	testEnsureLockNotExists(t, storage, testStorageLockId)

	err = storage.InsertWithVersion(context.Background(), testStorageLockId, testStorageVersion, getTestLockInformation(t))
	assert.Nil(t, err)

	newVersion := Version(testStorageVersion + 1)
	err = storage.UpdateWithVersion(context.Background(), testStorageLockId, testStorageVersion, newVersion, getTestLockInformation(t, newVersion))
	assert.Nil(t, err)

}

// 创建测试用的PostgreSQLStorage
func getTestPostgreSQLStorage(t *testing.T) *PostgreSQLStorage {
	envName := "STORAGE_LOCK_POSTGRESQL_DSN"
	dsn := os.Getenv(envName)
	assert.NotEmpty(t, dsn)
	connectionGetter := NewPostgreSQLStorageConnectionGetterFromDSN(dsn)
	storage, err := NewPostgreSQLStorage(context.Background(), &PostgreSQLStorageOptions{
		ConnectionGetter: connectionGetter,
		TableName:        "storage_lock_test",
	})
	assert.Nil(t, err)
	return storage
}
