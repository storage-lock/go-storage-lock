package storage_lock

import (
	"context"
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
)

func TestMongoConfigurationConnectionGetter_Get(t *testing.T) {
	getter := NewMongoConfigurationConnectionGetter(os.Getenv(""))
	storage, err := getter.Get(context.Background())
	assert.Nil(t, err)
	assert.NotNil(t, storage)
}

func TestMongoStorage_Close(t *testing.T) {
	err := getTestMongoStorage(t).Close(context.Background())
	assert.Nil(t, err)
}

func TestMongoStorage_DeleteWithVersion(t *testing.T) {

	storage := getTestMongoStorage(t)

	lockId := ""
	testEnsureLockNotExists(t, storage, lockId)

}

func TestMongoStorage_Get(t *testing.T) {

}

func TestMongoStorage_GetTime(t *testing.T) {

}

func TestMongoStorage_Init(t *testing.T) {

}

func TestMongoStorage_InsertWithVersion(t *testing.T) {
	// TODO
	//lockId := ""
	//var exceptedVersion Version = 1
	////
	//lockInformationJsonString :=
	//
	//storage := getTestMongoStorage(t)
	//
	//// 先插入锁的信息
	//storage.InsertWithVersion(context.Background(), lockId, exceptedVersion, lockInformationJsonString)
	//
	//// 然后再查询锁的信息，看看跟插入的时候能够对得上以检查更新是否成功
	//lockInformationJsonStringResult, err := storage.Get(context.Background(), lockId)
	//assert.Nil(t, err)
	//assert.Equal(t, lockInformationJsonString, lockInformationJsonStringResult)
}

func TestMongoStorage_UpdateWithVersion(t *testing.T) {
	lockId := ""
	var exceptedVersion Version = 1
	var newVersion Version = 2

	lockInformation := getTestLockInformation(t)

	storage := getTestMongoStorage(t)

	// 先保存锁的信息
	err := storage.UpdateWithVersion(context.Background(), lockId, exceptedVersion, newVersion, lockInformation)
	assert.Nil(t, err)

	// 然后再查询锁的信息，看看跟更新的时候能够对得上以检查更新是否成功
	lockInformationJsonStringResult, err := storage.Get(context.Background(), lockId)
	assert.Nil(t, err)
	assert.Equal(t, lockInformation, lockInformationJsonStringResult)
}

func TestNewMongoConfigurationConnectionGetter(t *testing.T) {
	getter := NewMongoConfigurationConnectionGetter("")
	conn, err := getter.Get(context.Background())
	assert.Nil(t, err)
	assert.NotNil(t, conn)
}

func TestNewMongoStorage(t *testing.T) {
	storage := NewMongoStorage(&MongoStorageOptions{
		ConnectionGetter: NewMongoConfigurationConnectionGetter(""),
		CollectionName:   "",
	})
	assert.NotNil(t, storage)
}

func getTestMongoStorage(t *testing.T) *MongoStorage {
	storage := NewMongoStorage(&MongoStorageOptions{
		ConnectionGetter: NewMongoConfigurationConnectionGetter(""),
		CollectionName:   "",
	})
	assert.NotNil(t, storage)
	return storage
}
