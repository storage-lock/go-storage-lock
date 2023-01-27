package storage_lock

import "context"

type MongoStorage struct{}

var _ Storage = &MongoStorage{}

func (x *MongoStorage) Init() error {
	//TODO implement me
	panic("implement me")
}

func (x *MongoStorage) UpdateWithVersion(ctx context.Context, lockId string, exceptedVersion, newVersion Version, lockInformationJsonString string) error {
	//TODO implement me
	panic("implement me")
}

func (x *MongoStorage) InsertWithVersion(ctx context.Context, lockId string, version Version, lockInformationJsonString string) error {
	//TODO implement me
	panic("implement me")
}

func (x *MongoStorage) DeleteWithVersion(ctx context.Context, lockId string, exceptedVersion Version) error {
	//TODO implement me
	panic("implement me")
}

func (x *MongoStorage) Get(ctx context.Context, lockId string) (string, error) {
	//TODO implement me
	panic("implement me")
}
