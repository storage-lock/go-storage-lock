package mongodb_storage

import (
	"errors"
	"github.com/storage-lock/go-storage-lock/pkg/storage"
	"github.com/storage-lock/go-storage-lock/pkg/storage/connection_manager"
	"go.mongodb.org/mongo-driver/mongo"
)

// MongoStorageOptions Mongo的存储选项
type MongoStorageOptions struct {

	// 获取连接
	ConnectionProvider connection_manager.ConnectionManager[*mongo.Client]

	// 要存储到的数据库的名称
	DatabaseName string

	// 集合名称
	CollectionName string
}

func NewMongoStorageOptions() *MongoStorageOptions {
	return &MongoStorageOptions{
		DatabaseName:   storage.DefaultStorageDatabaseName,
		CollectionName: storage.DefaultStorageTableName,
	}
}

func NewMongoStorageOptionsWithURI(uri string) *MongoStorageOptions {
	return NewMongoStorageOptions().WithConnectionProvider(NewMongoConnectionManager(uri))
}

func (x *MongoStorageOptions) WithDatabaseName(databaseName string) *MongoStorageOptions {
	x.DatabaseName = databaseName
	return x
}

func (x *MongoStorageOptions) WithCollectionName(collectionName string) *MongoStorageOptions {
	x.CollectionName = collectionName
	return x
}

func (x *MongoStorageOptions) WithConnectionProvider(connectionProvider connection_manager.ConnectionManager[*mongo.Client]) *MongoStorageOptions {
	x.ConnectionProvider = connectionProvider
	return x
}

func (x *MongoStorageOptions) Check() error {
	if x.DatabaseName == "" {
		return errors.New("mongodb database name can not be empty")
	}
	if x.CollectionName == "" {
		return errors.New("mongodb collection name can not be empty")
	}
	if x.ConnectionProvider == nil {
		return errors.New("ConnectionManager must set")
	}
	return nil
}
