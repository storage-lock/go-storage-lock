package sql_server_storage

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/storage-lock/go-storage-lock/pkg/storage/connection_manager"
	"sync"
)

// SqlServerStorageConnectionManager 创建一个SqlServer的连接
type SqlServerStorageConnectionManager struct {

	// 主机的名字
	Host string

	// 主机的端口
	Port uint

	// 用户名
	User string

	// 密码
	Passwd string

	// DSN
	// Example: "sqlserver://sa:UeGqAm8CxYGldMDLoNNt@192.168.128.206:1433"
	DSN string

	// 初始化好的数据库实例
	db   *sql.DB
	err  error
	once sync.Once
}

var _ connection_manager.ConnectionManager[*sql.DB] = &SqlServerStorageConnectionManager{}

// NewSqlServerStorageConnectionGetterFromDSN 从DSN创建SqlServer连接
func NewSqlServerStorageConnectionGetterFromDSN(dsn string) *SqlServerStorageConnectionManager {
	return &SqlServerStorageConnectionManager{
		DSN: dsn,
	}
}

// NewSqlServerStorageConnectionGetter 从服务器属性创建数据库连接
func NewSqlServerStorageConnectionGetter(host string, port uint, user, passwd string) *SqlServerStorageConnectionManager {
	return &SqlServerStorageConnectionManager{
		Host:   host,
		Port:   port,
		User:   user,
		Passwd: passwd,
	}
}

func (x *SqlServerStorageConnectionManager) Name() string {
	return "sql-server-connection-provider"
}

func (x *SqlServerStorageConnectionManager) GetDSN() string {
	if x.DSN != "" {
		return x.DSN
	}
	return fmt.Sprintf("sqlserver://%s:%s@%s:%d", x.User, x.Passwd, x.Host, x.Port)
}

// Take 获取到数据库的连接
func (x *SqlServerStorageConnectionManager) Take(ctx context.Context) (*sql.DB, error) {
	x.once.Do(func() {
		//db, err := sql.Open("sqlserver", x.GetDSN())
		db, err := sql.Open("mssql", x.GetDSN())
		if err != nil {
			x.err = err
			return
		}
		x.db = db
	})
	return x.db, x.err
}

func (x *SqlServerStorageConnectionManager) Return(ctx context.Context, db *sql.DB) error {
	return nil
}

func (x *SqlServerStorageConnectionManager) Shutdown(ctx context.Context) error {
	return nil
}
