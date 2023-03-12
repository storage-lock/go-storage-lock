package storage_lock

import (
	"context"
	"errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"time"
)

// ------------------------------------------------ ---------------------------------------------------------------------

// MongoConfigurationConnectionGetter 根据URI连接Mongo服务器获取连接
type MongoConfigurationConnectionGetter struct {

	// 连接到数据库的选项
	URI string

	// Mongo客户端
	client *mongo.Client
}

var _ ConnectionGetter[*mongo.Client] = &MongoConfigurationConnectionGetter{}

func NewMongoConfigurationConnectionGetter(uri string) *MongoConfigurationConnectionGetter {
	return &MongoConfigurationConnectionGetter{
		URI: uri,
	}
}

func (x *MongoConfigurationConnectionGetter) Get(ctx context.Context) (*mongo.Client, error) {
	if x.client == nil {
		if x.URI == "" {
			return nil, errors.New("mongo uri can not be empty")
		}
		client, err := mongo.Connect(ctx, options.Client().ApplyURI(x.URI))
		if err != nil {
			return nil, err
		}
		x.client = client
	}
	return x.client, nil
}

// ------------------------------------------------ ---------------------------------------------------------------------

// MongoStorageOptions Mongo的存储选项
type MongoStorageOptions struct {

	// 获取连接
	ConnectionGetter ConnectionGetter[*mongo.Client]

	// 数据库名称
	DatabaseName string

	// 集合名称
	CollectionName string
}

// ------------------------------------------------ ---------------------------------------------------------------------

type MongoStorage struct {
	options *MongoStorageOptions

	client     *mongo.Client
	collection *mongo.Collection

	session mongo.Session
}

var _ Storage = &MongoStorage{}

func NewMongoStorage(options *MongoStorageOptions) *MongoStorage {
	return &MongoStorage{
		options: options,
	}
}

func (x *MongoStorage) Init(ctx context.Context) error {
	client, err := x.options.ConnectionGetter.Get(ctx)
	if err != nil {
		return err
	}
	database := client.Database(x.options.DatabaseName)
	if database == nil {
		// TODO
		return nil
	}
	database.Collection("")

	// 初始化
	session, err := x.client.StartSession()
	if err != nil {
		return err
	}
	x.session = session
	return nil
}

func (x *MongoStorage) UpdateWithVersion(ctx context.Context, lockId string, exceptedVersion, newVersion Version, lockInformation *LockInformation, lockInformationJsonString string) error {
	filter := bson.M{
		"_id": bson.M{
			"$eq": lockId,
		},
		"version": bson.M{
			"$eq": exceptedVersion,
		},
	}
	rs, err := x.collection.UpdateOne(ctx, filter, &MongoLock{
		ID:             lockId,
		Version:        newVersion,
		LockJsonString: lockInformationJsonString,
	})
	if err != nil {
		return err
	}
	if rs.ModifiedCount == 0 {
		return ErrVersionMiss
	}
	return nil
}

func (x *MongoStorage) InsertWithVersion(ctx context.Context, lockId string, version Version, lockInformation *LockInformation, lockInformationJsonString string) error {
	_, err := x.collection.InsertOne(ctx, &MongoLock{
		ID:             lockId,
		Version:        version,
		LockJsonString: lockInformationJsonString,
	})
	return err
}

func (x *MongoStorage) DeleteWithVersion(ctx context.Context, lockId string, exceptedVersion Version, lockInformation *LockInformation) error {
	filter := bson.M{
		"_id": bson.M{
			"$eq": lockId,
		},
		"version": bson.M{
			"$eq": exceptedVersion,
		},
	}
	rs, err := x.collection.DeleteOne(ctx, filter)
	if err != nil {
		return err
	}
	if rs.DeletedCount == 0 {
		return ErrVersionMiss
	}
	return nil
}

func (x *MongoStorage) Get(ctx context.Context, lockId string) (string, error) {
	filter := bson.M{
		"_id": bson.M{
			"$eq": lockId,
		},
	}
	one := x.collection.FindOne(ctx, filter)
	if one.Err() != nil {
		return "", one.Err()
	}
	bytes, err := one.DecodeBytes()
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func (x *MongoStorage) GetTime(ctx context.Context) (time.Time, error) {
	// TODO 待定
	//x.session.ClusterTime()
	return time.Time{}, nil
}

func (x *MongoStorage) Close(ctx context.Context) error {
	if x.client != nil {
		return x.client.Disconnect(ctx)
	}
	return nil
}

// ------------------------------------------------ ---------------------------------------------------------------------

// MongoLock 锁在Mongo中存储的结构
type MongoLock struct {

	// 锁的ID，这个字段是一个唯一字段
	ID string `bson:"_id"`

	// 锁的版本，每次修改都会增加1
	Version Version `bson:"version"`

	// 锁的json信息，存储着更上层的通用的锁的信息，这里只需要认为它是一个字符串就可以了
	LockJsonString string `bson:"lock_json_string"`
}

// ------------------------------------------------ ---------------------------------------------------------------------
