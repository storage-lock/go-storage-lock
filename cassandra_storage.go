package storage_lock

import (
	"context"
	"time"
)

// 	Cassandra

type CassandraStorage struct {
}

var _ Storage = &CassandraStorage{}

func (x *CassandraStorage) Init(ctx context.Context) error {
	//TODO implement me
	panic("implement me")
}

func (x *CassandraStorage) UpdateWithVersion(ctx context.Context, lockId string, exceptedVersion, newVersion Version, lockInformation *LockInformation, lockInformationJsonString string) error {
	//TODO implement me
	panic("implement me")
}

func (x *CassandraStorage) InsertWithVersion(ctx context.Context, lockId string, version Version, lockInformation *LockInformation, lockInformationJsonString string) error {
	//TODO implement me
	panic("implement me")
}

func (x *CassandraStorage) DeleteWithVersion(ctx context.Context, lockId string, exceptedVersion Version, lockInformation *LockInformation) error {
	//TODO implement me
	panic("implement me")
}

func (x *CassandraStorage) Get(ctx context.Context, lockId string) (string, error) {
	//TODO implement me
	panic("implement me")
}

func (x *CassandraStorage) GetTime(ctx context.Context) (time.Time, error) {
	//TODO implement me
	panic("implement me")
}

func (x *CassandraStorage) Close(ctx context.Context) error {
	//TODO implement me
	panic("implement me")
}
