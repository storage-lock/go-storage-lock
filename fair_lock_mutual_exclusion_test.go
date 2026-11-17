package storage_lock

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/storage-lock/go-storage"
	"github.com/stretchr/testify/require"
)

// TestFairLockTwoInstancesMutualExclusion 锁住 v0.0.7 及更早版本里的互斥漏洞。
//
// 旧实现用 fmt.Sprintf("fairlock-%s", lockId) 当 owner，同一个 lockId 上所有客户端
// 共用一个 owner，于是第二个实例 TryLock 会因为“同一 owner 重入”直接成功，
// 两个进程可以同时持锁；调用方 UnLock 还会连带释放别人持有的锁。
//
// 这个用例用两个独立的 FairLock 实例（各自有自己的 owner）验证：
// 一方持锁时，另一方拿不到；而且拿不到时存储里的 owner 不能被改写。
func TestFairLockTwoInstancesMutualExclusion(t *testing.T) {
	ctx := context.Background()
	store := newMockSubscribeStorage()
	const lockID = "fair-mutex"

	holder, err := NewFairLock(store, lockID)
	require.NoError(t, err)
	other, err := NewFairLock(store, lockID)
	require.NoError(t, err)

	require.NoError(t, holder.Lock(ctx, "owner-A"), "第一个实例应当拿到锁")

	// 另一个 owner 必须在“锁被占用”上失败，不能因为共用 owner 变成重入。
	errB := other.TryLock(ctx, "owner-B")
	require.Error(t, errB, "第二个实例不能拿到已被占用的公平锁")
	require.ErrorIs(t, errB, ErrLockBusy)

	// 存储里的持有者仍然是最先拿到锁的那个 owner，没被别人的尝试改写。
	raw, err := store.Get(ctx, lockID)
	require.NoError(t, err)
	info, err := storage.LockInformationFromJsonString(raw)
	require.NoError(t, err)
	require.Equal(t, "owner-A", info.OwnerId)
	require.Equal(t, 1, info.LockCount)

	// A 释放后，B 应当能拿到。这里只验证“不是共用 owner 导致的假释放”:
	// 释放前 B 的尝试不能影响 A 的持有，释放后 B 才真正获得。
	require.NoError(t, holder.UnLock(ctx, "owner-A"))
	require.NoError(t, other.TryLock(ctx, "owner-B"))
}

// TestFairLockUnlockDoesNotReleaseOtherOwner 验证 A 释放不会连带释放 B。
//
// 旧实现里 owner 是同一个字符串，所以 A 的 UnLock 实际解锁的是“所有人的锁”。
// 这里先让 A 持锁，B 无法拿到；A 释放后 B 才持有；随后 B 必须能被 B 自己正常释放。
func TestFairLockUnlockDoesNotReleaseOtherOwner(t *testing.T) {
	ctx := context.Background()
	store := newMockSubscribeStorage()
	const lockID = "fair-unlock-scope"

	a, err := NewFairLock(store, lockID)
	require.NoError(t, err)
	b, err := NewFairLock(store, lockID)
	require.NoError(t, err)

	require.NoError(t, a.Lock(ctx, "owner-A"))
	require.ErrorIs(t, b.TryLock(ctx, "owner-B"), ErrLockBusy)

	// A 未释放前，B 的释放尝试不得生效。
	require.Error(t, b.UnLock(ctx, "owner-B"), "B 没持有锁时必须拒绝释放")

	require.NoError(t, a.UnLock(ctx, "owner-A"))
	require.NoError(t, b.Lock(ctx, "owner-B"))

	require.NoError(t, b.UnLock(ctx, "owner-B"), "B 必须能正常释放自己拿到的锁")

	// 释放后锁应有墓碑记录（LockCount == 0），不是被物理删除。
	raw, err := store.Get(ctx, lockID)
	require.NoError(t, err)
	info, err := storage.LockInformationFromJsonString(raw)
	require.NoError(t, err)
	require.Equal(t, 0, info.LockCount)
}

// TestFairLockConcurrentInstancesCriticalSection 在并发下验证临界区不重叠。
//
// 多个独立的 FairLock 实例（各自不同 owner）反复争抢同一把锁，
// 进入临界区的计数永远不能大于 1。
func TestFairLockConcurrentInstancesCriticalSection(t *testing.T) {
	ctx := context.Background()
	store := newMockSubscribeStorage()
	const lockID = "fair-mutex-concurrent"

	var inCritical int32
	var violations int32
	const goroutines = 6
	const rounds = 5

	done := make(chan struct{}, goroutines)
	for i := 0; i < goroutines; i++ {
		go func(i int) {
			defer func() { done <- struct{}{} }()
			lock, err := NewFairLock(store, lockID)
			if err != nil {
				return
			}
			owner := fmt.Sprintf("owner-%d", i)
			for j := 0; j < rounds; j++ {
				if err := lock.Lock(ctx, owner); err != nil {
					return
				}
				if atomic.AddInt32(&inCritical, 1) > 1 {
					atomic.AddInt32(&violations, 1)
				}
				time.Sleep(100 * time.Microsecond)
				atomic.AddInt32(&inCritical, -1)
				_ = lock.UnLock(ctx, owner)
			}
		}(i)
	}
	for i := 0; i < goroutines; i++ {
		<-done
	}
	require.Zero(t, violations, "并发下出现了多个持有者，互斥被破坏")
}
