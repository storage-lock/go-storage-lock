package storage_lock

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"sync"
	"time"
)

// ------------------------------------------------- --------------------------------------------------------------------

// NewMongoStorageLock 高层API，使用默认配置快速创建基于Mongo的分布式锁
func NewMongoStorageLock(ctx context.Context, lockId string, uri string) (*StorageLock, error) {
	connectionGetter := NewMongoConfigurationConnectionGetter(uri)
	storageOptions := &MongoStorageOptions{
		ConnectionGetter: connectionGetter,
		DatabaseName:     DefaultStorageTableName,
		CollectionName:   DefaultStorageTableName,
	}

	storage, err := NewMongoStorage(ctx, storageOptions)
	if err != nil {
		return nil, err
	}

	lockOptions := &StorageLockOptions{
		LockId:                lockId,
		LeaseExpireAfter:      DefaultLeaseExpireAfter,
		LeaseRefreshInterval:  DefaultLeaseRefreshInterval,
		VersionMissRetryTimes: DefaultVersionMissRetryTimes,
	}
	return NewStorageLock(storage, lockOptions), nil
}

// ------------------------------------------------ ---------------------------------------------------------------------

// MongoStorageConnectionGetter 根据URI连接Mongo服务器获取连接
type MongoStorageConnectionGetter struct {

	// 连接到数据库的选项
	URI string

	// Mongo客户端
	clientOnce sync.Once
	err        error
	client     *mongo.Client
}

var _ ConnectionGetter[*mongo.Client] = &MongoStorageConnectionGetter{}

func NewMongoConfigurationConnectionGetter(uri string) *MongoStorageConnectionGetter {
	return &MongoStorageConnectionGetter{
		URI: uri,
	}
}

func (x *MongoStorageConnectionGetter) Get(ctx context.Context) (*mongo.Client, error) {
	x.clientOnce.Do(func() {
		client, err := mongo.Connect(ctx, options.Client().ApplyURI(x.URI))
		if err != nil {
			x.err = err
			return
		}
		x.client = client
	})
	return x.client, x.err
}

// ------------------------------------------------ ---------------------------------------------------------------------

// MongoStorageOptions Mongo的存储选项
type MongoStorageOptions struct {

	// 获取连接
	ConnectionGetter ConnectionGetter[*mongo.Client]

	// 要存储到的数据库的名称
	DatabaseName string

	// 集合名称
	CollectionName string
}

// ------------------------------------------------ ---------------------------------------------------------------------

type MongoStorage struct {
	options *MongoStorageOptions

	client     *mongo.Client
	database   *mongo.Database
	collection *mongo.Collection

	session mongo.Session
}

var _ Storage = &MongoStorage{}

func NewMongoStorage(ctx context.Context, options *MongoStorageOptions) (*MongoStorage, error) {
	storage := &MongoStorage{
		options: options,
	}

	err := storage.Init(ctx)
	if err != nil {
		return nil, err
	}

	return storage, nil
}

func (x *MongoStorage) Init(ctx context.Context) error {

	client, err := x.options.ConnectionGetter.Get(ctx)
	if err != nil {
		return err
	}
	database := client.Database(x.options.DatabaseName)
	collection := database.Collection(x.options.CollectionName)
	// 初始化
	session, err := client.StartSession()
	if err != nil {
		return err
	}

	x.client = client
	x.session = session
	x.database = database
	x.collection = collection

	return nil
}

func (x *MongoStorage) UpdateWithVersion(ctx context.Context, lockId string, exceptedVersion, newVersion Version, lockInformation *LockInformation) error {
	filter := bson.M{
		"_id": bson.M{
			"$eq": lockId,
		},
		"owner_id": bson.M{
			"$eq": lockInformation.OwnerId,
		},
		"version": bson.M{
			"$eq": exceptedVersion,
		},
	}
	rs, err := x.collection.UpdateOne(ctx, filter, bson.M{
		"$set": &MongoLock{
			ID:             lockId,
			OwnerId:        lockInformation.OwnerId,
			Version:        newVersion,
			LockJsonString: lockInformation.ToJsonString(),
		},
	})
	if err != nil {
		return err
	}
	if rs.ModifiedCount == 0 {
		return ErrVersionMiss
	}
	return nil
}

func (x *MongoStorage) InsertWithVersion(ctx context.Context, lockId string, version Version, lockInformation *LockInformation) error {
	_, err := x.collection.InsertOne(ctx, &MongoLock{
		ID:             lockId,
		OwnerId:        lockInformation.OwnerId,
		Version:        version,
		LockJsonString: lockInformation.ToJsonString(),
	})
	return err
}

func (x *MongoStorage) DeleteWithVersion(ctx context.Context, lockId string, exceptedVersion Version, lockInformation *LockInformation) error {
	filter := bson.M{
		"_id": bson.M{
			"$eq": lockId,
		},
		"owner_id": bson.M{
			"$eq": lockInformation.OwnerId,
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

func IntToBytes(n int) []byte {
	x := int32(n)

	bytesBuffer := bytes.NewBuffer([]byte{})
	binary.Write(bytesBuffer, binary.BigEndian, x)
	return bytesBuffer.Bytes()
}

func (x *MongoStorage) Get(ctx context.Context, lockId string) (string, error) {
	filter := bson.M{
		"_id": bson.M{
			"$eq": lockId,
		},
	}
	one := x.collection.FindOne(ctx, filter)
	if one.Err() != nil {
		if errors.Is(one.Err(), mongo.ErrNoDocuments) {
			return "", ErrLockNotFound
		}
		return "", one.Err()
	}
	mongoLock := &MongoLock{}
	err := one.Decode(mongoLock)
	if err != nil {
		return "", err
	}
	return mongoLock.LockJsonString, nil
}

func (x *MongoStorage) GetTime(ctx context.Context) (time.Time, error) {
	// TODO 日了狗啊，到底该怎么拿Mongo服务器时间啊
	return time.Now(), nil
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

	// 锁的当前持有者的ID
	OwnerId string `bson:"owner_id"`

	// 锁的版本，每次修改都会增加1
	Version Version `bson:"version"`

	// 锁的json信息，存储着更上层的通用的锁的信息，这里只需要认为它是一个字符串就可以了
	LockJsonString string `bson:"lock_json_string"`
}

// ------------------------------------------------ ---------------------------------------------------------------------
