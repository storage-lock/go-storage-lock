package mongodb_storage

import (
	"context"
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
)

func TestNewMongoStorage(t *testing.T) {
	storage := getTestMongoStorage(t)
	assert.NotNil(t, storage)
}

func TestNewMongoStorageConnectionGetter(t *testing.T) {
}

func TestNewMongoStorageConnectionGetterFromDSN(t *testing.T) {
}

func TestMongoStorageConnectionGetter_Get(t *testing.T) {
}

func TestMongoStorage_Close(t *testing.T) {
	storage := getTestMongoStorage(t)
	assert.NotNil(t, storage)

	err := storage.Init(context.Background())
	assert.Nil(t, err)

	err = storage.Close(context.Background())
	assert.Nil(t, err)
}

func TestMongoStorage_DeleteWithVersion(t *testing.T) {
	storage := getTestMongoStorage(t)
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

func TestMongoStorage_Get(t *testing.T) {

	storage := getTestMongoStorage(t)
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

func TestMongoStorage_GetTime(t *testing.T) {

	storage := getTestMongoStorage(t)
	assert.NotNil(t, storage)

	err := storage.Init(context.Background())
	assert.Nil(t, err)

	time, err := storage.GetTime(context.Background())
	assert.Nil(t, err)
	assert.False(t, time.IsZero())

}

func TestMongoStorage_Init(t *testing.T) {
	storage := getTestMongoStorage(t)
	assert.NotNil(t, storage)

	err := storage.Init(context.Background())
	assert.Nil(t, err)
}

func TestMongoStorage_InsertWithVersion(t *testing.T) {
	storage := getTestMongoStorage(t)
	assert.NotNil(t, storage)

	err := storage.Init(context.Background())
	assert.Nil(t, err)

	testEnsureLockNotExists(t, storage, testStorageLockId)

	err = storage.InsertWithVersion(context.Background(), testStorageLockId, testStorageVersion, getTestLockInformation(t))
	assert.Nil(t, err)
}

func TestMongoStorage_UpdateWithVersion(t *testing.T) {
	storage := getTestMongoStorage(t)
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

// 创建测试用的MongoStorage
func getTestMongoStorage(t *testing.T) *MongoStorage {
	envName := "STORAGE_LOCK_MONGO_URI"
	uri := os.Getenv(envName)
	assert.NotEmpty(t, uri)
	connectionGetter := NewMongoConfigurationConnectionGetter(uri)
	storage, err := NewMongoStorage(context.Background(), &MongoStorageOptions{
		ConnectionGetter: connectionGetter,
		DatabaseName:     "storage_lock",
		CollectionName:   "storage_lock_test",
	})
	assert.Nil(t, err)
	return storage
}
