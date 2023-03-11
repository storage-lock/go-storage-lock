package storage_lock

import (
	"context"
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
)

func TestNewTidbStorage(t *testing.T) {
	storage := getTestTidbStorage(t)
	assert.NotNil(t, storage)
}

func TestNewTidbStorageConnectionGetter(t *testing.T) {
}

func TestNewTidbStorageConnectionGetterFromDSN(t *testing.T) {
}

func TestTidbStorageConnectionGetter_Get(t *testing.T) {
}

func TestTidbStorage_Close(t *testing.T) {
	storage := getTestTidbStorage(t)
	assert.NotNil(t, storage)

	err := storage.Init(context.Background())
	assert.Nil(t, err)

	err = storage.Close(context.Background())
	assert.Nil(t, err)
}

func TestTidbStorage_DeleteWithVersion(t *testing.T) {
	storage := getTestTidbStorage(t)
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

func TestTidbStorage_Get(t *testing.T) {

	storage := getTestTidbStorage(t)
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

func TestTidbStorage_GetTime(t *testing.T) {

}

func TestTidbStorage_Init(t *testing.T) {
	storage := getTestTidbStorage(t)
	assert.NotNil(t, storage)

	err := storage.Init(context.Background())
	assert.Nil(t, err)
}

func TestTidbStorage_InsertWithVersion(t *testing.T) {
	storage := getTestTidbStorage(t)
	assert.NotNil(t, storage)

	err := storage.Init(context.Background())
	assert.Nil(t, err)

	testEnsureLockNotExists(t, storage, testStorageLockId)

	err = storage.InsertWithVersion(context.Background(), testStorageLockId, testStorageVersion, getTestLockInformationJsonString(t))
	assert.Nil(t, err)
}

func TestTidbStorage_UpdateWithVersion(t *testing.T) {
	storage := getTestTidbStorage(t)
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

// 创建测试用的TidbStorage
func getTestTidbStorage(t *testing.T) *TidbStorage {
	envName := "STORAGE_LOCK_TIDB_DSN"
	dsn := os.Getenv(envName)
	assert.NotEmpty(t, dsn)
	connectionGetter := NewTidbStorageConnectionGetterFromDSN(dsn)
	storage := NewTidbStorage(&TidbStorageOptions{
		ConnectionGetter: connectionGetter,
		DatabaseName:     "storage_lock_test",
		TableName:        "storage_lock_test",
	})
	return storage
}
