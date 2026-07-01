package storage_lock

import (
	"context"
	"github.com/storage-lock/go-events"
	go_storage "github.com/storage-lock/go-storage"
	storage_events "github.com/storage-lock/go-storage-events"
)

// 栅栏令牌（Fencing Token）——修复"租约安全边界"这一分布式锁的经典理论漏洞。
//
// # 理论背景
//
// 所有基于租约（lease）的分布式锁都存在一个无法在锁层单独消除的边界：
// 持有者 A 若因 STW GC、进程长时间停顿、或网络分区导致租约在其【自身无感知】
// 的情况下过期，另一个客户端 B 可以合法抢占该锁。此刻 A（从停顿中恢复）与 B
// 会【同时认为】自己持有锁。
//
//   - 【锁记录层】的互斥由 CAS 保证：B 抢占后锁的 Version 已推进，A 后续对锁
//     记录的任何 UpdateWithVersion/DeleteWithVersion 都会 ErrVersionMiss。这部分是安全的。
//   - 【被保护资源层】的互斥锁框架无从拦截：A 恢复后若直接对业务资源（写文件、
//     扣库存、改数据库行）动手，锁框架看不到这次写入，也就拦不住。
//
// 这不是本框架特有的 bug，而是 Redlock 等所有租约锁的共有边界（见 Martin Kleppmann
// "How to do distributed locking"）。消除它的唯一理论手段是【栅栏令牌】。
//
// # 栅栏令牌如何消除该边界
//
// 锁每次易主时发放一个【单调递增】的令牌；被保护资源记住"见过的最大令牌"，
// 拒绝任何比它更小的令牌。于是即便停顿恢复的 A 仍以为自己持锁，它手里的旧令牌
// 也已小于 B 的新令牌，对资源的写入会被资源侧拒绝——互斥性下沉到被保护资源。
//
// 本框架的 LockInformation.Version 天然就是这样一个量：
//   - 全局锁记录只有一份，Version 随每次修改单调 +1；
//   - 抢占过期锁走 lockExpired，新 Version = 旧记录 Version + 1（storage_lock_lock.go）；
//   - 因此 B 抢占后的 Version 严格大于 A 曾持有过的任何 Version（含续租推进的值）。
//
// 所以 Version 可直接充当栅栏令牌，无需引入新的单调计数器。GetFencingToken 只是
// 把这个【已经存在但从未暴露】的量开放给调用方——这正是漏洞 F 的修复：能力本就
// 具备，缺的只是 API 出口。
//
// # 用法
//
//	if err := lock.Lock(ctx, ownerId); err != nil { ... }
//	token, err := lock.GetFencingToken(ctx, ownerId) // 持锁后取令牌
//	// 之后每次对被保护资源的写入都携带 token，
//	// 由资源侧校验 token 单调不减（拒绝 <= 已见过的最大值的写入）。
//
// 需要【被保护资源级别】互斥保证（严格正确性、不容忍上述停顿窗口）的场景才需要栅栏令牌；
// 若被保护的临界区完全在进程内、且信任租约+看门狗，可不使用。

// GetFencingToken 返回当前锁的栅栏令牌（即锁记录的当前 Version）。
//
// 仅当锁当前仍属于 ownerId 时返回有效令牌；若锁不存在返回 ErrLockNotFound，
// 若锁已不属于 ownerId（已被他人抢占）返回 ErrLockNotBelongYou——这本身也是
// 一个有用信号：说明调用方可能已经失去了锁，不应再对被保护资源动手。
//
// 令牌在跨所有权变更时严格单调递增，但在同一次持有期间会因看门狗续租而增大。
// 因此若需要在整个临界区使用一个稳定令牌，调用方应在 Lock 成功后取一次并自行保存，
// 而非每次写入前重新获取；任一时刻取到的令牌都小于后续抢占者的令牌，栅栏语义均成立。
func (x *StorageLock) GetFencingToken(ctx context.Context, ownerId string) (go_storage.Version, error) {

	lockId := x.options.LockId
	e := events.NewEvent(lockId).SetOwnerId(ownerId).SetStorageName(x.storage.GetName()).SetListeners(x.options.EventListeners)

	lockInformation, err := x.getLockInformation(ctx, e.Fork(), lockId)
	if err != nil {
		// 锁不存在（ErrLockNotFound）或读取失败，直接透传
		return 0, err
	}

	// 墓碑（LockCount==0）等价于锁已释放，其上不存在有效持有者，令牌无意义
	if lockInformation.LockCount == 0 {
		e.Fork().AddActionByName(ActionLockNotExists).Publish(ctx)
		return 0, ErrLockNotFound
	}

	// 锁已不属于调用方——它已经失去了这把锁（可能因租约过期被抢占）
	if lockInformation.OwnerId != ownerId {
		e.Fork().AddAction(events.NewAction(ActionNotLockOwner).AddPayload(storage_events.PayloadLockInformation, lockInformation)).Publish(ctx)
		return 0, ErrLockNotBelongYou
	}

	return lockInformation.Version, nil
}
