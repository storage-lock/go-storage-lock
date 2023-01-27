package storage_lock

import (
	"context"
)

// ------------------------------------------------ ---------------------------------------------------------------------

type RedisStorageOptions struct {

	// 连接DSN

	// pool

}

// ------------------------------------------------ ---------------------------------------------------------------------

type RedisStorage struct {
}

var _ Storage = &RedisStorage{}

func NewRedisStorage(options *RedisStorageOptions) {

}

func (x *RedisStorage) Init() error {
	//TODO implement me
	panic("implement me")
}

func (x *RedisStorage) UpdateWithVersion(ctx context.Context, lockId string, exceptedVersion, newVersion Version, lockInformationJsonString string) error {
	//TODO implement me
	panic("implement me")
}

func (x *RedisStorage) InsertWithVersion(ctx context.Context, lockId string, version Version, lockInformationJsonString string) error {
	//TODO implement me
	panic("implement me")
}

func (x *RedisStorage) DeleteWithVersion(ctx context.Context, lockId string, exceptedVersion Version) error {
	//TODO implement me
	panic("implement me")
}

func (x *RedisStorage) Get(ctx context.Context, lockId string) (string, error) {
	//TODO implement me
	panic("implement me")
}

// ------------------------------------------------ ---------------------------------------------------------------------
