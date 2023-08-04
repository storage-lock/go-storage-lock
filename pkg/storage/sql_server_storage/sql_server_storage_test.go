package sql_server_storage

import (
	"context"
	"github.com/storage-lock/go-storage-lock/pkg/storage/test_helper"
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
)

func TestNewSqlServerStorage(t *testing.T) {
	envName := "STORAGE_LOCK_SQLSERVER_DSN"
	dsn := os.Getenv(envName)
	assert.NotEmpty(t, dsn)
	connectionGetter := NewSqlServerStorageConnectionGetterFromDSN(dsn)
	storage, err := NewSqlServerStorage(context.Background(), &SqlServerStorageOptions{
		ConnectionManager: connectionGetter,
		TableName:         test_helper.TestTableName,
	})
	assert.Nil(t, err)
	test_helper.TestStorage(t, storage)
}
