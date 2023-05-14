package postgresql_storage

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/storage-lock/go-storage-lock/pkg/storage"
	"sync"
)

const DefaultPostgreSQLStorageSchema = "public"

type PostgreSQLConnectionProvider struct {

	// 主机的名字
	Host string

	// 主机的端口
	Port uint

	// 用户名
	User string

	// 密码
	Passwd string

	DatabaseName string

	// DSN
	// Example: "host=192.168.128.206 user=postgres password=123456 port=5432 dbname=postgres sslmode=disable"
	DSN string

	// 初始化好的数据库实例
	db   *sql.DB
	err  error
	once sync.Once
}

var _ storage.ConnectionProvider[*sql.DB] = &PostgreSQLConnectionProvider{}

// NewPostgreSQLConnectionGetterFromDSN 从DSN创建MySQL连接
func NewPostgreSQLConnectionGetterFromDSN(dsn string) *PostgreSQLConnectionProvider {
	return &PostgreSQLConnectionProvider{
		DSN: dsn,
	}
}

// NewPostgreSQLConnectionGetter 从服务器属性创建数据库连接
func NewPostgreSQLConnectionGetter(host string, port uint, user, passwd, databaseName string) *PostgreSQLConnectionProvider {
	return &PostgreSQLConnectionProvider{
		Host:         host,
		Port:         port,
		User:         user,
		Passwd:       passwd,
		DatabaseName: databaseName,
	}
}

func (x *PostgreSQLConnectionProvider) Name() string {
	return "postgresql-connection-provider"
}

// Get 获取到数据库的连接
func (x *PostgreSQLConnectionProvider) Get(ctx context.Context) (*sql.DB, error) {
	x.once.Do(func() {
		db, err := sql.Open("postgres", x.GetDSN())
		if err != nil {
			x.err = err
			return
		}
		x.db = db
	})
	return x.db, x.err
}

func (x *PostgreSQLConnectionProvider) GetDSN() string {
	if x.DSN != "" {
		return x.DSN
	}
	return fmt.Sprintf("host=%s user=%s password=%s port=%d dbname=%s sslmode=disable", x.Host, x.User, x.Passwd, x.Port, x.DatabaseName)
}

