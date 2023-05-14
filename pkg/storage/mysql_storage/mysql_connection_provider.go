package mysql_storage

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/storage-lock/go-storage-lock/pkg/storage"
	"sync"
)

// MySQLConnectionProvider 创建一个MySQL的连接
type MySQLConnectionProvider struct {

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
	// Example: "root:UeGqAm8CxYGldMDLoNNt@tcp(192.168.128.206:3306)/storage_lock_test"
	DSN string

	// 初始化好的数据库实例
	db   *sql.DB
	err  error
	once sync.Once
}

var _ storage.ConnectionProvider[*sql.DB] = &MySQLConnectionProvider{}

// NewMySQLConnectionProviderFromDSN 从DSN创建MySQL连接
func NewMySQLConnectionProviderFromDSN(dsn string) *MySQLConnectionProvider {
	return &MySQLConnectionProvider{
		DSN: dsn,
	}
}

// NewMySQLConnectionProvider 从服务器属性创建数据库连接
func NewMySQLConnectionProvider(host string, port uint, user, passwd, database string) *MySQLConnectionProvider {
	return &MySQLConnectionProvider{
		Host:         host,
		Port:         port,
		User:         user,
		Passwd:       passwd,
		DatabaseName: database,
	}
}

//	// DSN
//
//	DSN string

func (x *MySQLConnectionProvider) WithHost(host string) *MySQLConnectionProvider {
	x.Host = host
	return x
}

func (x *MySQLConnectionProvider) WithPort(port uint) *MySQLConnectionProvider {
	x.Port = port
	return x
}

func (x *MySQLConnectionProvider) WithUser(user string) *MySQLConnectionProvider {
	x.User = user
	return x
}

func (x *MySQLConnectionProvider) WithPasswd(passwd string) *MySQLConnectionProvider {
	x.Passwd = passwd
	return x
}

func (x *MySQLConnectionProvider) WithDatabaseName(databaseName string) *MySQLConnectionProvider {
	x.DatabaseName = databaseName
	return x
}

// WithDSN Example: "root:UeGqAm8CxYGldMDLoNNt@tcp(192.168.128.206:3306)/storage_lock_test"
func (x *MySQLConnectionProvider) WithDSN(dsn string) *MySQLConnectionProvider {
	x.DSN = dsn
	return x
}

func (x *MySQLConnectionProvider) Name() string {
	return "mysql-connection-provider"
}

// Get 获取到数据库的连接
func (x *MySQLConnectionProvider) Get(ctx context.Context) (*sql.DB, error) {
	x.once.Do(func() {
		db, err := sql.Open("mysql", x.GetDSN())
		if err != nil {
			x.err = err
			return
		}
		x.db = db
	})
	return x.db, x.err
}

func (x *MySQLConnectionProvider) GetDSN() string {
	if x.DSN != "" {
		return x.DSN
	}
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s", x.User, x.Passwd, x.Host, x.Port, x.DatabaseName)
}
