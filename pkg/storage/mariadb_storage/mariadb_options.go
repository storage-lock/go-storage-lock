package mariadb_storage

import (
	"github.com/storage-lock/go-storage-lock/pkg/storage/mysql_storage"
)

type MariaStorageOptions struct {
	*mysql_storage.MySQLStorageOptions
}

func NewMariaStorageOptions() *MariaStorageOptions {
	return &MariaStorageOptions{
		MySQLStorageOptions: mysql_storage.NewMySQLStorageOptions(),
	}
}
