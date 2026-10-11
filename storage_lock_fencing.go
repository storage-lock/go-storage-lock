package storage_lock

import (
	"context"
	storage "github.com/storage-lock/go-storage"
)

// GetFencingToken 返回当前有效独占锁的版本号，用作栅栏令牌。
// 已释放返回 ErrLockNotFound，其他持有者或共享读锁返回 ErrLockNotBelongYou，
// 租约到期返回 ErrLeaseExpired。
//
// 版本在同一 lockId 的所有权变更之间严格递增，释放和回滚保留版本记录。
// 该保证要求后端持久保留记录：不能清理墓碑、回滚数据或与删除记录的旧客户端混用。
// 同一次持有中，重入和续租也会增加版本；可获取一次并在该次临界区内复用。
//
// 业务资源必须原子地校验令牌并拒绝小于已接受最大值的请求；相同令牌的重复请求
// 另行按业务幂等规则处理。令牌不是锁有效性的持续证明，也不能替代业务幂等。
func (x *StorageLock) GetFencingToken(ctx context.Context, ownerId string) (storage.Version, error) {
	if err := validateOwner(ctx, ownerId); err != nil {
		return 0, err
	}
	status, err := x.Inspect(ctx)
	if err != nil {
		return 0, err
	}
	if err := checkExclusiveOwner(&status.Information, ownerId); err != nil {
		return 0, err
	}
	if !status.Held {
		return 0, ErrLeaseExpired
	}
	return status.Information.Version, nil
}
