package storage_lock

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestWatchdogLease(t *testing.T) {
	ctx := context.Background()

	t.Run("rollback refresh does not write earlier", func(t *testing.T) {
		store, clock := newBranchClock()
		var lost atomic.Bool
		lock := openBranchLock(t, store, clock, "dog-rollback", true, 0, &lost)
		require.NoError(t, lock.Lock(ctx, "owner"))
		before := lockSnap(t, lock)
		clock.Set(before.LeaseExpireTime.Add(-time.Hour))
		marker := lockSnap(t, lock)
		require.Eventually(t, func() bool {
			status, err := lock.Inspect(ctx)
			if err != nil {
				return false
			}
			info := status.Information
			return info.Version > marker.Version && !info.LeaseExpireTime.Before(marker.LeaseExpireTime)
		}, 5*time.Second, 20*time.Millisecond)
		after := lockSnap(t, lock)
		require.False(t, after.LeaseExpireTime.Before(before.LeaseExpireTime))
		require.False(t, after.LeaseExpireTime.Before(marker.LeaseExpireTime))
		require.Equal(t, before.OwnerId, after.OwnerId)
		require.Equal(t, before.LockCount, after.LockCount)
		require.Equal(t, before.ReadLock, after.ReadLock)
		require.False(t, lost.Load())
		fmt.Println("PASS rollback does not write earlier")
	})

	t.Run("other holder renew is not loss and version still increases", func(t *testing.T) {
		store, clock := newBranchClock()
		var lost atomic.Bool
		first := openBranchLock(t, store, clock, "dog-share", true, 0, &lost)
		second := openBranchLock(t, store, clock, "dog-share", true, 0, nil)
		require.NoError(t, first.RLock(ctx, "reader-a"))
		require.NoError(t, second.RLock(ctx, "reader-b"))
		shared := lockSnap(t, first)
		require.Equal(t, "reader-b", shared.OwnerId)
		require.NotEqual(t, "reader-a", shared.OwnerId)
		require.NoError(t, second.Renew(ctx, "reader-b"))
		renewed := lockSnap(t, second)
		require.Greater(t, renewed.Version, shared.Version)
		require.Equal(t, shared.OwnerId, renewed.OwnerId)
		require.Equal(t, shared.ReadDepth("reader-a"), renewed.ReadDepth("reader-a"))
		require.Equal(t, shared.ReadDepth("reader-b"), renewed.ReadDepth("reader-b"))
		require.Equal(t, shared.ReadLock, renewed.ReadLock)
		require.False(t, lost.Load())
		require.Eventually(t, func() bool {
			status, err := first.Inspect(ctx)
			if err != nil {
				return false
			}
			return status.Information.Version > renewed.Version && !lost.Load()
		}, 5*time.Second, 20*time.Millisecond)
		require.False(t, lost.Load())
		fmt.Println("PASS version conflict no lost")
	})

	t.Run("exact expiry notifies once and does not rewrite", func(t *testing.T) {
		store, clock := newBranchClock()
		var lost atomic.Bool
		lock := openBranchLock(t, store, clock, "dog-exact", true, 0, &lost)
		require.NoError(t, lock.Lock(ctx, "owner"))
		initial := lockSnap(t, lock)
		require.Eventually(t, func() bool {
			status, err := lock.Inspect(ctx)
			if err != nil {
				return false
			}
			return status.Information.Version > initial.Version
		}, 5*time.Second, 20*time.Millisecond)
		running := lockSnap(t, lock)
		clock.Set(running.LeaseExpireTime)
		require.Eventually(t, func() bool { return lost.Load() }, 3*time.Second, 20*time.Millisecond)
		stable := lockSnap(t, lock)
		time.Sleep(1200 * time.Millisecond)
		after := lockSnap(t, lock)
		require.Equal(t, stable.Version, after.Version)
		require.True(t, stable.LeaseExpireTime.Equal(after.LeaseExpireTime))
		require.Equal(t, stable.OwnerId, after.OwnerId)
		require.Equal(t, stable.LockCount, after.LockCount)
		require.True(t, lost.Load())
		fmt.Println("PASS exact expiry lost without rewrite")
	})

	t.Run("takeover or delete notifies and normal release does not", func(t *testing.T) {
		store, clock := newBranchClock()
		var taken atomic.Bool
		holder := openBranchLock(t, store, clock, "dog-take", true, 0, &taken)
		require.NoError(t, holder.Lock(ctx, "owner"))
		owned := lockSnap(t, holder)
		clock.Set(owned.LeaseExpireTime.Add(time.Second))
		other := openBranchLock(t, store, clock, "dog-take", false, 0, nil)
		require.NoError(t, other.Lock(ctx, "other"))
		require.Eventually(t, func() bool { return taken.Load() }, 5*time.Second, 20*time.Millisecond)
		fmt.Println("PASS takeover lost")

		var gone atomic.Bool
		doomed := openBranchLock(t, store, clock, "dog-gone", true, 0, &gone)
		require.NoError(t, doomed.Lock(ctx, "owner"))
		info := lockSnap(t, doomed)
		require.NoError(t, store.DeleteWithVersion(ctx, "dog-gone", info.Version, &info))
		require.Eventually(t, func() bool { return gone.Load() }, 5*time.Second, 20*time.Millisecond)
		fmt.Println("PASS delete lost")

		var quiet atomic.Bool
		normal := openBranchLock(t, store, clock, "dog-quiet", true, 0, &quiet)
		require.NoError(t, normal.Lock(ctx, "owner"))
		require.NoError(t, normal.UnLock(ctx, "owner"))
		time.Sleep(1500 * time.Millisecond)
		require.False(t, quiet.Load())
		fmt.Println("PASS normal unlock no lost")

		var readLost atomic.Bool
		first := openBranchLock(t, store, clock, "dog-read", true, 0, &readLost)
		second := openBranchLock(t, store, clock, "dog-read", true, 0, nil)
		require.NoError(t, first.RLock(ctx, "reader-a"))
		require.NoError(t, second.RLock(ctx, "reader-b"))
		require.NoError(t, first.RUnLock(ctx, "reader-a"))
		time.Sleep(1500 * time.Millisecond)
		require.False(t, readLost.Load())
		still := lockSnap(t, second)
		require.Greater(t, still.ReadDepth("reader-b"), 0)
		require.Equal(t, 0, still.ReadDepth("reader-a"))
		fmt.Println("PASS read unlock no lost")
	})
}
