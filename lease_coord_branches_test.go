package storage_lock

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-infrastructure/go-iterator"
	"github.com/storage-lock/go-storage"
	"github.com/stretchr/testify/require"
)

func TestLeaseCoordBranches(t *testing.T) {
	ctx := context.Background()

	t.Run("inspect reports missing tombstone exact expiry and readers", func(t *testing.T) {
		store, clock := newBranchClock()
		lock := openBranchLock(t, store, clock, "lease-inspect", false, 0, nil)
		_, err := lock.Inspect(ctx)
		require.ErrorIs(t, err, ErrLockNotFound)
		held, err := lock.IsLocked(ctx)
		require.NoError(t, err)
		require.False(t, held)

		require.NoError(t, lock.Lock(ctx, "owner"))
		status, err := lock.Inspect(ctx)
		require.NoError(t, err)
		require.True(t, status.Held)
		require.Greater(t, status.RemainingLease, time.Duration(0))
		held, err = lock.IsLocked(ctx)
		require.NoError(t, err)
		require.True(t, held)
		owner, err := lock.GetOwner(ctx)
		require.NoError(t, err)
		require.Equal(t, "owner", owner)

		expire := status.Information.LeaseExpireTime
		clock.Set(expire)
		exact, err := lock.Inspect(ctx)
		require.NoError(t, err)
		require.False(t, exact.Held)
		require.Equal(t, time.Duration(0), exact.RemainingLease)
		require.True(t, exact.Information.LeaseExpireTime.Equal(expire))
		_, err = lock.GetOwner(ctx)
		require.ErrorIs(t, err, ErrLockNotFound)

		clock.Set(expire.Add(-time.Minute))
		require.NoError(t, lock.UnLock(ctx, "owner"))
		tomb, err := lock.Inspect(ctx)
		require.NoError(t, err)
		require.False(t, tomb.Held)
		require.Equal(t, time.Duration(0), tomb.RemainingLease)
		require.Equal(t, 0, tomb.Information.LockCount)

		read := openBranchLock(t, store, clock, "lease-readers", false, 0, nil)
		other := openBranchLock(t, store, clock, "lease-readers", false, 0, nil)
		require.NoError(t, read.RLock(ctx, "reader-a"))
		require.NoError(t, read.RLock(ctx, "reader-a"))
		require.NoError(t, other.RLock(ctx, "reader-b"))
		shared, err := read.Inspect(ctx)
		require.NoError(t, err)
		require.True(t, shared.Held)
		require.True(t, shared.Information.ReadLock)
		require.Equal(t, 2, shared.Information.ReadDepth("reader-a"))
		require.Equal(t, 1, shared.Information.ReadDepth("reader-b"))
		got, err := read.GetOwner(ctx)
		require.ErrorIs(t, err, ErrLockNotBelongYou)
		require.Empty(t, got)
		require.NotEqual(t, shared.Information.OwnerId, got)
	})

	t.Run("ttl waiters and fencing fail without extending the lease", func(t *testing.T) {
		store, clock := newBranchClock()
		lock := openBranchLock(t, store, clock, "lease-query", false, 0, nil)
		other := openBranchLock(t, store, clock, "lease-query", false, 0, nil)
		require.NoError(t, lock.RLock(ctx, "reader-a"))
		require.NoError(t, other.RLock(ctx, "reader-b"))
		before := lockSnap(t, lock)
		_, err := lock.TTL(ctx, "reader-a")
		require.ErrorIs(t, err, ErrLockNotBelongYou)
		_, err = lock.GetFencingToken(ctx, "reader-b")
		require.ErrorIs(t, err, ErrLockNotBelongYou)
		_, err = lock.Waiters(ctx)
		require.ErrorIs(t, err, ErrLockNotBelongYou)
		requireStable(t, lock, before, "reader-a", "reader-b")

		writer := openBranchLock(t, store, clock, "lease-ex", false, 0, nil)
		require.NoError(t, writer.Lock(ctx, "owner"))
		require.NoError(t, writer.prepareWrite(ctx, "waiter"))
		held := lockSnap(t, writer)
		waiters, err := writer.Waiters(ctx)
		require.NoError(t, err)
		require.Equal(t, "waiter", waiters[0].OwnerId)
		_, err = writer.TTL(ctx, "other")
		require.ErrorIs(t, err, ErrLockNotBelongYou)
		_, err = writer.GetFencingToken(ctx, "other")
		require.ErrorIs(t, err, ErrLockNotBelongYou)
		requireStable(t, writer, held)

		expire := held.LeaseExpireTime
		clock.Set(expire)
		exact := lockSnap(t, writer)
		_, err = writer.TTL(ctx, "owner")
		require.ErrorIs(t, err, ErrLeaseExpired)
		_, err = writer.GetFencingToken(ctx, "owner")
		require.ErrorIs(t, err, ErrLeaseExpired)
		_, err = writer.Waiters(ctx)
		require.ErrorIs(t, err, ErrLeaseExpired)
		requireStable(t, writer, exact)

		clock.Set(expire.Add(-time.Minute))
		require.NoError(t, writer.UnLock(ctx, "owner"))
		released := lockSnap(t, writer)
		_, err = writer.TTL(ctx, "owner")
		require.ErrorIs(t, err, ErrLockNotFound)
		_, err = writer.GetFencingToken(ctx, "owner")
		require.ErrorIs(t, err, ErrLockNotFound)
		_, err = writer.Waiters(ctx)
		require.ErrorIs(t, err, ErrLockNotFound)
		requireStable(t, writer, released)
	})

	t.Run("renew extends exclusive and an earlier reader only", func(t *testing.T) {
		store, clock := newBranchClock()
		lock := openBranchLock(t, store, clock, "lease-renew-ex", false, 0, nil)
		require.NoError(t, lock.Lock(ctx, "owner"))
		require.NoError(t, lock.Lock(ctx, "owner"))
		before := lockSnap(t, lock)
		clock.Advance(time.Second)
		require.NoError(t, lock.Renew(ctx, "owner"))
		after := lockSnap(t, lock)
		require.True(t, after.LeaseExpireTime.After(before.LeaseExpireTime))
		require.Equal(t, before.LockCount, after.LockCount)
		require.Equal(t, before.OwnerId, after.OwnerId)
		require.False(t, after.ReadLock)
		require.Greater(t, after.Version, before.Version)

		read := openBranchLock(t, store, clock, "lease-renew-rd", false, 0, nil)
		second := openBranchLock(t, store, clock, "lease-renew-rd", false, 0, nil)
		require.NoError(t, read.RLock(ctx, "reader-a"))
		require.NoError(t, read.RLock(ctx, "reader-a"))
		require.NoError(t, second.RLock(ctx, "reader-b"))
		shared := lockSnap(t, read)
		require.Equal(t, "reader-b", shared.OwnerId)
		require.NotEqual(t, "reader-a", shared.OwnerId)
		clock.Advance(time.Second)
		require.NoError(t, read.Renew(ctx, "reader-a"))
		renewed := lockSnap(t, read)
		require.True(t, renewed.LeaseExpireTime.After(shared.LeaseExpireTime))
		require.True(t, renewed.ReadLock)
		require.Equal(t, 2, renewed.ReadDepth("reader-a"))
		require.Equal(t, 1, renewed.ReadDepth("reader-b"))
		require.Equal(t, shared.OwnerId, renewed.OwnerId)
		require.Greater(t, renewed.Version, shared.Version)

		require.ErrorIs(t, read.Renew(ctx, "stranger"), ErrLockNotBelongYou)
		requireStable(t, read, renewed, "reader-a", "reader-b")
		clock.Set(renewed.LeaseExpireTime)
		require.ErrorIs(t, read.Renew(ctx, "reader-a"), ErrLeaseExpired)
		requireStable(t, read, renewed, "reader-a", "reader-b")
	})

	t.Run("lock lost on expiry takeover or disappearance", func(t *testing.T) {
		store, clock := newBranchClock()
		var lost atomic.Bool
		lock := openBranchLock(t, store, clock, "lease-lost-expire", true, 0, &lost)
		require.NoError(t, lock.Lock(ctx, "owner"))
		held := lockSnap(t, lock)
		clock.Set(held.LeaseExpireTime.Add(time.Second))
		require.Eventually(t, func() bool { return lost.Load() }, 3*time.Second, 20*time.Millisecond)
		same := lockSnap(t, lock)
		require.Equal(t, "owner", same.OwnerId)
		require.True(t, same.LeaseExpireTime.Equal(held.LeaseExpireTime))

		var taken atomic.Bool
		holder := openBranchLock(t, store, clock, "lease-lost-take", true, 0, &taken)
		require.NoError(t, holder.Lock(ctx, "owner"))
		owned := lockSnap(t, holder)
		clock.Set(owned.LeaseExpireTime.Add(time.Second))
		other := openBranchLock(t, store, clock, "lease-lost-take", false, 0, nil)
		require.NoError(t, other.Lock(ctx, "other"))
		require.Eventually(t, func() bool { return taken.Load() }, 4*time.Second, 20*time.Millisecond)

		var gone atomic.Bool
		doomed := openBranchLock(t, store, clock, "lease-lost-gone", true, 0, &gone)
		require.NoError(t, doomed.Lock(ctx, "owner"))
		info := lockSnap(t, doomed)
		require.NoError(t, store.DeleteWithVersion(ctx, "lease-lost-gone", info.Version, &info))
		require.Eventually(t, func() bool { return gone.Load() }, 4*time.Second, 20*time.Millisecond)

		var quiet atomic.Bool
		normal := openBranchLock(t, store, clock, "lease-lost-quiet", true, 0, &quiet)
		require.NoError(t, normal.Lock(ctx, "owner"))
		token, err := normal.GetFencingToken(ctx, "owner")
		require.NoError(t, err)
		require.NoError(t, normal.UnLock(ctx, "owner"))
		time.Sleep(1200 * time.Millisecond)
		require.False(t, quiet.Load())
		tomb := lockSnap(t, normal)
		require.Equal(t, 0, tomb.LockCount)
		require.Greater(t, tomb.Version, token)

		next := openBranchLock(t, store, clock, "lease-lost-quiet", false, 0, nil)
		require.NoError(t, next.Lock(ctx, "next"))
		again, err := next.GetFencingToken(ctx, "next")
		require.NoError(t, err)
		require.Greater(t, again, tomb.Version)
		require.NotEqual(t, storage.Version(1), again)

		var readLost atomic.Bool
		first := openBranchLock(t, store, clock, "lease-lost-read", true, 0, &readLost)
		second := openBranchLock(t, store, clock, "lease-lost-read", true, 0, nil)
		require.NoError(t, first.RLock(ctx, "reader-a"))
		require.NoError(t, second.RLock(ctx, "reader-b"))
		require.NoError(t, first.RUnLock(ctx, "reader-a"))
		time.Sleep(1200 * time.Millisecond)
		require.False(t, readLost.Load())
		still := lockSnap(t, second)
		require.Greater(t, still.ReadDepth("reader-b"), 0)
		require.Equal(t, 0, still.ReadDepth("reader-a"))
	})

	t.Run("with lock passes the token and always leaves a tombstone", func(t *testing.T) {
		store, clock := newBranchClock()
		lock := openBranchLock(t, store, clock, "with-nil", false, 0, nil)
		require.ErrorIs(t, lock.WithLock(ctx, nil), ErrCallbackNil)
		requireAbsent(t, store, "with-nil")

		okLock := openBranchLock(t, store, clock, "with-ok", false, 0, nil)
		var got storage.Version
		require.NoError(t, okLock.WithLock(ctx, func(_ context.Context, token storage.Version) error {
			got = token
			return nil
		}))
		require.Greater(t, got, storage.Version(0))
		done := lockSnap(t, okLock)
		require.Equal(t, 0, done.LockCount)
		require.Greater(t, done.Version, got)

		bad := openBranchLock(t, store, clock, "with-err", false, 0, nil)
		err := bad.WithLock(ctx, func(context.Context, storage.Version) error {
			return errors.New("biz")
		})
		require.Error(t, err)
		require.Equal(t, 0, lockSnap(t, bad).LockCount)

		panicked := openBranchLock(t, store, clock, "with-panic", false, 0, nil)
		require.Panics(t, func() {
			_ = panicked.WithLock(ctx, func(context.Context, storage.Version) error {
				panic("callback")
			})
		})
		require.Equal(t, 0, lockSnap(t, panicked).LockCount)

		holder := openBranchLock(t, store, clock, "with-busy", false, 0, nil)
		contender := openBranchLock(t, store, clock, "with-busy", false, 0, nil)
		require.NoError(t, holder.Lock(ctx, "holder"))
		before := lockSnap(t, holder)
		called := false
		busyCtx, cancel := context.WithTimeout(ctx, 40*time.Millisecond)
		defer cancel()
		require.Error(t, contender.WithLock(busyCtx, func(context.Context, storage.Version) error {
			called = true
			return nil
		}))
		require.False(t, called)
		requireStable(t, holder, before)
	})

	t.Run("leader election fails closed and resign is a no-op", func(t *testing.T) {
		_, err := NewLeaderElection(nil, "election", "a")
		require.Error(t, err)
		store, clock := newBranchClock()
		_, err = NewLeaderElection(store, "", "a")
		require.Error(t, err)
		_, err = NewLeaderElection(store, "election", "")
		require.Error(t, err)
		requireAbsent(t, store, "election")

		leader, err := NewLeaderElection(store, "election", "a")
		require.NoError(t, err)
		t.Cleanup(func() { _ = leader.Close() })
		canceled, cancel := context.WithCancel(ctx)
		cancel()
		require.Error(t, leader.Campaign(canceled))
		require.False(t, leader.IsLeader())
		requireAbsent(t, store, "election")

		require.NoError(t, leader.Campaign(ctx))
		require.True(t, leader.IsLeader())
		current, err := leader.Leader(ctx)
		require.NoError(t, err)
		require.Equal(t, "a", current)

		other, err := NewLeaderElection(store, "election", "b")
		require.NoError(t, err)
		t.Cleanup(func() { _ = other.Close() })
		short, stop := context.WithTimeout(ctx, 50*time.Millisecond)
		defer stop()
		require.Error(t, other.Campaign(short))
		require.False(t, other.IsLeader())
		held := lockSnap(t, leader.lock)
		require.NoError(t, other.Resign(ctx))
		requireStable(t, leader.lock, held)
		require.Equal(t, "a", mustOwner(t, leader))

		require.NoError(t, leader.Close())
		require.False(t, leader.IsLeader())
		closed := lockSnap(t, leader.lock)
		require.Equal(t, 0, closed.LockCount)
		_, err = leader.Leader(ctx)
		require.ErrorIs(t, err, ErrLockNotFound)
		_ = clock
	})

	t.Run("list keeps prefix held count and rejects nil or list errors", func(t *testing.T) {
		store, clock := newBranchClock()
		app := openBranchLock(t, store, clock, "app/held", false, 0, nil)
		other := openBranchLock(t, store, clock, "other/held", false, 0, nil)
		tomb := openBranchLock(t, store, clock, "app/tomb", false, 0, nil)
		require.NoError(t, app.Lock(ctx, "owner"))
		require.NoError(t, other.Lock(ctx, "owner"))
		require.NoError(t, tomb.Lock(ctx, "owner"))
		require.NoError(t, tomb.UnLock(ctx, "owner"))

		listed, err := ListLocks(ctx, store, "")
		require.NoError(t, err)
		ids := map[string]bool{}
		for listed.Next() {
			ids[listed.Value().LockId] = true
		}
		require.True(t, ids["app/held"])
		require.True(t, ids["other/held"])
		require.True(t, ids["app/tomb"])

		prefixed, err := ListLocks(ctx, store, "app/")
		require.NoError(t, err)
		prefixIDs := map[string]bool{}
		for prefixed.Next() {
			prefixIDs[prefixed.Value().LockId] = true
		}
		require.Equal(t, map[string]bool{"app/held": true, "app/tomb": true}, prefixIDs)

		held, err := ListHeldLocks(ctx, store, "")
		require.NoError(t, err)
		heldIDs := map[string]bool{}
		for held.Next() {
			info := held.Value()
			heldIDs[info.LockId] = true
			require.Greater(t, info.LockCount, 0)
		}
		require.Equal(t, map[string]bool{"app/held": true, "other/held": true}, heldIDs)
		count, err := CountLocks(ctx, store, "")
		require.NoError(t, err)
		require.Equal(t, len(heldIDs), count)

		_, err = ListLocks(ctx, nil, "")
		require.Error(t, err)
		_, err = ListHeldLocks(ctx, nil, "")
		require.Error(t, err)
		_, err = CountLocks(ctx, nil, "")
		require.Error(t, err)

		broken := &mockStorageWithList{
			mockSubscribeStorage: store,
			listFunc: func(context.Context) (iterator.Iterator[*storage.LockInformation], error) {
				return nil, errors.New("list failed")
			},
		}
		_, err = ListLocks(ctx, broken, "")
		require.Error(t, err)
		_, err = ListHeldLocks(ctx, broken, "")
		require.Error(t, err)
		_, err = CountLocks(ctx, broken, "")
		require.Error(t, err)
	})
}

func mustOwner(t *testing.T, le *LeaderElection) string {
	t.Helper()
	owner, err := le.Leader(context.Background())
	require.NoError(t, err)
	return owner
}
