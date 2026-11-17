package storage_lock

import (
	"context"
	"fmt"
	"math"
	"sync/atomic"
	"testing"
	"time"

	"github.com/storage-lock/go-storage"
	"github.com/stretchr/testify/require"
)

func TestWatchdogBlip(t *testing.T) {
	ctx := context.Background()

	t.Run("time source failure then recovery", func(t *testing.T) {
		store := newMockSubscribeStorage()
		clock := &storedClock{mockSubscribeTimeProvider: newMockSubscribeTimeProvider(time.Now())}
		var lost atomic.Bool
		lock := openStoredLock(t, store, clock, "blip-clock", true, &lost)
		require.NoError(t, lock.Lock(ctx, "owner"))
		initial := lockSnap(t, lock)
		require.Eventually(t, func() bool {
			status, err := lock.Inspect(ctx)
			return err == nil && status.Information.Version > initial.Version
		}, 5*time.Second, 20*time.Millisecond)
		clock.fail.Store(true)
		time.Sleep(50 * time.Millisecond)
		marker := storedSnap(t, store, "blip-clock")
		time.Sleep(1500 * time.Millisecond)
		during := storedSnap(t, store, "blip-clock")
		require.Equal(t, marker.Version, during.Version)
		require.True(t, marker.LeaseExpireTime.Equal(during.LeaseExpireTime))
		require.Equal(t, marker.OwnerId, during.OwnerId)
		require.Equal(t, marker.LockCount, during.LockCount)
		require.Equal(t, marker.ReadLock, during.ReadLock)
		require.False(t, lost.Load())
		fmt.Println("PASS time failure does not lose or rewrite")

		clock.fail.Store(false)
		require.Eventually(t, func() bool {
			status, err := lock.Inspect(ctx)
			if err != nil || lost.Load() {
				return false
			}
			info := status.Information
			return info.Version > during.Version && !info.LeaseExpireTime.Before(during.LeaseExpireTime)
		}, 5*time.Second, 20*time.Millisecond)
		require.False(t, lost.Load())
		recovered := lockSnap(t, lock)
		require.Equal(t, during.OwnerId, recovered.OwnerId)
		require.Equal(t, during.LockCount, recovered.LockCount)
		require.Equal(t, during.ReadLock, recovered.ReadLock)
		fmt.Println("PASS time recovery refreshes without writing earlier")
	})

	t.Run("version exhausted then delete notifies", func(t *testing.T) {
		store, clock := newBranchClock()
		var lost atomic.Bool
		lock := openBranchLock(t, store, clock, "blip-version", true, 0, &lost)
		require.NoError(t, lock.Lock(ctx, "owner"))
		initial := lockSnap(t, lock)
		require.Eventually(t, func() bool {
			status, err := lock.Inspect(ctx)
			return err == nil && status.Information.Version > initial.Version
		}, 5*time.Second, 20*time.Millisecond)
		store.mu.Lock()
		store.data["blip-version"].Version = math.MaxUint64
		store.mu.Unlock()
		time.Sleep(50 * time.Millisecond)
		marker := lockSnap(t, lock)
		require.Equal(t, storage.Version(math.MaxUint64), marker.Version)
		time.Sleep(1500 * time.Millisecond)
		during := lockSnap(t, lock)
		require.Equal(t, marker.Version, during.Version)
		require.True(t, marker.LeaseExpireTime.Equal(during.LeaseExpireTime))
		require.Equal(t, marker.OwnerId, during.OwnerId)
		require.Equal(t, marker.LockCount, during.LockCount)
		require.False(t, lost.Load())
		fmt.Println("PASS version exhausted does not lose or rewrite")

		require.NoError(t, store.DeleteWithVersion(ctx, "blip-version", during.Version, &during))
		require.Eventually(t, func() bool { return lost.Load() }, 5*time.Second, 20*time.Millisecond)
		fmt.Println("PASS delete after version exhausted notifies")
	})
}
