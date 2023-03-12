package storage_lock

import (
	"context"
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
)

func TestNewMariaDbStorage(t *testing.T) {
	storage := getTestMariaDbStorage(t)
	assert.NotNil(t, storage)
}

func TestNewMariaDbStorageConnectionGetter(t *testing.T) {
}

func TestNewMariaDbStorageConnectionGetterFromDSN(t *testing.T) {
}

func TestMariaDbStorageConnectionGetter_Get(t *testing.T) {
}

func TestMariaDbStorage_Close(t *testing.T) {
	storage := getTestMariaDbStorage(t)
	assert.NotNil(t, storage)

	err := storage.Init(context.Background())
	assert.Nil(t, err)

	err = storage.Close(context.Background())
	assert.Nil(t, err)
}

func TestMariaDbStorage_DeleteWithVersion(t *testing.T) {
	storage := getTestMariaDbStorage(t)
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

func TestMariaDbStorage_Get(t *testing.T) {

	storage := getTestMariaDbStorage(t)
	assert.NotNil(t, storage)

	err := storage.Init(context.Background())
	assert.Nil(t, err)

	testEnsureLockNotExists(t, storage, testStorageLockId)

	lockInformationJsonString := getTestLockInformation(t)
	err = storage.InsertWithVersion(context.Background(), testStorageLockId, testStorageVersion, lockInformationJsonString)
	assert.Nil(t, err)

	lockInformationJsonStringRs, err := storage.Get(context.Background(), testStorageLockId)
	assert.Nil(t, err)
	assert.Equal(t, lockInformationJsonString, lockInformationJsonStringRs)

}

func TestMariaDbStorage_GetTime(t *testing.T) {

	storage := getTestMariaDbStorage(t)
	assert.NotNil(t, storage)

	err := storage.Init(context.Background())
	assert.Nil(t, err)

	time, err := storage.GetTime(context.Background())
	assert.Nil(t, err)
	assert.False(t, time.IsZero())

}

func TestMariaDbStorage_Init(t *testing.T) {
	storage := getTestMariaDbStorage(t)
	assert.NotNil(t, storage)

	err := storage.Init(context.Background())
	assert.Nil(t, err)
}

func TestMariaDbStorage_InsertWithVersion(t *testing.T) {
	storage := getTestMariaDbStorage(t)
	assert.NotNil(t, storage)

	err := storage.Init(context.Background())
	assert.Nil(t, err)

	testEnsureLockNotExists(t, storage, testStorageLockId)

	err = storage.InsertWithVersion(context.Background(), testStorageLockId, testStorageVersion, getTestLockInformation(t))
	assert.Nil(t, err)
}

func TestMariaDbStorage_UpdateWithVersion(t *testing.T) {
	storage := getTestMariaDbStorage(t)
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

// 创建测试用的MariaDbStorage
func getTestMariaDbStorage(t *testing.T) *MariaDbStorage {
	envName := "STORAGE_LOCK_MARIA_DSN"
	dsn := os.Getenv(envName)
	assert.NotEmpty(t, dsn)
	connectionGetter := NewMariaStorageConnectionGetterFromDSN(dsn)
	storage := NewMariaDbStorage(&MariaStorageOptions{
		ConnectionGetter: connectionGetter,
		DatabaseName:     "storage_lock_test",
		TableName:        "storage_lock_test",
	})
	return storage
}
