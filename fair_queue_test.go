package storage_lock

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fairLock 用同一份存储创建一把跨实例的公平锁。
// 两个返回值代表两个进程：它们不共享任何内存里的队列，只共享存储记录。
func fairLock(t *testing.T, store *mockSubscribeStorage, clock *mockSubscribeTimeProvider, lockId string) *StorageLock {
	t.Helper()
	options := NewStorageLockOptionsWithLockId(lockId).
		SetTimeProvider(clock).
		SetFair(true).
		SetLeaseExpireAfter(5 * time.Minute).
		SetVersionMissRetryInterval(5 * time.Millisecond)
	lock, err := NewStorageLockWithOptions(store, options)
	require.NoError(t, err)
	return lock
}

func TestFairQueueOrdersAcrossInstances(t *testing.T) {
	store := newMockSubscribeStorage()
	clock := newMockSubscribeTimeProvider(time.Now())
	first := fairLock(t, store, clock, "fair-order")
	second := fairLock(t, store, clock, "fair-order")
	ctx := context.Background()

	require.NoError(t, first.Lock(ctx, "holder"))

	// 后启动的进程先登记。登记顺序写在存储里，不看谁的进程先跑起来。
	lateCtx, cancelLate := context.WithCancel(ctx)
	defer cancelLate()
	lateDone := make(chan error, 1)
	go func() { lateDone <- second.Lock(lateCtx, "second") }()
	require.Eventually(t, func() bool {
		waiters, err := first.Waiters(ctx)
		return err == nil && len(waiters) == 1 && waiters[0].OwnerId == "second"
	}, time.Second, 5*time.Millisecond)

	earlyDone := make(chan error, 1)
	go func() { earlyDone <- first.Lock(ctx, "first") }()
	require.Eventually(t, func() bool {
		waiters, err := first.Waiters(ctx)
		return err == nil && len(waiters) == 2
	}, time.Second, 5*time.Millisecond)

	require.NoError(t, first.UnLock(ctx, "holder"))

	// 先登记的进程先拿到锁，后登记的继续等。
	select {
	case err := <-lateDone:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("队首没有拿到锁")
	}
	select {
	case err := <-earlyDone:
		t.Fatalf("队尾不应在队首之前拿到锁: %v", err)
	case <-time.After(150 * time.Millisecond):
	}

	require.NoError(t, second.UnLock(ctx, "second"))
	require.NoError(t, <-earlyDone)

	status, err := second.Inspect(ctx)
	require.NoError(t, err)
	assert.Equal(t, "first", status.Information.OwnerId)
	assert.Empty(t, status.Waiters)
	require.NoError(t, first.UnLock(ctx, "first"))
}

func TestFairQueueRejectsCuttingIn(t *testing.T) {
	store := newMockSubscribeStorage()
	clock := newMockSubscribeTimeProvider(time.Now())
	queued := fairLock(t, store, clock, "fair-cut")
	unfair := fairLock(t, store, clock, "fair-cut")
	unfair.options.Fair = false
	ctx := context.Background()

	require.NoError(t, queued.Lock(ctx, "holder"))
	waitCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- queued.Lock(waitCtx, "queued") }()
	require.Eventually(t, func() bool {
		waiters, err := queued.Waiters(ctx)
		return err == nil && len(waiters) == 1
	}, time.Second, 5*time.Millisecond)
	require.NoError(t, queued.UnLock(ctx, "holder"))

	// 关闭公平的实例看到的是一条空闲记录，但它提交的 CAS 版本已经被队首的抢占推进，所以拿不到。
	assert.ErrorIs(t, unfair.TryLock(ctx, "cutter"), ErrLockBusy)

	cancel()
	require.Error(t, <-done)
}
