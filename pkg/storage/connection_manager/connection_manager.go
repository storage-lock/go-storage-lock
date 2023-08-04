package connection_manager

import (
	"context"
)

// ConnectionManager 把数据库连接的使用管理为一个组件，属于比较底层的接口，用来适配上层的各种情况
// 比如上层可以是从DSN直接创建连接，也可以是从连接池中拿出来连接，甚至从已有的ORM、sqlx、sql.DB中复用连接
// 或者任何你想扩展的实现，总之它是一个带泛型的接口，你可以任意创造！
type ConnectionManager[Connection any] interface {

	// Name 连接提供器的名字，用于区分不同的连接提供器，连接器的名字必须指定不允许为空字符串
	Name() string

	// Take 获取Storage的连接
	Take(ctx context.Context) (Connection, error)

	// Return 使用完毕，把Storage的连接归还
	Return(ctx context.Context, connection Connection) error

	// Shutdown 把整个连接管理器关闭掉
	Shutdown(ctx context.Context) error
}
