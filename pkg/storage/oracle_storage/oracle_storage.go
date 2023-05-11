package oracle_storage

import (
	"context"
	"time"
)

// TODO Oracle的要实现一下吗

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

func (x *OracleStorage) UpdateWithVersion(ctx context.Context, lockId string, exceptedVersion, newVersion Version, lockInformation *LockInformation) error {
	//TODO implement me
	panic("implement me")
}

func (x *OracleStorage) InsertWithVersion(ctx context.Context, lockId string, version Version, lockInformation *LockInformation) error {
	//TODO implement me
	panic("implement me")
}

func (x *OracleStorage) DeleteWithVersion(ctx context.Context, lockId string, exceptedVersion Version, lockInformation *LockInformation) error {
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
