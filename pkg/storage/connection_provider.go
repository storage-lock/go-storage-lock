package storage

import (
	"context"
	"database/sql"
	"sync"
)

// ------------------------------------------------ ---------------------------------------------------------------------

// ConnectionProvider 提供数据库连接
type ConnectionProvider[Connection any] interface {

	// Name 连接提供器的名字，用于区分不同的连接提供器
	Name() string

	// Get 获取数据库连接
	Get(ctx context.Context) (Connection, error)
}

// ------------------------------------------------ ---------------------------------------------------------------------

// FuncConnectionProvider 通过一个函数获取连接，这样就不必再单独写一个接口了
type FuncConnectionProvider[Connection any] struct {
	name string
	f    func() (Connection, error)
}

var _ ConnectionProvider[any] = &FuncConnectionProvider[any]{}

func NewFuncConnectionProvider[Connection any](name string, f func() (Connection, error)) *FuncConnectionProvider[Connection] {
	return &FuncConnectionProvider[Connection]{
		name: name,
		f:    f,
	}
}

func (x *FuncConnectionProvider[Connection]) Name() string {
	return x.name
}

func (x *FuncConnectionProvider[Connection]) Get(ctx context.Context) (Connection, error) {
	return x.f()
}

// ------------------------------------------------ ---------------------------------------------------------------------

// ConfigurationConnectionProvider 根据配合获取连接
type ConfigurationConnectionProvider[Connection any] struct {
}

var _ ConnectionProvider[any] = &ConfigurationConnectionProvider[any]{}

func (x *ConfigurationConnectionProvider[Connection]) Name() string {
	return "configuration-connection-provider"
}

func (x *ConfigurationConnectionProvider[Connection]) Get(ctx context.Context) (Connection, error) {
	//TODO implement me
	panic("implement me")
}

// ------------------------------------------------ ---------------------------------------------------------------------

// SQLStorageConnectionProvider 创建到SQL类型的数据库的连接
type SQLStorageConnectionProvider struct {

	// 主机的名字
	Host string

	// 主机的端口
	Port uint

	// 用户名
	User string

	// 密码
	Passwd string

	// DSN
	// "root:123456@tcp(127.0.0.1:4000)/test?charset=utf8mb4"
	DSN string

	// 初始化好的数据库实例
	db   *sql.DB
	err  error
	once sync.Once
}

var _ ConnectionProvider[*sql.DB] = &SQLStorageConnectionProvider{}

// NewSQLStorageConnectionGetterFromDSN 从DSN创建MySQL连接
func NewSQLStorageConnectionGetterFromDSN(dsn string) *SQLStorageConnectionProvider {
	return &SQLStorageConnectionProvider{
		DSN: dsn,
	}
}

// NewSQLStorageConnectionGetter 从服务器属性创建数据库连接
func NewSQLStorageConnectionGetter(host string, port uint, user, passwd string) *SQLStorageConnectionProvider {
	return &SQLStorageConnectionProvider{
		Host:   host,
		Port:   port,
		User:   user,
		Passwd: passwd,
	}
}

func (x *SQLStorageConnectionProvider) Name() string {
	return "sql-storage-connection-provider"
}

// Get 获取到数据库的连接
func (x *SQLStorageConnectionProvider) Get(ctx context.Context) (*sql.DB, error) {
	x.once.Do(func() {
		// TODO 此处修改为具体的实现
		db, err := sql.Open("mysql", x.DSN)
		if err != nil {
			x.err = err
			return
		}
		x.db = db
	})
	return x.db, x.err
}

// ------------------------------------------------- --------------------------------------------------------------------
