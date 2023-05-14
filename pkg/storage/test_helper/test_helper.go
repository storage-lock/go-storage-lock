package test_helper

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/storage-lock/go-storage-lock/pkg/storage"
	"github.com/storage-lock/go-storage-lock/pkg/storage_lock"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

// ------------------------------------------------- --------------------------------------------------------------------

// 单元测试统一使用的一些常量
const (
	TestDatabaseName = "storage_lock_test"
	TestTableName    = "storage_lock_test"

	TestLockId      = "lock_id_for_test"
	TestLockVersion = 1

	TestOwnerIdA = "owner_id_A"
	TestOwnerIdB = "owner_id_B"
)

// ------------------------------------------------- --------------------------------------------------------------------

// TestEnsureLockNotExists 确保给定的锁在数据库中不存在，如果存在的话则将其删除
func TestEnsureLockNotExists(t *testing.T, s storage.Storage, lockId ...string) {

	if len(lockId) == 0 {
		lockId = append(lockId, TestLockId)
	}

	lockInformationJsonString, err := s.Get(context.Background(), lockId[0])
	if errors.Is(err, storage_lock.ErrLockNotFound) {
		return
	} else {
		assert.Nil(t, err)
	}

	information := &storage.LockInformation{}
	err = json.Unmarshal([]byte(lockInformationJsonString), &information)
	assert.Nil(t, err)
	err = s.DeleteWithVersion(context.Background(), lockId[0], information.Version, information)
	assert.Nil(t, err)
}

// BuildTestLockInformation 创建一个单元测试中使用的锁的信息
func BuildTestLockInformation(t *testing.T, version ...storage.Version) *storage.LockInformation {
	if len(version) == 0 {
		version = append(version, TestLockVersion)
	}
	information := &storage.LockInformation{
		OwnerId:         TestOwnerIdA,
		Version:         version[0],
		LockCount:       1,
		LockBeginTime:   time.Now(),
		LeaseExpireTime: time.Now().Add(time.Second * 30),
	}
	return information
}

// ------------------------------------------------- --------------------------------------------------------------------

// TestStorage 用于测试Storage的实现是否OK
func TestStorage(t *testing.T, storage storage.Storage) {

	TestStorage_GetName(t, storage)

	TestStorage_Init(t, storage)

	TestStorage_Get(t, storage)

	TestStorage_GetTime(t, storage)

	TestStorage_UpdateWithVersion(t, storage)

	TestStorage_InsertWithVersion(t, storage)

	TestStorage_DeleteWithVersion(t, storage)

	TestStorage_Close(t, storage)
}

func TestStorage_GetName(t *testing.T, storage storage.Storage) {
	assert.NotNilf(t, storage, "storage is nil")
	assert.NotEmptyf(t, storage.GetName(), "storage name is empty")
}

func TestStorage_Init(t *testing.T, storage storage.Storage) {
	assert.NotNilf(t, storage, "storage is nil")
	err := storage.Init(context.Background())
	assert.Nilf(t, err, "storage %s initialization error: %#v", storage.GetName(), err)
}

func TestStorage_UpdateWithVersion(t *testing.T, s storage.Storage) {
	assert.NotNilf(t, s, "storage is nil")

	err := s.Init(context.Background())
	assert.Nilf(t, err, "s %s initialization error: %#v", s.GetName(), err)

	TestEnsureLockNotExists(t, s, TestLockId)

	err = s.InsertWithVersion(context.Background(), TestLockId, TestLockVersion, BuildTestLockInformation(t))
	assert.Nilf(t, err, "s %s insert error: %#v", s.GetName(), err)

	newVersion := storage.Version(TestLockVersion + 1)
	err = s.UpdateWithVersion(context.Background(), TestLockId, TestLockVersion, newVersion, BuildTestLockInformation(t, newVersion))
	assert.Nilf(t, err, "s %s update error: %#v", s.GetName(), err)
}

func TestStorage_InsertWithVersion(t *testing.T, s storage.Storage) {
	assert.NotNilf(t, s, "storage is nil")

	err := s.Init(context.Background())
	assert.Nilf(t, err, "storage %s init error: %#v", s.GetName(), err)

	TestEnsureLockNotExists(t, s, TestLockId)

	err = s.InsertWithVersion(context.Background(), TestLockId, TestLockVersion, BuildTestLockInformation(t))
	assert.Nilf(t, err, "storage %s insert error: %#v", s.GetName(), err)
}

func TestStorage_DeleteWithVersion(t *testing.T, s storage.Storage) {
	assert.NotNilf(t, s, "storage is nil")

	err := s.Init(context.Background())
	assert.Nilf(t, err, "storage %s init error: %#v", s.GetName(), err)

	TestEnsureLockNotExists(t, s)

	// 先插入一条
	lockInformation := BuildTestLockInformation(t)
	err = s.InsertWithVersion(context.Background(), TestLockId, TestLockVersion, lockInformation)
	assert.Nil(t, err)

	// 确认能够查询得到
	lockInformationJsonString, err := s.Get(context.Background(), TestLockId)
	assert.Nil(t, err)
	assert.NotEmpty(t, lockInformationJsonString)

	// 再尝试将这一条删除
	err = s.DeleteWithVersion(context.Background(), TestLockId, TestLockVersion, lockInformation)
	assert.Nil(t, err)

	// 然后再查询，应该就查不到了
	lockInformationJsonString, err = s.Get(context.Background(), TestLockId)
	assert.ErrorIs(t, err, storage_lock.ErrLockNotFound)
	assert.Empty(t, lockInformationJsonString)

}

func TestStorage_Get(t *testing.T, s storage.Storage) {
	assert.NotNilf(t, s, "storage is nil")

	err := s.Init(context.Background())
	assert.Nilf(t, err, "storage %s init error: %#v", s.GetName(), err)

	TestEnsureLockNotExists(t, s, TestLockId)

	lockInformation := BuildTestLockInformation(t)
	err = s.InsertWithVersion(context.Background(), TestLockId, TestLockVersion, lockInformation)
	assert.Nilf(t, err, "storage %s insert error: %#v", s.GetName(), err)

	lockInformationJsonStringRs, err := s.Get(context.Background(), TestLockId)
	assert.Nilf(t, err, "storage %s get lock error: %#v", s.GetName(), err)
	assert.Equalf(t, lockInformation.ToJsonString(), lockInformationJsonStringRs, "storage %s get lock not equals", s.GetName())

}

func TestStorage_GetTime(t *testing.T, s storage.Storage) {
	assert.NotNilf(t, s, "storage is nil")

	err := s.Init(context.Background())
	assert.Nilf(t, err, "storage %s init error: %#v", s.GetName(), err)

	time, err := s.GetTime(context.Background())
	assert.Nilf(t, err, "storage %s GetTime error: %#v", s.GetName(), err)
	assert.Falsef(t, time.IsZero(), "storage %s GetTime return zero time", s.GetName())
}

// TestStorage_Close 用于测试Storage的Close实现是否正确
func TestStorage_Close(t *testing.T, storage storage.Storage) {

	assert.NotNilf(t, storage, "storage is nil")

	err := storage.Init(context.Background())
	assert.Nilf(t, err, "storage %s initialization error: %#v", storage.GetName(), err)

	err = storage.Close(context.Background())
	assert.Nilf(t, err, "storage %s close error: %#v", err)
}

// ------------------------------------------------- --------------------------------------------------------------------
