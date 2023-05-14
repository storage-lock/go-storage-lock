package tidb_storage

import (
	"github.com/storage-lock/go-storage-lock/pkg/storage/mysql_storage"
)

type TidbStorageOptions struct {
	*mysql_storage.MySQLStorageOptions
}

func NewTidbStorageOptions() *TidbStorageOptions {
	return &TidbStorageOptions{mysql_storage.NewMySQLStorageOptions()}
}

