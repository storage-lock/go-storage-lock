package storage_lock

import (
	"context"
	"time"
)

type OracleStorage struct {
}

var _ Storage = &OracleStorage{}

//
//func NewOracleStorage() *OracleStorage {
//	return &OracleStorage{}
//}

func (x *OracleStorage) Init(ctx context.Context) error {
	//TODO implement me
	panic("implement me")
}

func (x *OracleStorage) UpdateWithVersion(ctx context.Context, lockId string, exceptedVersion, newVersion Version, lockInformationJsonString string) error {
	//TODO implement me
	panic("implement me")
}

func (x *OracleStorage) InsertWithVersion(ctx context.Context, lockId string, version Version, lockInformationJsonString string) error {
	//TODO implement me
	panic("implement me")
}

func (x *OracleStorage) DeleteWithVersion(ctx context.Context, lockId string, exceptedVersion Version) error {
	//TODO implement me
	panic("implement me")
}

func (x *OracleStorage) Get(ctx context.Context, lockId string) (string, error) {
	//TODO implement me
	panic("implement me")
}

func (x *OracleStorage) GetTime(ctx context.Context) (time.Time, error) {
	//TODO implement me
	panic("implement me")
}

func (x *OracleStorage) Close(ctx context.Context) error {
	//TODO implement me
	panic("implement me")
}
