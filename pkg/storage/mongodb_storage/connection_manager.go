package mongodb_storage

import (
	"context"
	"github.com/storage-lock/go-storage-lock/pkg/storage/connection_manager"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"sync"
)

// MongoConnectionManager 根据URI连接Mongo服务器获取连接
type MongoConnectionManager struct {

	// 连接到数据库的选项
	URI string

	// Mongo客户端
	clientOnce sync.Once
	err        error
	client     *mongo.Client
}

var _ connection_manager.ConnectionManager[*mongo.Client] = &MongoConnectionManager{}

func NewMongoConnectionManager(uri string) *MongoConnectionManager {
	return &MongoConnectionManager{
		URI: uri,
	}
}

func NewMongoConnectionManagerFromClient(client *mongo.Client) *MongoConnectionManager {
	return &MongoConnectionManager{
		client: client,
	}
}

func (x *MongoConnectionManager) Name() string {
	return "mongodb-connection-manager"
}

func (x *MongoConnectionManager) Take(ctx context.Context) (*mongo.Client, error) {
	x.clientOnce.Do(func() {
		if x.client == nil {
			client, err := mongo.Connect(ctx, options.Client().ApplyURI(x.URI))
			if err != nil {
				x.err = err
				return
			}
			x.client = client
		}
	})
	return x.client, x.err
}

func (x *MongoConnectionManager) Return(ctx context.Context, connection *mongo.Client) error {
	return nil
}

func (x *MongoConnectionManager) Shutdown(ctx context.Context) error {
	return nil
}
