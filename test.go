package storage_lock

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/storage-lock/go-storage"
	storage_pkg "github.com/storage-lock/go-storage"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

// 这个文件中的方法是用来方便的测试Storage的实现

const (

	// 测试时锁定的资源ID
	testStorageLockId = "storage_lock_lock_id_test"

	// 测试时锁的初始版本号
	testStorageVersion = 1
)

// 创建一个单元测试中使用的锁的信息
// 参数是一个可选的
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
	if errors.Is(err, ErrLockNotFound) {
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
