package mongodb_storage

import (
	"context"
	"github.com/storage-lock/go-storage-lock/pkg/storage/test_helper"
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
)

func TestNewMongoStorage(t *testing.T) {
	envName := "STORAGE_LOCK_MONGO_URI"
	uri := os.Getenv(envName)
	assert.NotEmpty(t, uri)
	connectionGetter := NewMongoConnectionManager(uri)
	s, err := NewMongoStorage(context.Background(), &MongoStorageOptions{
		ConnectionProvider: connectionGetter,
		DatabaseName:       test_helper.TestDatabaseName,
		CollectionName:     test_helper.TestTableName,
	})
	assert.Nil(t, err)
	test_helper.TestStorage(t, s)
}
