package storage_lock

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	storage "github.com/storage-lock/go-storage"
)

func TestTryUpgradeOnlyReader(t *testing.T) {
	lock, _, _ := testRWStorageLock(t, "upgrade-only")
	ctx := context.Background()
	require.NoError(t, lock.RLock(ctx, "reader"))

	require.NoError(t, lock.TryUpgrade(ctx, "reader"))
	status, err := lock.Inspect(ctx)
	require.NoError(t, err)
	assert.False(t, status.Information.ReadLock)
	assert.Equal(t, "reader", status.Information.OwnerId)
	assert.Equal(t, 1, status.Information.LockCount)

	// 升级后是独占锁，其他读者进不来。
	readCtx, cancel := context.WithTimeout(ctx, 150*time.Millisecond)
	defer cancel()
	assert.Error(t, lock.RLock(readCtx, "other"))
	require.NoError(t, lock.UnLock(ctx, "reader"))
}

func TestTryUpgradeKeepsSharedReadLock(t *testing.T) {
	lock, _, _ := testRWStorageLock(t, "upgrade-conflict")
	ctx := context.Background()
	require.NoError(t, lock.RLock(ctx, "reader-a"))
	require.NoError(t, lock.RLock(ctx, "reader-b"))

	err := lock.TryUpgrade(ctx, "reader-a")
	require.ErrorIs(t, err, ErrUpgradeConflict)

	status, err := lock.Inspect(ctx)
	require.NoError(t, err)
	assert.True(t, status.Information.ReadLock)
	assert.Equal(t, 2, status.Information.LockCount)
	assert.Equal(t, 1, status.Information.ReadDepth("reader-a"))
	assert.Equal(t, 1, status.Information.ReadDepth("reader-b"))

	require.NoError(t, lock.RUnLock(ctx, "reader-b"))
	require.NoError(t, lock.TryUpgrade(ctx, "reader-a"))
	require.NoError(t, lock.UnLock(ctx, "reader-a"))
}

func TestDowngradeLetsOtherReaderEnter(t *testing.T) {
	lock, _, _ := testRWStorageLock(t, "downgrade")
	ctx := context.Background()
	require.NoError(t, lock.Lock(ctx, "writer"))
	require.NoError(t, lock.Lock(ctx, "writer"))

	require.NoError(t, lock.Downgrade(ctx, "writer"))
	status, err := lock.Inspect(ctx)
	require.NoError(t, err)
	assert.False(t, status.Information.ReadLock, "重入写锁降一层后仍是独占")
	assert.Equal(t, 1, status.Information.LockCount)

	require.NoError(t, lock.Downgrade(ctx, "writer"))
	require.NoError(t, lock.RLock(ctx, "reader"))
	status, err = lock.Inspect(ctx)
	require.NoError(t, err)
	assert.True(t, status.Information.ReadLock)
	assert.Equal(t, 2, status.Information.LockCount)

	require.ErrorIs(t, lock.Downgrade(ctx, "reader"), ErrLockNotBelongYou)
	require.NoError(t, lock.RUnLock(ctx, "writer"))
	require.NoError(t, lock.RUnLock(ctx, "reader"))
}

func TestWriterIntentBlocksNewReaders(t *testing.T) {
	lock, _, _ := testRWStorageLock(t, "writer-priority")
	ctx := context.Background()
	require.NoError(t, lock.RLock(ctx, "reader"))

	acquired := make(chan error, 1)
	go func() { acquired <- lock.WLock(ctx, "writer") }()

	require.Eventually(t, func() bool {
		status, err := lock.Inspect(ctx)
		return err == nil && status.WriteIntentOwner == "writer"
	}, time.Second, 10*time.Millisecond)

	lateCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()
	assert.Error(t, lock.RLock(lateCtx, "late-reader"), "写者排队后新读者不得进入")

	// 已在读的持有者仍然可以重入，不会被写意图打断。
	require.NoError(t, lock.RLock(ctx, "reader"))
	require.NoError(t, lock.RUnLock(ctx, "reader"))
	require.NoError(t, lock.RUnLock(ctx, "reader"))
	require.NoError(t, <-acquired)
	require.NoError(t, lock.UnLock(ctx, "writer"))
}

func TestWaitersVisibleWhileBlocked(t *testing.T) {
	lock, store, clock := testRWStorageLock(t, "waiters")
	ctx := context.Background()
	require.NoError(t, lock.RLock(ctx, "holder"))

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = lock.WLock(ctx, "waiting-writer")
	}()
	require.Eventually(t, func() bool {
		waiters, err := lock.Waiters(ctx)
		return err == nil && len(waiters) == 1 && waiters[0].OwnerId == "waiting-writer" && waiters[0].Mode == "write"
	}, time.Second, 10*time.Millisecond)

	status, err := lock.Inspect(ctx)
	require.NoError(t, err)
	assert.False(t, status.Information.WriteIntentSince.IsZero())
	clock.Advance(2 * time.Second)
	status, err = lock.Inspect(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, status.WriteIntentAge, 2*time.Second)
	_ = store

	require.NoError(t, lock.RUnLock(ctx, "holder"))
	wg.Wait()
	waiters, err := lock.Waiters(ctx)
	require.NoError(t, err)
	assert.Empty(t, waiters)
	require.NoError(t, lock.UnLock(ctx, "waiting-writer"))
}

func TestCanceledWriterLeavesQueue(t *testing.T) {
	lock, _, _ := testRWStorageLock(t, "cancel-waiter")
	ctx := context.Background()
	require.NoError(t, lock.RLock(ctx, "reader"))

	waitCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- lock.WLock(waitCtx, "writer") }()
	require.Eventually(t, func() bool {
		status, err := lock.Inspect(ctx)
		return err == nil && status.WriteIntentOwner == "writer"
	}, time.Second, 10*time.Millisecond)

	cancel()
	require.ErrorIs(t, <-done, context.Canceled)
	require.Eventually(t, func() bool {
		status, err := lock.Inspect(ctx)
		return err == nil && status.WriteIntentOwner == "" && len(status.Waiters) == 0
	}, time.Second, 10*time.Millisecond)

	// 写意图清除后，新读者可以进入。
	require.NoError(t, lock.RLock(ctx, "another"))
	require.NoError(t, lock.RUnLock(ctx, "another"))
	require.NoError(t, lock.RUnLock(ctx, "reader"))
}

func TestFairLockQueueLength(t *testing.T) {
	store := newMockSubscribeStorage()
	fair, err := NewFairStorageLock(store, "fair-queue")
	require.NoError(t, err)
	ctx := context.Background()
	require.NoError(t, fair.Lock(ctx, "holder"))

	waitCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() { _ = fair.Lock(waitCtx, "second") }()
	require.Eventually(t, func() bool { return fair.QueueLength() == 1 }, time.Second, 10*time.Millisecond)
	cancel()
	require.NoError(t, fair.UnLock(ctx, "holder"))
}

func TestLockLostCancelsWithLock(t *testing.T) {
	lock, _, clock := testRWStorageLock(t, "lock-lost-with")
	ctx := context.Background()
	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- lock.WithLock(ctx, func(owned context.Context, _ storage.Version) error {
			close(started)
			<-owned.Done()
			return context.Cause(owned)
		})
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("callback did not start")
	}
	clock.Advance(lock.options.LeaseExpireAfter + time.Second)
	select {
	case err := <-done:
		require.ErrorIs(t, err, ErrLockLost)
	case <-time.After(3 * time.Second):
		t.Fatal("ownership loss did not cancel the callback")
	}
}

func TestBeginLeaseCancelsOnLoss(t *testing.T) {
	lock, _, clock := testRWStorageLock(t, "begin-lease")
	lock.options.LeaseRefreshInterval = 30 * time.Millisecond
	owned, stop, err := lock.BeginLease(context.Background(), "owner")
	require.NoError(t, err)
	defer stop()
	time.Sleep(150 * time.Millisecond)
	clock.Advance(lock.options.LeaseExpireAfter + time.Second)
	select {
	case <-owned.Done():
		require.ErrorIs(t, context.Cause(owned), ErrLockLost)
	case <-time.After(3 * time.Second):
		t.Fatal("begin lease context was not canceled")
	}
	status, err := lock.Inspect(context.Background())
	require.NoError(t, err)
	assert.False(t, status.Held, "丢失后租约不得继续有效")
}

func TestUpgradeRejectsNonReader(t *testing.T) {
	lock, _, _ := testRWStorageLock(t, "upgrade-stranger")
	ctx := context.Background()
	require.NoError(t, lock.RLock(ctx, "reader"))
	require.ErrorIs(t, lock.TryUpgrade(ctx, "stranger"), ErrLockNotBelongYou)
	require.Error(t, lock.TryUpgrade(canceled(ctx), "reader"))
	require.NoError(t, lock.RUnLock(ctx, "reader"))
	require.Error(t, lock.TryUpgrade(ctx, "reader"))
}

// 确认错误类型可被 errors.Is 识别，避免包装层改动后语义漂移。
var _ = errors.Is
