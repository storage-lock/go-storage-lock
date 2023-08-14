package storage_lock

import (
	"context"
	"github.com/storage-lock/go-events"
)

// WatchDogFactoryFuncWrapper 用于不声明struct的情况下创建工厂
type WatchDogFactoryFuncWrapper struct {

	// factory的名字
	name string

	// 实际创建WatchDog的工厂方法
	newFunc func(ctx context.Context, e *events.Event, lock *StorageLock, ownerId string) (WatchDog, error)
}

var _ WatchDogFactory = &WatchDogFactoryFuncWrapper{}

func NewWatchDogFactoryFuncWrapper(name string, newFunc func(ctx context.Context, e *events.Event, lock *StorageLock, ownerId string) (WatchDog, error)) *WatchDogFactoryFuncWrapper {
	return &WatchDogFactoryFuncWrapper{
		name:    name,
		newFunc: newFunc,
	}
}

func (x *WatchDogFactoryFuncWrapper) Name() string {
	return x.name
}

func (x *WatchDogFactoryFuncWrapper) NewWatchDog(ctx context.Context, e *events.Event, lock *StorageLock, ownerId string) (WatchDog, error) {
	return x.newFunc(ctx, e, lock, ownerId)
}
