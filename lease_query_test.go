package storage_lock

import (
	"context"
	"errors"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/storage-lock/go-storage"
	"github.com/stretchr/testify/require"
)

// cancelMissStorage 在第一次版本冲突返回前取消调用方上下文。
type cancelMissStorage struct {
	*scriptedCAS
	cancel context.CancelFunc
}

func (c *cancelMissStorage) UpdateWithVersion(ctx context.Context, lockID string, oldVersion, newVersion storage.Version, info *storage.LockInformation) error {
	err := c.scriptedCAS.UpdateWithVersion(ctx, lockID, oldVersion, newVersion, info)
	if errors.Is(err, ErrVersionMiss) {
		c.cancel()
	}
	return err
}

func TestLeaseQueryBranches(t *testing.T) {
	ctx := context.Background()

	t.Run("renew failures leave the record unchanged", func(t *testing.T) {
		store := newMockSubscribeStorage()
		clock := &storedClock{mockSubscribeTimeProvider: newMockSubscribeTimeProvider(time.Now())}
		lock := openStoredLock(t, store, clock, "query-clock", false, nil)
		require.NoError(t, lock.Lock(ctx, "owner"))
		require.NoError(t, lock.Lock(ctx, "owner"))
		before := lockSnap(t, lock)
		clock.fail.Store(true)
		require.ErrorIs(t, lock.Renew(ctx, "owner"), errStoredClock)
		got := storedSnap(t, store, "query-clock")
		require.Equal(t, before.Version, got.Version)
		require.True(t, before.LeaseExpireTime.Equal(got.LeaseExpireTime))
		require.Equal(t, before.LockCount, got.LockCount)
		require.Equal(t, before.OwnerId, got.OwnerId)
		require.Equal(t, before.ReadLock, got.ReadLock)
		fmt.Println("PASS renew time failure")

		clock.fail.Store(false)
		exhausted := openStoredLock(t, store, clock, "query-version", false, nil)
		require.NoError(t, exhausted.Lock(ctx, "owner"))
		store.mu.Lock()
		store.data["query-version"].Version = math.MaxUint64
		store.mu.Unlock()
		held := lockSnap(t, exhausted)
		require.Equal(t, storage.Version(math.MaxUint64), held.Version)
		require.ErrorIs(t, exhausted.Renew(ctx, "owner"), ErrVersionExhausted)
		requireStable(t, exhausted, held)
		fmt.Println("PASS renew version exhausted")

		base, baseClock := newBranchClock()
		script := &scriptedCAS{mockSubscribeStorage: base, updateErr: ErrVersionMiss}
		renewCtx, cancel := context.WithCancel(ctx)
		wrapper := &cancelMissStorage{scriptedCAS: script, cancel: cancel}
		contended := openBranchLock(t, wrapper, baseClock, "query-cancel", false, 0, nil)
		require.NoError(t, contended.Lock(renewCtx, "owner"))
		stable := lockSnap(t, contended)
		script.failUpdates = 1
		require.ErrorIs(t, contended.Renew(renewCtx, "owner"), context.Canceled)
		require.Equal(t, 0, script.failUpdates)
		requireStable(t, contended, stable)
		fmt.Println("PASS renew cancel while waiting")
	})

	t.Run("ttl matches the same inspect and does not write", func(t *testing.T) {
		store, clock := newBranchClock()
		lock := openBranchLock(t, store, clock, "query-ttl", false, 0, nil)
		require.NoError(t, lock.Lock(ctx, "owner"))
		require.NoError(t, lock.Lock(ctx, "owner"))
		clock.Advance(time.Second)
		require.NoError(t, lock.Renew(ctx, "owner"))
		status, err := lock.Inspect(ctx)
		require.NoError(t, err)
		ttl, err := lock.TTL(ctx, "owner")
		require.NoError(t, err)
		require.Equal(t, status.RemainingLease, ttl)
		require.Equal(t, status.Information.LeaseExpireTime.Sub(status.ObservedAt), ttl)
		require.Greater(t, ttl, time.Duration(0))
		before := lockSnap(t, lock)
		again, err := lock.TTL(ctx, "owner")
		require.NoError(t, err)
		require.Equal(t, ttl, again)
		requireStable(t, lock, before)
		fmt.Println("PASS ttl matches inspect")
	})
}
