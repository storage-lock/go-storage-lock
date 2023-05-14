package mongodb_storage

import (
	"context"
	"github.com/storage-lock/go-storage-lock/pkg/storage"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"sync"
)

// MongoConnectionProvider 根据URI连接Mongo服务器获取连接
type MongoConnectionProvider struct {

	// 连接到数据库的选项
	URI string

	// Mongo客户端
	clientOnce sync.Once
	err        error
	client     *mongo.Client
}

var _ storage.ConnectionProvider[*mongo.Client] = &MongoConnectionProvider{}

func NewMongoConnectionProvider(uri string) *MongoConnectionProvider {
	return &MongoConnectionProvider{
		URI: uri,
	}
}

func (x *MongoConnectionProvider) Name() string {
	return "mongodb-connection-provider"
}

func (x *MongoConnectionProvider) Get(ctx context.Context) (*mongo.Client, error) {
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
