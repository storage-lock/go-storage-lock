package mysql_storage

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/storage-lock/go-storage-lock/pkg/storage/connection_manager"
	"sync"
)

// MySQLConnectionManager 创建一个MySQL的连接管理器
type MySQLConnectionManager struct {

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

var _ connection_manager.ConnectionManager[*sql.DB] = &MySQLConnectionManager{}

// NewMySQLConnectionProviderFromDSN 从DSN创建MySQL连接
func NewMySQLConnectionProviderFromDSN(dsn string) *MySQLConnectionManager {
	return &MySQLConnectionManager{
		DSN: dsn,
	}
}

// NewMySQLConnectionProvider 从服务器属性创建数据库连接
func NewMySQLConnectionProvider(host string, port uint, user, passwd, database string) *MySQLConnectionManager {
	return &MySQLConnectionManager{
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

func (x *MySQLConnectionManager) WithHost(host string) *MySQLConnectionManager {
	x.Host = host
	return x
}

func (x *MySQLConnectionManager) WithPort(port uint) *MySQLConnectionManager {
	x.Port = port
	return x
}

func (x *MySQLConnectionManager) WithUser(user string) *MySQLConnectionManager {
	x.User = user
	return x
}

func (x *MySQLConnectionManager) WithPasswd(passwd string) *MySQLConnectionManager {
	x.Passwd = passwd
	return x
}

func (x *MySQLConnectionManager) WithDatabaseName(databaseName string) *MySQLConnectionManager {
	x.DatabaseName = databaseName
	return x
}

// WithDSN Example: "root:UeGqAm8CxYGldMDLoNNt@tcp(192.168.128.206:3306)/storage_lock_test"
func (x *MySQLConnectionManager) WithDSN(dsn string) *MySQLConnectionManager {
	x.DSN = dsn
	return x
}

func (x *MySQLConnectionManager) Name() string {
	return "mysql-connection-provider"
}

// Take( 获取到数据库的连接
func (x *MySQLConnectionManager) Take(ctx context.Context) (*sql.DB, error) {
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

func (x *MySQLConnectionManager) GetDSN() string {
	if x.DSN != "" {
		return x.DSN
	}
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s", x.User, x.Passwd, x.Host, x.Port, x.DatabaseName)
}

func (x *MySQLConnectionManager) Return(ctx context.Context, db *sql.DB) error {
	return nil
}

func (x *MySQLConnectionManager) Shutdown(ctx context.Context) error {
	return nil
}
