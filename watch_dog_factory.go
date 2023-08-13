package storage_lock

import (
	"context"
	"github.com/storage-lock/go-events"
)

// WatchDogFactory 负责创建看门狗的工厂
type WatchDogFactory interface {

	// Name 此工厂是什么品种的狗狗而工作，用于表明工厂的类型
	Name() string

	// New 创建一只看门狗
	// ctx: 控制超时传递上下文之类的
	// e: 新创建的WatchDog如果需要支持事件的话，则保存这个WatchDog的根事件，后面它所触发的事件都是这个事件的子事件
	// lock: 在为哪个锁创建WatchDog，WatchDog可能会用到锁的一些上下文之类的，比如lockId啥的
	// ownerId: 这个WatchDog是为哪个主人而创建，狗狗都是很忠诚的，一只狗狗终生只为一个主人守护他的锁
	New(ctx context.Context, e *events.Event, lock *StorageLock, ownerId string) (WatchDog, error)
}
