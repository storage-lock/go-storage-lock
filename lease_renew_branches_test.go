package storage_lock

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLeaseRenewBranches(t *testing.T) {
	ctx := context.Background()

	t.Run("exclusive reentry", func(t *testing.T) {
		store, clock := newBranchClock()
		lock := openBranchLock(t, store, clock, "renew-reentry", false, 0, nil)
		require.NoError(t, lock.Lock(ctx, "owner"))
		require.NoError(t, lock.Lock(ctx, "owner"))
		before := lockSnap(t, lock)
		require.Greater(t, before.LockCount, 1)
		clock.Advance(time.Second)
		require.NoError(t, lock.Renew(ctx, "owner"))
		after := lockSnap(t, lock)
		require.True(t, after.LeaseExpireTime.After(before.LeaseExpireTime))
		require.Equal(t, before.Version+1, after.Version)
		require.Equal(t, before.LockCount, after.LockCount)
		require.Equal(t, before.OwnerId, after.OwnerId)
		require.False(t, after.ReadLock)
		fmt.Println("PASS exclusive reentry")
	})

	t.Run("both readers extend the same lease", func(t *testing.T) {
		store, clock := newBranchClock()
		first := openBranchLock(t, store, clock, "renew-readers", false, 0, nil)
		second := openBranchLock(t, store, clock, "renew-readers", false, 0, nil)
		require.NoError(t, first.RLock(ctx, "reader-a"))
		require.NoError(t, first.RLock(ctx, "reader-a"))
		require.NoError(t, second.RLock(ctx, "reader-b"))
		shared := lockSnap(t, first)
		require.Equal(t, "reader-b", shared.OwnerId)
		require.Equal(t, 2, shared.ReadDepth("reader-a"))
		require.Equal(t, 1, shared.ReadDepth("reader-b"))
		require.True(t, shared.ReadLock)

		clock.Advance(time.Second)
		require.NoError(t, second.Renew(ctx, "reader-b"))
		last := lockSnap(t, second)
		require.True(t, last.LeaseExpireTime.After(shared.LeaseExpireTime))
		require.Equal(t, shared.Version+1, last.Version)
		require.Equal(t, shared.LockCount, last.LockCount)
		require.Equal(t, shared.OwnerId, last.OwnerId)
		require.True(t, last.ReadLock)
		require.Equal(t, 2, last.ReadDepth("reader-a"))
		require.Equal(t, 1, last.ReadDepth("reader-b"))
		fmt.Println("PASS last reader")

		clock.Advance(time.Second)
		require.NoError(t, first.Renew(ctx, "reader-a"))
		earlier := lockSnap(t, first)
		require.True(t, earlier.LeaseExpireTime.After(last.LeaseExpireTime))
		require.Equal(t, last.Version+1, earlier.Version)
		require.Equal(t, last.LockCount, earlier.LockCount)
		require.Equal(t, "reader-b", earlier.OwnerId)
		require.True(t, earlier.ReadLock)
		require.Equal(t, 2, earlier.ReadDepth("reader-a"))
		require.Equal(t, 1, earlier.ReadDepth("reader-b"))
		fmt.Println("PASS earlier reader")
	})

	t.Run("clock rollback does not write earlier", func(t *testing.T) {
		store, clock := newBranchClock()
		lock := openBranchLock(t, store, clock, "renew-rollback", false, 0, nil)
		require.NoError(t, lock.Lock(ctx, "owner"))
		before := lockSnap(t, lock)
		clock.Set(before.LeaseExpireTime.Add(-time.Hour))
		require.NoError(t, lock.Renew(ctx, "owner"))
		after := lockSnap(t, lock)
		require.False(t, after.LeaseExpireTime.Before(before.LeaseExpireTime))
		require.Equal(t, before.Version+1, after.Version)
		require.Equal(t, before.LockCount, after.LockCount)
		require.Equal(t, before.OwnerId, after.OwnerId)
		require.Equal(t, before.ReadLock, after.ReadLock)
		fmt.Println("PASS clock rollback")
	})

	t.Run("failures leave the record unchanged", func(t *testing.T) {
		store, clock := newBranchClock()
		lock := openBranchLock(t, store, clock, "renew-fail", false, 0, nil)
		require.NoError(t, lock.Lock(ctx, "owner"))
		require.NoError(t, lock.Lock(ctx, "owner"))
		held := lockSnap(t, lock)

		require.ErrorIs(t, lock.Renew(ctx, "stranger"), ErrLockNotBelongYou)
		requireStable(t, lock, held)
		fmt.Println("PASS non-holder")

		require.ErrorIs(t, lock.Renew(ctx, ""), ErrOwnerIdEmpty)
		requireStable(t, lock, held)
		fmt.Println("PASS empty owner")

		canceled, cancel := context.WithCancel(ctx)
		cancel()
		require.ErrorIs(t, lock.Renew(canceled, "owner"), context.Canceled)
		requireStable(t, lock, held)
		fmt.Println("PASS canceled context")

		clock.Set(held.LeaseExpireTime)
		require.ErrorIs(t, lock.Renew(ctx, "owner"), ErrLeaseExpired)
		requireStable(t, lock, held)
		fmt.Println("PASS exact expiry")

		clock.Set(held.LeaseExpireTime.Add(-time.Minute))
		require.NoError(t, lock.UnLock(ctx, "owner"))
		require.NoError(t, lock.UnLock(ctx, "owner"))
		tomb := lockSnap(t, lock)
		require.Equal(t, 0, tomb.LockCount)
		require.ErrorIs(t, lock.Renew(ctx, "owner"), ErrLockNotFound)
		requireStable(t, lock, tomb)
		fmt.Println("PASS tombstone")

		missing := openBranchLock(t, store, clock, "renew-missing", false, 0, nil)
		require.ErrorIs(t, missing.Renew(ctx, "owner"), ErrLockNotFound)
		requireAbsent(t, store, "renew-missing")
		fmt.Println("PASS missing record")
	})

	t.Run("version miss retries once", func(t *testing.T) {
		store, clock := newBranchClock()
		script := &scriptedCAS{mockSubscribeStorage: store, updateErr: ErrVersionMiss}
		lock := openBranchLock(t, script, clock, "renew-miss", false, 0, nil)
		require.NoError(t, lock.Lock(ctx, "owner"))
		before := lockSnap(t, lock)
		script.failUpdates = 1
		require.NoError(t, lock.Renew(ctx, "owner"))
		after := lockSnap(t, lock)
		require.Equal(t, 0, script.failUpdates)
		require.Equal(t, before.Version+1, after.Version)
		require.False(t, after.LeaseExpireTime.Before(before.LeaseExpireTime))
		require.Equal(t, before.LockCount, after.LockCount)
		require.Equal(t, before.OwnerId, after.OwnerId)
		require.Equal(t, before.ReadLock, after.ReadLock)
		fmt.Println("PASS version miss retry")
	})
}
