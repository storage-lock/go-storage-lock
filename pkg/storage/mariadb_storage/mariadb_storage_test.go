package mariadb_storage

import (
	"context"
	"github.com/storage-lock/go-storage-lock/pkg/storage/mysql_storage"
	"github.com/storage-lock/go-storage-lock/pkg/storage/test_helper"
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
)

func TestNewMariaDbStorage(t *testing.T) {
	envName := "STORAGE_LOCK_MARIA_DSN"
	dsn := os.Getenv(envName)
	assert.NotEmpty(t, dsn)
	s, err := NewMariaDbStorage(context.Background(), &MariaStorageOptions{
		MySQLStorageOptions: &mysql_storage.MySQLStorageOptions{
			ConnectionProvider: NewMariaStorageConnectionProviderFromDSN(dsn),
			TableName:          test_helper.TestTableName,
		},
	})
	assert.Nil(t, err)
	test_helper.TestStorage(t, s)
}
