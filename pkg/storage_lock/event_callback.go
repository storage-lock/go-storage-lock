package storage_lock

import "context"

// 定义锁的声明周期中的各个阶段的事件回调，让外部能够有机会切入进来或者感知到

// WatchDogRefresh 锁被刷新的时候
type WatchDogRefresh func(ctx context.Context, lockId string, isRefreshSuccess bool)

// LockInitDoneCallback 锁初始化完成时的回调事件
type LockInitDoneCallback func(ctx context.Context, lockId string, err error)
