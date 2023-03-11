package storage_lock

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

const (
	testStorageLockId  = "storage_lock_lock_id_test"
	testStorageVersion = 1
)

// 创建一个单元测试中使用的锁的信息
func getTestLockInformationJsonString(t *testing.T, version ...Version) string {
	if len(version) == 0 {
		version = append(version, testStorageVersion)
	}
	information := LockInformation{
		OwnerId:         "test-case",
		Version:         version[0],
		LockCount:       1,
		LockBeginTime:   time.Now(),
		LeaseExpireTime: time.Now().Add(time.Second * 30),
	}
	marshal, err := json.Marshal(information)
	assert.Nil(t, err)
	return string(marshal)
}

// 确保给定的锁在数据库中不存在，如果存在的话则将其删除
func testEnsureLockNotExists(t *testing.T, storage Storage, lockId string) {
	lockInformationJsonString, err := storage.Get(context.Background(), lockId)
	if errors.Is(err, ErrLockNotFound) {
		return
	} else {
		assert.Nil(t, err)
	}

	information := &LockInformation{}
	err = json.Unmarshal([]byte(lockInformationJsonString), &information)
	assert.Nil(t, err)
	err = storage.DeleteWithVersion(context.Background(), lockId, information.Version)
	assert.Nil(t, err)
}
