package key_value_storage

import (
	"context"
	"github.com/golang-infrastructure/go-iterator"
	"github.com/storage-lock/go-storage-lock/pkg/storage"
	"time"
)

type KeyValueStorage struct {
}

var _ storage.Storage = &KeyValueStorage{}

func (x *KeyValueStorage) ReadValue(ctx context.Context, key string) (string, error) {

}

func (x *KeyValueStorage) WriteKey() {

}

func (x *KeyValueStorage) GetName() string {
	return "key-value-storage"
}

func (x *KeyValueStorage) Init(ctx context.Context) error {
	
}

func (x *KeyValueStorage) UpdateWithVersion(ctx context.Context, lockId string, exceptedVersion, newVersion storage.Version, lockInformation *storage.LockInformation) error {

}

func (x *KeyValueStorage) InsertWithVersion(ctx context.Context, lockId string, version storage.Version, lockInformation *storage.LockInformation) error {
	//TODO implement me
	panic("implement me")
}

func (x *KeyValueStorage) DeleteWithVersion(ctx context.Context, lockId string, exceptedVersion storage.Version, lockInformation *storage.LockInformation) error {
	//TODO implement me
	panic("implement me")
}

func (x *KeyValueStorage) Get(ctx context.Context, lockId string) (string, error) {
	//TODO implement me
	panic("implement me")
}

func (x *KeyValueStorage) GetTime(ctx context.Context) (time.Time, error) {
	//TODO implement me
	panic("implement me")
}

func (x *KeyValueStorage) Close(ctx context.Context) error {
	//TODO implement me
	panic("implement me")
}

func (x *KeyValueStorage) List(ctx context.Context) (iterator.Iterator[*storage.LockInformation], error) {
	//TODO implement me
	panic("implement me")
}
