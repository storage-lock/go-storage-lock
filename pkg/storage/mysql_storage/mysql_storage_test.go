package mysql_storage

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/golang-infrastructure/go-storage-lock/storage"
	storage_pkg "github.com/golang-infrastructure/go-storage-lock/storage"
	"github.com/golang-infrastructure/go-storage-lock/storage_lock"
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
	"time"
)

const (
	testStorageLockId  = "storage_lock_lock_id_test"
	testStorageVersion = 1
)

// 创建一个单元测试中使用的锁的信息
func getTestLockInformation(t *testing.T, version ...storage.Version) *storage.LockInformation {
	if len(version) == 0 {
		version = append(version, testStorageVersion)
	}
	information := &storage.LockInformation{
		OwnerId:         "test-case",
		Version:         version[0],
		LockCount:       1,
		LockBeginTime:   time.Now(),
		LeaseExpireTime: time.Now().Add(time.Second * 30),
	}
	return information
}

// 确保给定的锁在数据库中不存在，如果存在的话则将其删除
func testEnsureLockNotExists(t *testing.T, storage storage.Storage, lockId string) {
	lockInformationJsonString, err := storage.Get(context.Background(), lockId)
	if errors.Is(err, storage_lock.ErrLockNotFound) {
		return
	} else {
		assert.Nil(t, err)
	}

	information := &storage_pkg.LockInformation{}
	err = json.Unmarshal([]byte(lockInformationJsonString), &information)
	assert.Nil(t, err)
	err = storage.DeleteWithVersion(context.Background(), lockId, information.Version, information)
	assert.Nil(t, err)
}
func TestNewMySQLStorage(t *testing.T) {
	storage := getTestMySQLStorage(t)
	assert.NotNil(t, storage)
}

func TestNewMySQLStorageConnectionGetter(t *testing.T) {
}

func TestNewMySQLStorageConnectionGetterFromDSN(t *testing.T) {
}

func TestMySQLStorageConnectionGetter_Get(t *testing.T) {
}

func TestMySQLStorage_Close(t *testing.T) {
	storage := getTestMySQLStorage(t)
	assert.NotNil(t, storage)

	err := storage.Init(context.Background())
	assert.Nil(t, err)

	err = storage.Close(context.Background())
	assert.Nil(t, err)
}

func TestMySQLStorage_DeleteWithVersion(t *testing.T) {
	storage := getTestMySQLStorage(t)
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
	assert.ErrorIs(t, err, storage_lock.ErrLockNotFound)
	assert.Empty(t, lockInformationJsonString)

}

func TestMySQLStorage_Get(t *testing.T) {

	storage := getTestMySQLStorage(t)
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

func TestMySQLStorage_GetTime(t *testing.T) {

	storage := getTestMySQLStorage(t)
	assert.NotNil(t, storage)

	err := storage.Init(context.Background())
	assert.Nil(t, err)

	time, err := storage.GetTime(context.Background())
	assert.Nil(t, err)
	assert.False(t, time.IsZero())

}

func TestMySQLStorage_Init(t *testing.T) {
	storage := getTestMySQLStorage(t)
	assert.NotNil(t, storage)

	err := storage.Init(context.Background())
	assert.Nil(t, err)
}

func TestMySQLStorage_InsertWithVersion(t *testing.T) {
	storage := getTestMySQLStorage(t)
	assert.NotNil(t, storage)

	err := storage.Init(context.Background())
	assert.Nil(t, err)

	testEnsureLockNotExists(t, storage, testStorageLockId)

	err = storage.InsertWithVersion(context.Background(), testStorageLockId, testStorageVersion, getTestLockInformation(t))
	assert.Nil(t, err)
}

func TestMySQLStorage_UpdateWithVersion(t *testing.T) {
	storage := getTestMySQLStorage(t)
	assert.NotNil(t, storage)

	err := storage.Init(context.Background())
	assert.Nil(t, err)

	testEnsureLockNotExists(t, storage, testStorageLockId)

	err = storage.InsertWithVersion(context.Background(), testStorageLockId, testStorageVersion, getTestLockInformation(t))
	assert.Nil(t, err)

	newVersion := storage_pkg.Version(testStorageVersion + 1)
	err = storage.UpdateWithVersion(context.Background(), testStorageLockId, testStorageVersion, newVersion, getTestLockInformation(t, newVersion))
	assert.Nil(t, err)

}

// 创建测试用的MySQLStorage
func getTestMySQLStorage(t *testing.T) *MySQLStorage {
	envName := "STORAGE_LOCK_MYSQL_DSN"
	dsn := os.Getenv(envName)
	assert.NotEmpty(t, dsn)
	connectionGetter := NewMySQLStorageConnectionGetterFromDSN(dsn)
	storage, err := NewMySQLStorage(context.Background(), &MySQLStorageOptions{
		ConnectionGetter: connectionGetter,
		TableName:        "storage_lock_test",
	})
	assert.Nil(t, err)
	return storage
}
