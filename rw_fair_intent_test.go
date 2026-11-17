package storage_lock

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLastReaderReleaseKeepsWriteIntent(t *testing.T) {
	lock, store, clock := staticRWLock(t, "release-intent")
	ctx := context.Background()
	require.NoError(t, lock.RLock(ctx, "reader"))
	require.NoError(t, lock.prepareWrite(ctx, "writer"))

	require.NoError(t, lock.RUnLock(ctx, "reader"))
	released, err := lock.Inspect(ctx)
	require.NoError(t, err)
	require.True(t, released.WriteIntentOwner != "" || len(released.Waiters) > 0)
	version := released.Information.Version
	owner := released.Information.OwnerId

	require.Error(t, lock.TryRLock(ctx, "late-reader"))
	afterReader, err := lock.Inspect(ctx)
	require.NoError(t, err)
	require.Equal(t, version, afterReader.Information.Version)
	require.Equal(t, owner, afterReader.Information.OwnerId)

	fair := fairLock(t, store, clock, "release-intent")
	require.ErrorIs(t, fair.TryLock(ctx, "cutter"), ErrNotYourTurn)
	afterFair, err := lock.Inspect(ctx)
	require.NoError(t, err)
	require.Equal(t, version, afterFair.Information.Version)
	require.Equal(t, owner, afterFair.Information.OwnerId)
	require.Equal(t, released.WriteIntentOwner, afterFair.WriteIntentOwner)
}

func TestDowngradeKeepsWriteIntent(t *testing.T) {
	lock, store, clock := staticRWLock(t, "downgrade-intent")
	ctx := context.Background()
	require.NoError(t, lock.Lock(ctx, "holder"))
	require.NoError(t, lock.prepareWrite(ctx, "writer"))
	require.NoError(t, lock.Downgrade(ctx, "holder"))

	downgraded, err := lock.Inspect(ctx)
	require.NoError(t, err)
	require.True(t, downgraded.Information.ReadLock)
	require.True(t, downgraded.WriteIntentOwner != "" || len(downgraded.Waiters) > 0)
	require.Error(t, lock.TryRLock(ctx, "late-reader"))

	clock.Advance(lock.options.LeaseExpireAfter + time.Second)
	before, err := lock.Inspect(ctx)
	require.NoError(t, err)
	fair := fairLock(t, store, clock, "downgrade-intent")
	require.ErrorIs(t, fair.TryLock(ctx, "cutter"), ErrNotYourTurn)
	after, err := lock.Inspect(ctx)
	require.NoError(t, err)
	require.Equal(t, before.Information.Version, after.Information.Version)
	require.Equal(t, before.Information.OwnerId, after.Information.OwnerId)
	require.Equal(t, before.WriteIntentOwner, after.WriteIntentOwner)
}

func TestUpgradeKeepsWriteIntent(t *testing.T) {
	lock, store, clock := staticRWLock(t, "upgrade-intent")
	ctx := context.Background()
	require.NoError(t, lock.RLock(ctx, "reader"))
	require.NoError(t, lock.prepareWrite(ctx, "writer"))
	require.NoError(t, lock.TryUpgrade(ctx, "reader"))

	upgraded, err := lock.Inspect(ctx)
	require.NoError(t, err)
	require.False(t, upgraded.Information.ReadLock)
	require.Equal(t, "reader", upgraded.Information.OwnerId)
	require.True(t, upgraded.WriteIntentOwner != "" || len(upgraded.Waiters) > 0)
	require.Error(t, lock.TryRLock(ctx, "late-reader"))

	clock.Advance(lock.options.LeaseExpireAfter + time.Second)
	before, err := lock.Inspect(ctx)
	require.NoError(t, err)
	fair := fairLock(t, store, clock, "upgrade-intent")
	require.ErrorIs(t, fair.TryLock(ctx, "cutter"), ErrNotYourTurn)
	after, err := lock.Inspect(ctx)
	require.NoError(t, err)
	require.Equal(t, before.Information.Version, after.Information.Version)
	require.Equal(t, before.Information.OwnerId, after.Information.OwnerId)
	require.Equal(t, before.WriteIntentOwner, after.WriteIntentOwner)
}

func staticRWLock(t *testing.T, lockID string) (*StorageLock, *mockSubscribeStorage, *mockSubscribeTimeProvider) {
	t.Helper()
	store := newMockSubscribeStorage()
	clock := newMockSubscribeTimeProvider(time.Now())
	options := NewStorageLockOptionsWithLockId(lockID).
		SetTimeProvider(clock).
		SetLeaseExpireAfter(5*time.Minute).
		SetDisableWatchDog(true).
		SetVersionMissRetryInterval(5 * time.Millisecond)
	lock, err := NewStorageLockWithOptions(store, options)
	require.NoError(t, err)
	return lock, store, clock
}
