package storage_lock

import (
	"context"
	"time"
)

type MariaDbStorage struct{}

var _ Storage = &MariaDbStorage{}

func (x *MariaDbStorage) Init(ctx context.Context) error {
	//TODO implement me
	panic("implement me")
}

func (x *MariaDbStorage) UpdateWithVersion(ctx context.Context, lockId string, exceptedVersion, newVersion Version, lockInformationJsonString string) error {
	//TODO implement me
	panic("implement me")
}

func (x *MariaDbStorage) InsertWithVersion(ctx context.Context, lockId string, version Version, lockInformationJsonString string) error {
	//TODO implement me
	panic("implement me")
}

func (x *MariaDbStorage) DeleteWithVersion(ctx context.Context, lockId string, exceptedVersion Version) error {
	//TODO implement me
	panic("implement me")
}

func (x *MariaDbStorage) Get(ctx context.Context, lockId string) (string, error) {
	//TODO implement me
	panic("implement me")
}

func (x *MariaDbStorage) GetTime(ctx context.Context) (time.Time, error) {
	//TODO implement me
	panic("implement me")
}

func (x *MariaDbStorage) Close(ctx context.Context) error {
	//TODO implement me
	panic("implement me")
}
