package mysql_storage

import (
	"context"
	"github.com/storage-lock/go-storage-lock/pkg/storage/test_helper"
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
)

func TestNewMySQLStorage(t *testing.T) {
	envName := "STORAGE_LOCK_MYSQL_DSN"
	dsn := os.Getenv(envName)
	assert.NotEmpty(t, dsn)
	connectionGetter := NewMySQLConnectionProviderFromDSN(dsn)
	s, err := NewMySQLStorage(context.Background(), &MySQLStorageOptions{
		ConnectionManager: connectionGetter,
		TableName:         "storage_lock_test",
	})
	assert.Nil(t, err)
	test_helper.TestStorage(t, s)
}
