package postgresql_storage

import (
	"context"
	"github.com/storage-lock/go-storage-lock/pkg/storage/test_helper"
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
)

func TestNewPostgreSQLStorage(t *testing.T) {
	envName := "STORAGE_LOCK_POSTGRESQL_DSN"
	dsn := os.Getenv(envName)
	assert.NotEmpty(t, dsn)
	connectionGetter := NewPostgreSQLConnectionGetterFromDSN(dsn)
	s, err := NewPostgreSQLStorage(context.Background(), &PostgreSQLStorageOptions{
		ConnectionProvider: connectionGetter,
		TableName:          test_helper.TestTableName,
	})
	assert.Nil(t, err)
	test_helper.TestStorage(t, s)
}
