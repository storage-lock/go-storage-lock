package storage_lock

import "context"

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
