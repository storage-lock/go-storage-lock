package tidb_storage

import (
	"context"
	"github.com/storage-lock/go-storage-lock/pkg/storage/mysql_storage"
	"github.com/storage-lock/go-storage-lock/pkg/storage/test_helper"
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
)

func TestNewTidbStorage(t *testing.T) {
	envName := "STORAGE_LOCK_TIDB_DSN"
	dsn := os.Getenv(envName)
	assert.NotEmpty(t, dsn)
	connectionGetter := NewTidbConnectionProviderFromDSN(dsn)
	storage, err := NewTidbStorage(context.Background(), &TidbStorageOptions{
		MySQLStorageOptions: &mysql_storage.MySQLStorageOptions{
			ConnectionManager: connectionGetter,
			TableName:         test_helper.TestTableName,
		},
	})
	assert.Nil(t, err)
	test_helper.TestStorage(t, storage)
}
