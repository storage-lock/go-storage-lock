package storage_lock

import (
	"context"
	"testing"
	"time"

	"github.com/storage-lock/go-storage"
	"github.com/stretchr/testify/require"
)

func TestLocalOrderedLockWaitsForEarlierTicket(t *testing.T) {
	store := newMockSubscribeStorage()
	lock, err := NewLocalOrderedLock(store, "ordered")
	require.NoError(t, err)
	ctx := context.Background()
	require.NoError(t, lock.Lock(ctx, "holder"))

	earlyReady := make(chan struct{})
	lateReady := make(chan struct{})
	releaseEarly := make(chan struct{})
	earlyDone := make(chan error, 1)
	lateDone := make(chan error, 1)

	go func() {
		if err := lock.Lock(ctx, "early"); err != nil {
			earlyDone <- err
			return
		}
		close(earlyReady)
		<-releaseEarly
		earlyDone <- lock.UnLock(ctx, "early")
	}()
	require.Eventually(t, func() bool { return lock.QueueLength() == 1 }, time.Second, 5*time.Millisecond)

	go func() {
		if err := lock.Lock(ctx, "late"); err != nil {
			lateDone <- err
			return
		}
		close(lateReady)
		lateDone <- lock.UnLock(ctx, "late")
	}()
	require.Eventually(t, func() bool { return lock.QueueLength() == 2 }, time.Second, 5*time.Millisecond)

	require.NoError(t, lock.UnLock(ctx, "holder"))
	select {
	case <-earlyReady:
	case <-lateReady:
		t.Fatal("后排队者在先排队者之前拿到了锁")
	case <-time.After(2 * time.Second):
		t.Fatal("先排队者没有拿到锁")
	}
	select {
	case <-lateReady:
		t.Fatal("先排队者还持有时，后排队者已经拿到了锁")
	case <-time.After(100 * time.Millisecond):
	}
	status, err := lock.GetStorageLock().Inspect(ctx)
	require.NoError(t, err)
	require.Equal(t, "early", status.Information.OwnerId)
	require.Equal(t, 1, lock.QueueLength())

	close(releaseEarly)
	select {
	case <-lateReady:
	case <-time.After(2 * time.Second):
		t.Fatal("先排队者释放后，后排队者没有拿到锁")
	}
	require.NoError(t, <-earlyDone)
	require.NoError(t, <-lateDone)
}

func TestFairTryLockReturnsNotYourTurn(t *testing.T) {
	store := newMockSubscribeStorage()
	clock := newMockSubscribeTimeProvider(time.Now())
	const lockID = "fair-try"
	now, err := clock.GetTime(context.Background())
	require.NoError(t, err)
	store.data[lockID] = &storage.LockInformation{
		LockId:          lockID,
		OwnerId:         "previous",
		Version:         7,
		LockCount:       1,
		LeaseExpireTime: now.Add(-time.Second),
		Waiters:         []storage.LockWaiter{{OwnerId: "head", Mode: waiterModeWrite, Since: now}},
	}
	fair := fairLock(t, store, clock, lockID)
	ctx := context.Background()
	before := *store.data[lockID]

	require.ErrorIs(t, fair.TryLock(ctx, "cutter"), ErrNotYourTurn)
	raw, err := store.Get(ctx, lockID)
	require.NoError(t, err)
	after, err := storage.LockInformationFromJsonString(raw)
	require.NoError(t, err)
	require.Equal(t, before.Version, after.Version)
	require.Equal(t, before.OwnerId, after.OwnerId)
}

func TestMultiLockDoesNotCutFairQueue(t *testing.T) {
	store := newBatchCASStorage()
	clock := newMockSubscribeTimeProvider(time.Now())
	ctx := context.Background()
	now, err := clock.GetTime(ctx)
	require.NoError(t, err)
	const queuedID = "queued-lock"
	store.data[queuedID] = &storage.LockInformation{
		LockId:          queuedID,
		OwnerId:         "previous",
		Version:         7,
		LockCount:       0,
		LeaseExpireTime: now.Add(-time.Second),
		Waiters:         []storage.LockWaiter{{OwnerId: "head", Mode: waiterModeWrite, Since: now}},
	}
	options := func(id string) *StorageLockOptions {
		return NewStorageLockOptionsWithLockId(id).
			SetTimeProvider(clock).
			SetLeaseExpireAfter(5 * time.Minute).
			SetDisableWatchDog(true)
	}
	queued, err := NewStorageLockWithOptions(store, options(queuedID))
	require.NoError(t, err)
	other, err := NewStorageLockWithOptions(store, options("fresh-lock"))
	require.NoError(t, err)
	multi, err := NewMultiLock("cutter", queued, other)
	require.NoError(t, err)

	require.ErrorIs(t, multi.Lock(ctx), ErrNotYourTurn)
	raw, err := store.Get(ctx, queuedID)
	require.NoError(t, err)
	after, err := storage.LockInformationFromJsonString(raw)
	require.NoError(t, err)
	require.Equal(t, storage.Version(7), after.Version)
	require.Equal(t, "previous", after.OwnerId)
	_, err = store.Get(ctx, "fresh-lock")
	require.ErrorIs(t, err, ErrLockNotFound)
}

type batchCASStorage struct {
	*mockSubscribeStorage
	failBatches int
}

func newBatchCASStorage() *batchCASStorage {
	return &batchCASStorage{mockSubscribeStorage: newMockSubscribeStorage()}
}

func (b *batchCASStorage) Capabilities() []storage.StorageCapability {
	caps := append([]storage.StorageCapability{}, b.mockSubscribeStorage.Capabilities()...)
	return append(caps, storage.CapabilityAtomicBatch)
}

func (b *batchCASStorage) CompareAndSwapBatch(ctx context.Context, ops []storage.CASOp) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.failBatches > 0 {
		b.failBatches--
		return ErrVersionMiss
	}
	for _, op := range ops {
		existing, ok := b.data[op.LockID]
		if op.ExpectedVersion == 0 {
			if ok {
				return ErrLockAlreadyExists
			}
			continue
		}
		if !ok || existing.Version != op.ExpectedVersion {
			return ErrVersionMiss
		}
	}
	for _, op := range ops {
		info := op.Information
		info.Version = op.NewVersion
		b.data[op.LockID] = info
	}
	return nil
}
