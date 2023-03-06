package storage_lock

import (
	"context"
	"time"
)

type SqlServerStorage struct{}

var _ Storage = &SqlServerStorage{}

func (x *SqlServerStorage) Init(ctx context.Context) error {
	//TODO implement me
	panic("implement me")
}

func (x *SqlServerStorage) UpdateWithVersion(ctx context.Context, lockId string, exceptedVersion, newVersion Version, lockInformationJsonString string) error {
	//TODO implement me
	panic("implement me")
}

func (x *SqlServerStorage) InsertWithVersion(ctx context.Context, lockId string, version Version, lockInformationJsonString string) error {
	//TODO implement me
	panic("implement me")
}

func (x *SqlServerStorage) DeleteWithVersion(ctx context.Context, lockId string, exceptedVersion Version) error {
	//TODO implement me
	panic("implement me")
}

func (x *SqlServerStorage) Get(ctx context.Context, lockId string) (string, error) {
	//TODO implement me
	panic("implement me")
}

func (x *SqlServerStorage) GetTime(ctx context.Context) (time.Time, error) {
	//TODO implement me
	panic("implement me")
}

func (x *SqlServerStorage) Close(ctx context.Context) error {
	//TODO implement me
	panic("implement me")
}
