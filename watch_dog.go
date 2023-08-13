package storage_lock

import (
	"context"
	"github.com/storage-lock/go-events"
)

// WatchDog 看门狗协程，让锁的实现者可以自己提供续租的具体实现来替换掉内置的
type WatchDog interface {

	// Name 狗狗的品种
	Name() string

	// Start 启动看门狗
	Start(ctx context.Context) error

	// Stop 停止看门狗
	Stop(ctx context.Context) error

	// GetID 获取狗狗的ID
	GetID() string

	// Eventable 为狗狗设置事件
	//SetEvent(e *events.Event)
	events.Eventable
}
