package storage_lock

import (
	"context"
	"errors"
	"fmt"

	storage "github.com/storage-lock/go-storage"
)

// WithLock 获取独占锁，将本次持有的栅栏令牌传给 fn，并在退出时释放锁。
// 每次调用生成独立 ownerId；fn 正常返回、返回错误或 panic 都会尝试释放。
// 释放使用保留 ctx 值但不受其取消影响的上下文，受 options.UnlockTimeout 限制。
// 回调和释放错误通过 errors.Join 合并；panic 在清理后继续传播。
//
// fn 必须响应 ctx 取消。持有权被看门狗确认丢失时，传给 fn 的 context 会被取消；
// 这不能追回已经发出的写入，业务资源仍必须自行校验栅栏令牌。
func (x *StorageLock) WithLock(ctx context.Context, fn func(context.Context, storage.Version) error) (err error) {
	if fn == nil {
		return ErrCallbackNil
	}
	ownerId := x.ownerIdGenerator.GenOwnerId()
	if err := x.Lock(ctx, ownerId); err != nil {
		return err
	}
	defer func() {
		cleanupCtx, cancel := x.cleanupContext(ctx)
		defer cancel()
		if releaseErr := x.UnLock(cleanupCtx, ownerId); releaseErr != nil {
			err = errors.Join(err, fmt.Errorf("release lock: %w", releaseErr))
		}
	}()
	token, err := x.GetFencingToken(ctx, ownerId)
	if err != nil {
		return err
	}
	// 获取与签发令牌之间调用方可能取消；此时不能执行业务回调。
	if err := context.Cause(ctx); err != nil {
		return err
	}
	owned, cancelOwned := x.OwnedContext(ctx)
	defer cancelOwned()
	return fn(owned, token)
}

// ErrLockLost 表示看门狗确认持有权已经丢失，业务 context 因此被取消。
var ErrLockLost = errors.New("lock ownership lost")

// OwnedContext 派生业务上下文：调用方取消或持有权丢失都会取消它。
// 返回的 cancel 不释放锁。看门狗未启动时只跟随调用方的取消。
func (x *StorageLock) OwnedContext(ctx context.Context) (context.Context, context.CancelFunc) {
	owned, cancel := context.WithCancelCause(ctx)
	lost := x.lostChannel()
	go func() {
		select {
		case <-owned.Done():
		case <-lost:
			cancel(ErrLockLost)
		}
	}()
	return owned, func() { cancel(context.Canceled) }
}
