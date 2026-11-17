package storage_lock

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/storage-lock/go-events"
	"github.com/storage-lock/go-storage"
	"github.com/stretchr/testify/require"
)

// errStoredClock 是测试时间源失败时返回的固定错误，用来核对调用方拿到的就是这个错误。
var errStoredClock = errors.New("clock down")

// storedClock 在 fail 置位时拒绝提供时间，其余时刻转发到可注入时钟。
type storedClock struct {
	*mockSubscribeTimeProvider
	fail atomic.Bool
}

func (c *storedClock) GetTime(ctx context.Context) (time.Time, error) {
	if c.fail.Load() {
		return time.Time{}, errStoredClock
	}
	return c.mockSubscribeTimeProvider.GetTime(ctx)
}

func storedSnap(t *testing.T, store *mockSubscribeStorage, id string) storage.LockInformation {
	t.Helper()
	raw, err := store.Get(context.Background(), id)
	require.NoError(t, err)
	info, err := storage.LockInformationFromJsonString(raw)
	require.NoError(t, err)
	return *info
}

func openStoredLock(t *testing.T, store *mockSubscribeStorage, clock *storedClock, id string, watch bool, lost *atomic.Bool) *StorageLock {
	t.Helper()
	options := NewStorageLockOptionsWithLockId(id).
		SetTimeProvider(clock).
		SetVersionMissRetryInterval(5 * time.Millisecond).
		SetDisableWatchDog(!watch)
	if watch {
		options.SetLeaseExpireAfter(3 * time.Second).SetLeaseRefreshInterval(time.Second)
	} else {
		options.SetLeaseExpireAfter(5 * time.Minute)
	}
	if lost != nil {
		options.SetOnLockLost(func(error) { lost.Store(true) })
	}
	lock, err := NewStorageLockWithOptions(store, options)
	require.NoError(t, err)
	if watch {
		t.Cleanup(func() {
			lock.stopWatchDog(context.Background(), events.NewEvent(id), nil)
		})
	}
	return lock
}

func TestLeaseDepthBranches(t *testing.T) {
	ctx := context.Background()

	t.Run("rollback does not write earlier", func(t *testing.T) {
		store, clock := newBranchClock()
		exclusive := openBranchLock(t, store, clock, "depth-ex", false, 0, nil)
		require.NoError(t, exclusive.Lock(ctx, "owner"))
		before := lockSnap(t, exclusive)
		clock.Set(before.LeaseExpireTime.Add(-time.Hour))
		require.NoError(t, exclusive.Lock(ctx, "owner"))
		after := lockSnap(t, exclusive)
		require.False(t, after.LeaseExpireTime.Before(before.LeaseExpireTime))
		require.Equal(t, before.Version+1, after.Version)
		require.Equal(t, before.LockCount+1, after.LockCount)
		require.Equal(t, before.OwnerId, after.OwnerId)
		require.False(t, after.ReadLock)
		fmt.Println("PASS exclusive reentry rollback")

		clock.Set(after.LeaseExpireTime.Add(-time.Hour))
		partial := lockSnap(t, exclusive)
		require.NoError(t, exclusive.UnLock(ctx, "owner"))
		released := lockSnap(t, exclusive)
		require.False(t, released.LeaseExpireTime.Before(partial.LeaseExpireTime))
		require.Equal(t, partial.Version+1, released.Version)
		require.Equal(t, partial.LockCount-1, released.LockCount)
		require.Greater(t, released.LockCount, 0)
		require.Equal(t, partial.OwnerId, released.OwnerId)
		require.False(t, released.ReadLock)
		fmt.Println("PASS exclusive partial unlock rollback")

		first := openBranchLock(t, store, clock, "depth-read", false, 0, nil)
		second := openBranchLock(t, store, clock, "depth-read", false, 0, nil)
		require.NoError(t, first.RLock(ctx, "reader-a"))
		require.NoError(t, second.RLock(ctx, "reader-b"))
		shared := lockSnap(t, first)
		require.Equal(t, "reader-b", shared.OwnerId)
		require.Equal(t, 1, shared.ReadDepth("reader-a"))
		clock.Set(shared.LeaseExpireTime.Add(-time.Hour))
		require.NoError(t, first.RLock(ctx, "reader-a"))
		reentered := lockSnap(t, first)
		require.False(t, reentered.LeaseExpireTime.Before(shared.LeaseExpireTime))
		require.Equal(t, shared.Version+1, reentered.Version)
		require.Equal(t, 2, reentered.ReadDepth("reader-a"))
		require.Equal(t, 1, reentered.ReadDepth("reader-b"))
		require.True(t, reentered.ReadLock)
		require.Equal(t, shared.LockCount+1, reentered.LockCount)
		fmt.Println("PASS earlier reader reentry rollback")

		clock.Set(reentered.LeaseExpireTime.Add(-time.Hour))
		held := lockSnap(t, first)
		require.NoError(t, first.RUnLock(ctx, "reader-a"))
		stepped := lockSnap(t, first)
		require.False(t, stepped.LeaseExpireTime.Before(held.LeaseExpireTime))
		require.Equal(t, held.Version+1, stepped.Version)
		require.Equal(t, 1, stepped.ReadDepth("reader-a"))
		require.Equal(t, 1, stepped.ReadDepth("reader-b"))
		require.True(t, stepped.ReadLock)
		require.Equal(t, held.LockCount-1, stepped.LockCount)
		fmt.Println("PASS read partial unlock rollback")

		only := openBranchLock(t, store, clock, "depth-up", false, 0, nil)
		require.NoError(t, only.RLock(ctx, "reader-a"))
		read := lockSnap(t, only)
		clock.Set(read.LeaseExpireTime.Add(-time.Hour))
		require.NoError(t, only.TryUpgrade(ctx, "reader-a"))
		upgraded := lockSnap(t, only)
		require.False(t, upgraded.LeaseExpireTime.Before(read.LeaseExpireTime))
		require.Equal(t, read.Version+1, upgraded.Version)
		require.Equal(t, "reader-a", upgraded.OwnerId)
		require.Equal(t, 1, upgraded.LockCount)
		require.False(t, upgraded.ReadLock)
		fmt.Println("PASS unique reader upgrade rollback")
	})

	t.Run("exact expiry does not write", func(t *testing.T) {
		store, clock := newBranchClock()
		read := openBranchLock(t, store, clock, "depth-exact-read", false, 0, nil)
		require.NoError(t, read.RLock(ctx, "reader-a"))
		before := lockSnap(t, read)
		clock.Set(before.LeaseExpireTime)
		require.ErrorIs(t, read.RLock(ctx, "reader-a"), ErrLeaseExpired)
		requireStable(t, read, before, "reader-a")
		fmt.Println("PASS exact expiry read reentry")

		sharedA := openBranchLock(t, store, clock, "depth-exact-share", false, 0, nil)
		sharedB := openBranchLock(t, store, clock, "depth-exact-share", false, 0, nil)
		require.NoError(t, sharedA.RLock(ctx, "reader-a"))
		require.NoError(t, sharedB.RLock(ctx, "reader-b"))
		shared := lockSnap(t, sharedA)
		clock.Set(shared.LeaseExpireTime)
		require.ErrorIs(t, sharedA.RUnLock(ctx, "reader-a"), ErrLeaseExpired)
		requireStable(t, sharedA, shared, "reader-a", "reader-b")
		fmt.Println("PASS exact expiry read partial unlock")

		exclusive := openBranchLock(t, store, clock, "depth-exact-ex", false, 0, nil)
		require.NoError(t, exclusive.Lock(ctx, "owner"))
		require.NoError(t, exclusive.Lock(ctx, "owner"))
		held := lockSnap(t, exclusive)
		require.Greater(t, held.LockCount, 1)
		clock.Set(held.LeaseExpireTime)
		require.ErrorIs(t, exclusive.UnLock(ctx, "owner"), ErrLeaseExpired)
		requireStable(t, exclusive, held)
		fmt.Println("PASS exact expiry exclusive partial unlock")

		only := openBranchLock(t, store, clock, "depth-exact-up", false, 0, nil)
		require.NoError(t, only.RLock(ctx, "reader-a"))
		readHeld := lockSnap(t, only)
		clock.Set(readHeld.LeaseExpireTime)
		require.ErrorIs(t, only.TryUpgrade(ctx, "reader-a"), ErrLeaseExpired)
		requireStable(t, only, readHeld, "reader-a")
		require.True(t, lockSnap(t, only).ReadLock)
		fmt.Println("PASS exact expiry upgrade")

		sole := openBranchLock(t, store, clock, "depth-exact-sole", false, 0, nil)
		require.NoError(t, sole.RLock(ctx, "reader-a"))
		soleHeld := lockSnap(t, sole)
		require.Equal(t, 1, soleHeld.LockCount)
		require.Equal(t, "reader-a", soleHeld.OwnerId)
		require.True(t, soleHeld.ReadLock)
		clock.Set(soleHeld.LeaseExpireTime)
		require.ErrorIs(t, sole.RUnLock(ctx, "reader-a"), ErrLeaseExpired)
		requireStable(t, sole, soleHeld, "reader-a")
		fmt.Println("PASS exact expiry unique reader unlock")
	})

	t.Run("time source failure does not write", func(t *testing.T) {
		store := newMockSubscribeStorage()
		clock := &storedClock{mockSubscribeTimeProvider: newMockSubscribeTimeProvider(time.Now())}
		read := openStoredLock(t, store, clock, "depth-clock-read", false, nil)
		other := openStoredLock(t, store, clock, "depth-clock-read", false, nil)
		require.NoError(t, read.RLock(ctx, "reader-a"))
		require.NoError(t, other.RLock(ctx, "reader-b"))
		shared := lockSnap(t, read)
		clock.fail.Store(true)
		require.ErrorIs(t, read.RLock(ctx, "reader-a"), errStoredClock)
		require.ErrorIs(t, read.RUnLock(ctx, "reader-a"), errStoredClock)
		got := storedSnap(t, store, "depth-clock-read")
		require.Equal(t, shared.Version, got.Version)
		require.True(t, shared.LeaseExpireTime.Equal(got.LeaseExpireTime))
		require.Equal(t, shared.LockCount, got.LockCount)
		require.Equal(t, shared.OwnerId, got.OwnerId)
		require.Equal(t, shared.ReadLock, got.ReadLock)
		require.Equal(t, shared.ReadDepth("reader-a"), got.ReadDepth("reader-a"))
		require.Equal(t, shared.ReadDepth("reader-b"), got.ReadDepth("reader-b"))
		fmt.Println("PASS time failure read reentry and partial unlock")

		exclusive := openStoredLock(t, store, clock, "depth-clock-ex", false, nil)
		clock.fail.Store(false)
		require.NoError(t, exclusive.Lock(ctx, "owner"))
		require.NoError(t, exclusive.Lock(ctx, "owner"))
		held := lockSnap(t, exclusive)
		clock.fail.Store(true)
		require.ErrorIs(t, exclusive.UnLock(ctx, "owner"), errStoredClock)
		got = storedSnap(t, store, "depth-clock-ex")
		require.Equal(t, held.Version, got.Version)
		require.True(t, held.LeaseExpireTime.Equal(got.LeaseExpireTime))
		require.Equal(t, held.LockCount, got.LockCount)
		require.Equal(t, held.OwnerId, got.OwnerId)
		require.False(t, got.ReadLock)
		fmt.Println("PASS time failure exclusive partial unlock")

		only := openStoredLock(t, store, clock, "depth-clock-up", false, nil)
		clock.fail.Store(false)
		require.NoError(t, only.RLock(ctx, "reader-a"))
		readHeld := lockSnap(t, only)
		clock.fail.Store(true)
		require.ErrorIs(t, only.TryUpgrade(ctx, "reader-a"), errStoredClock)
		got = storedSnap(t, store, "depth-clock-up")
		require.Equal(t, readHeld.Version, got.Version)
		require.True(t, readHeld.LeaseExpireTime.Equal(got.LeaseExpireTime))
		require.Equal(t, readHeld.LockCount, got.LockCount)
		require.True(t, got.ReadLock)
		require.Equal(t, 1, got.ReadDepth("reader-a"))
		fmt.Println("PASS time failure upgrade")

		sole := openStoredLock(t, store, clock, "depth-clock-sole", false, nil)
		clock.fail.Store(false)
		require.NoError(t, sole.RLock(ctx, "reader-a"))
		soleHeld := lockSnap(t, sole)
		require.Equal(t, 1, soleHeld.LockCount)
		clock.fail.Store(true)
		require.ErrorIs(t, sole.RUnLock(ctx, "reader-a"), errStoredClock)
		got = storedSnap(t, store, "depth-clock-sole")
		require.Equal(t, soleHeld.Version, got.Version)
		require.True(t, soleHeld.LeaseExpireTime.Equal(got.LeaseExpireTime))
		require.Equal(t, soleHeld.LockCount, got.LockCount)
		require.Equal(t, soleHeld.OwnerId, got.OwnerId)
		require.True(t, got.ReadLock)
		require.Equal(t, 1, got.ReadDepth("reader-a"))
		fmt.Println("PASS time failure unique reader unlock")
	})

	t.Run("exact expiry partial unlock keeps the watchdog", func(t *testing.T) {
		store, clock := newBranchClock()
		var lost atomic.Bool
		first := openBranchLock(t, store, clock, "depth-dog", true, 0, &lost)
		second := openBranchLock(t, store, clock, "depth-dog", false, 0, nil)
		require.NoError(t, first.RLock(ctx, "reader-a"))
		require.NoError(t, second.RLock(ctx, "reader-b"))
		before := lockSnap(t, first)
		clock.Set(before.LeaseExpireTime)
		require.ErrorIs(t, first.RUnLock(ctx, "reader-a"), ErrLeaseExpired)
		clock.Set(before.LeaseExpireTime.Add(-time.Minute))
		held := lockSnap(t, first)
		require.True(t, held.LeaseExpireTime.Equal(before.LeaseExpireTime))
		require.Equal(t, before.ReadDepth("reader-a"), held.ReadDepth("reader-a"))
		require.Equal(t, before.ReadDepth("reader-b"), held.ReadDepth("reader-b"))
		require.Equal(t, before.LockCount, held.LockCount)
		require.Equal(t, before.ReadLock, held.ReadLock)
		require.False(t, lost.Load())
		require.Eventually(t, func() bool {
			status, err := first.Inspect(ctx)
			if err != nil || lost.Load() {
				return false
			}
			info := status.Information
			return info.Version > held.Version && !info.LeaseExpireTime.Before(held.LeaseExpireTime) &&
				info.ReadDepth("reader-a") == held.ReadDepth("reader-a") &&
				info.ReadDepth("reader-b") == held.ReadDepth("reader-b")
		}, 5*time.Second, 20*time.Millisecond)
		require.False(t, lost.Load())
		fmt.Println("PASS exact expiry read partial unlock keeps watchdog")
	})

	t.Run("exact expiry unique reader unlock keeps the watchdog", func(t *testing.T) {
		store, clock := newBranchClock()
		var lost atomic.Bool
		only := openBranchLock(t, store, clock, "depth-dog-sole", true, 0, &lost)
		require.NoError(t, only.RLock(ctx, "reader-a"))
		before := lockSnap(t, only)
		require.Equal(t, 1, before.LockCount)
		clock.Set(before.LeaseExpireTime)
		require.ErrorIs(t, only.RUnLock(ctx, "reader-a"), ErrLeaseExpired)
		clock.Set(before.LeaseExpireTime.Add(-time.Minute))
		held := lockSnap(t, only)
		require.True(t, held.LeaseExpireTime.Equal(before.LeaseExpireTime))
		require.Equal(t, before.Version, held.Version)
		require.Equal(t, before.OwnerId, held.OwnerId)
		require.Equal(t, before.LockCount, held.LockCount)
		require.Equal(t, before.ReadLock, held.ReadLock)
		require.Equal(t, before.ReadDepth("reader-a"), held.ReadDepth("reader-a"))
		require.False(t, lost.Load())
		require.Eventually(t, func() bool {
			status, err := only.Inspect(ctx)
			if err != nil || lost.Load() {
				return false
			}
			info := status.Information
			return info.Version > held.Version && !info.LeaseExpireTime.Before(held.LeaseExpireTime) &&
				info.LockCount == held.LockCount && info.ReadLock &&
				info.OwnerId == held.OwnerId &&
				info.ReadDepth("reader-a") == held.ReadDepth("reader-a")
		}, 5*time.Second, 20*time.Millisecond)
		require.False(t, lost.Load())
		fmt.Println("PASS exact expiry unique reader unlock keeps watchdog")
	})
}
