package cassandra_storage

//import (
//	"context"
//	"github.com/gocql/gocql"
//	"github.com/storage-lock/go-storage-lock/pkg/storage"
//	"time"
//)
//
//// 	以Cassandra为存储引擎
//
//type CassandraStorage struct {
//}
//
//var _ storage.Storage = &CassandraStorage{}
//
//func (x *CassandraStorage) Init(ctx context.Context) error {
//
//}
//
//func (x *CassandraStorage) UpdateWithVersion(ctx context.Context, lockId string, exceptedVersion, newVersion storage.Version, lockInformation *storage.LockInformation) error {
//	//TODO implement me
//	panic("implement me")
//}
//
//func (x *CassandraStorage) InsertWithVersion(ctx context.Context, lockId string, version storage.Version, lockInformation *storage.LockInformation) error {
//	//TODO implement me
//	panic("implement me")
//}
//
//func (x *CassandraStorage) DeleteWithVersion(ctx context.Context, lockId string, exceptedVersion storage.Version, lockInformation *storage.LockInformation) error {
//	//TODO implement me
//	panic("implement me")
//}
//
//func (x *CassandraStorage) Take(ctx context.Context, lockId string) (string, error) {
//	//TODO implement me
//	panic("implement me")
//}
//
//func (x *CassandraStorage) GetTime(ctx context.Context) (time.Time, error) {
//	//TODO implement me
//	panic("implement me")
//}
//
//func (x *CassandraStorage) Close(ctx context.Context) error {
//	//TODO implement me
//	panic("implement me")
//}
