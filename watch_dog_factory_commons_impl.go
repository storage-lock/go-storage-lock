package storage_lock

import (
	"context"
	"github.com/storage-lock/go-events"
)

// WatchDogFactoryCommonsImpl 默认的WatchDogFactory的实现，当创建锁的时候未指定WatchDogFactory时便会使用此实现
type WatchDogFactoryCommonsImpl struct {
}

var _ WatchDogFactory = &WatchDogFactoryCommonsImpl{}

func NewWatchDogFactoryCommonsImpl() *WatchDogFactoryCommonsImpl {
	return &WatchDogFactoryCommonsImpl{}
}

const WatchDogFactoryCommonsImplName = "watch-dog-factory-commons-impl"

func (x *WatchDogFactoryCommonsImpl) Name() string {
	return WatchDogFactoryCommonsImplName
}

func (x *WatchDogFactoryCommonsImpl) NewWatchDog(ctx context.Context, e *events.Event, lock *StorageLock, ownerId string) (WatchDog, error) {
	return NewWatchDogCommonsImpl(ctx, e, lock, ownerId), nil
}
