package storage_lock

import (
	"context"
	"time"
)

// ------------------------------------------------ ---------------------------------------------------------------------

type SqliteStorageOptions struct {
}

// ------------------------------------------------ ---------------------------------------------------------------------

type SqliteStorage struct{}

var _ Storage = &SqliteStorage{}

func (x *SqliteStorage) Init() error {
	//TODO implement me
	panic("implement me")
}

func (x *SqliteStorage) UpdateWithVersion(ctx context.Context, lockId string, exceptedVersion, newVersion Version, lockInformationJsonString string) error {
	//TODO implement me
	panic("implement me")
}

func (x *SqliteStorage) InsertWithVersion(ctx context.Context, lockId string, version Version, lockInformationJsonString string) error {
	//TODO implement me
	panic("implement me")
}

func (x *SqliteStorage) DeleteWithVersion(ctx context.Context, lockId string, exceptedVersion Version) error {
	//TODO implement me
	panic("implement me")
}

func (x *SqliteStorage) Get(ctx context.Context, lockId string) (string, error) {
	//TODO implement me
	panic("implement me")
}

func (x *SqliteStorage) GetTime(ctx context.Context) (time.Time, error) {
	//TODO implement me
	panic("implement me")
}

// ------------------------------------------------ ---------------------------------------------------------------------
