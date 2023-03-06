package storage_lock

import (
	"context"
	"time"
)

// TidbStorage 把锁存储在Tidb数据库中
type TidbStorage struct {
}

var _ Storage = &TidbStorage{}

func (x *TidbStorage) Init(ctx context.Context) error {
	//TODO implement me
	panic("implement me")
}

func (x *TidbStorage) UpdateWithVersion(ctx context.Context, lockId string, exceptedVersion, newVersion Version, lockInformationJsonString string) error {
	//TODO implement me
	panic("implement me")
}

func (x *TidbStorage) InsertWithVersion(ctx context.Context, lockId string, version Version, lockInformationJsonString string) error {
	//TODO implement me
	panic("implement me")
}

func (x *TidbStorage) DeleteWithVersion(ctx context.Context, lockId string, exceptedVersion Version) error {
	//TODO implement me
	panic("implement me")
}

func (x *TidbStorage) Get(ctx context.Context, lockId string) (string, error) {
	//TODO implement me
	panic("implement me")
}

func (x *TidbStorage) GetTime(ctx context.Context) (time.Time, error) {
	//TODO implement me
	panic("implement me")
}

func (x *TidbStorage) Close(ctx context.Context) error {
	//TODO implement me
	panic("implement me")
}
