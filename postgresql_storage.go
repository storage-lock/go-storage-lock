package storage_lock

import (
	"context"
	"time"
)

type PostgreSQLStorage struct {
}

var _ Storage = &PostgreSQLStorage{}

func (x *PostgreSQLStorage) Init(ctx context.Context) error {
	//TODO implement me
	panic("implement me")
}

func (x *PostgreSQLStorage) UpdateWithVersion(ctx context.Context, lockId string, exceptedVersion, newVersion Version, lockInformationJsonString string) error {
	//TODO implement me
	panic("implement me")
}

func (x *PostgreSQLStorage) InsertWithVersion(ctx context.Context, lockId string, version Version, lockInformationJsonString string) error {
	//TODO implement me
	panic("implement me")
}

func (x *PostgreSQLStorage) DeleteWithVersion(ctx context.Context, lockId string, exceptedVersion Version) error {
	//TODO implement me
	panic("implement me")
}

func (x *PostgreSQLStorage) Get(ctx context.Context, lockId string) (string, error) {
	//TODO implement me
	panic("implement me")
}

func (x *PostgreSQLStorage) GetTime(ctx context.Context) (time.Time, error) {
	//TODO implement me
	panic("implement me")
}

func (x *PostgreSQLStorage) Close(ctx context.Context) error {
	//TODO implement me
	panic("implement me")
}
