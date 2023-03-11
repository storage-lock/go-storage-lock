package storage_lock

import (
	"context"
	"database/sql"
	"sync"
)

// ------------------------------------------------ ---------------------------------------------------------------------

// ConnectionGetter 获取连接
type ConnectionGetter[Connection any] interface {

	// Get 获取连接
	Get(ctx context.Context) (Connection, error)
}

// ------------------------------------------------ ---------------------------------------------------------------------

// FuncConnectionGetter 通过一个函数获取连接
type FuncConnectionGetter[Connection any] struct {
	f func() (Connection, error)
}

var _ ConnectionGetter[any] = &FuncConnectionGetter[any]{}

func NewFuncConnectionGetter[Connection any](f func() (Connection, error)) *FuncConnectionGetter[Connection] {
	return &FuncConnectionGetter[Connection]{
		f: f,
	}
}

func (x *FuncConnectionGetter[Connection]) Get(ctx context.Context) (Connection, error) {
	return x.f()
}

// ------------------------------------------------ ---------------------------------------------------------------------

// ConfigurationConnectionGetter 根据配合获取连接
type ConfigurationConnectionGetter[Connection any] struct {
}

var _ ConnectionGetter[any] = &ConfigurationConnectionGetter[any]{}

func (x *ConfigurationConnectionGetter[Connection]) Get(ctx context.Context) (Connection, error) {
	//TODO implement me
	panic("implement me")
}

// ------------------------------------------------ ---------------------------------------------------------------------

// SQLStorageConnectionGetter 创建到SQL类型的数据库的连接
type SQLStorageConnectionGetter struct {

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

var _ ConnectionGetter[*sql.DB] = &SQLStorageConnectionGetter{}

// NewSQLStorageConnectionGetterFromDSN 从DSN创建MySQL连接
func NewSQLStorageConnectionGetterFromDSN(dsn string) *SQLStorageConnectionGetter {
	return &SQLStorageConnectionGetter{
		DSN: dsn,
	}
}

// NewSQLStorageConnectionGetter 从服务器属性创建数据库连接
func NewSQLStorageConnectionGetter(host string, port uint, user, passwd string) *SQLStorageConnectionGetter {
	return &SQLStorageConnectionGetter{
		Host:   host,
		Port:   port,
		User:   user,
		Passwd: passwd,
	}
}

// Get 获取到数据库的连接
func (x *SQLStorageConnectionGetter) Get(ctx context.Context) (*sql.DB, error) {
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
