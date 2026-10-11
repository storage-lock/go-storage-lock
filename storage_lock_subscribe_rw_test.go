package storage_lock

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestStorageLock_RLock_MultipleReaders(t *testing.T) {
	sl, _, _ := testRWStorageLock(t, "rw-multi-reader")
	ctx := context.Background()

	err := sl.RLock(ctx, "reader-1")
	if !assert.NoError(t, err) {
		return
	}
	defer sl.RUnLock(ctx, "reader-1")

	err = sl.RLock(ctx, "reader-2")
	if !assert.NoError(t, err) {
		return
	}
	defer sl.RUnLock(ctx, "reader-2")

	// 同一把写锁互相排斥
	writeCtx, writeCancel := context.WithTimeout(ctx, time.Second)
	defer writeCancel()
	err = sl.WLock(writeCtx, "writer-1")
	if !assert.Error(t, err) {
		sl.WUnLock(ctx, "writer-1")
	}

	err = sl.RUnLock(ctx, "reader-1")
	assert.NoError(t, err)
	err = sl.RUnLock(ctx, "reader-2")
	assert.NoError(t, err)

	writeCtx, writeCancel = context.WithTimeout(ctx, time.Second)
	defer writeCancel()
	err = sl.WLock(writeCtx, "writer-1")
	assert.NoError(t, err)
	sl.WUnLock(ctx, "writer-1")
}

func TestStorageLock_RLock_WriterBlocksReader(t *testing.T) {
	sl, _, _ := testRWStorageLock(t, "rw-writer-block")
	ctx := context.Background()

	err := sl.WLock(ctx, "owner-w")
	if !assert.NoError(t, err) {
		return
	}
	defer sl.WUnLock(ctx, "owner-w")

	readCtx, readCancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer readCancel()
	err = sl.RLock(readCtx, "reader-blocked")
	assert.Error(t, err)
}

func TestStorageLock_RLock_ReaderBlocksWriter(t *testing.T) {
	sl, _, _ := testRWStorageLock(t, "rw-reader-block")
	ctx := context.Background()

	err := sl.RLock(ctx, "owner-r")
	if !assert.NoError(t, err) {
		return
	}
	defer sl.RUnLock(ctx, "owner-r")

	writeCtx, writeCancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer writeCancel()
	err = sl.WLock(writeCtx, "writer-blocked")
	assert.Error(t, err)
}

func TestStorageLock_RLock_ReentrantSameOwner(t *testing.T) {
	sl, _, _ := testRWStorageLock(t, "rw-reentrant")
	ctx := context.Background()

	err := sl.RLock(ctx, "same-owner")
	if !assert.NoError(t, err) {
		return
	}

	err = sl.RLock(ctx, "same-owner")
	if !assert.NoError(t, err) {
		sl.RUnLock(ctx, "same-owner")
		return
	}

	err = sl.RUnLock(ctx, "same-owner")
	assert.NoError(t, err)
	err = sl.RUnLock(ctx, "same-owner")
	assert.NoError(t, err)
}

func TestStorageLock_RLock_SubscribeReceivesLockState(t *testing.T) {
	sl, _, _ := testRWStorageLock(t, "rw-sub")
	ctx := context.Background()
	ch, err := sl.Subscribe(ctx, "rw-sub")
	if !assert.NoError(t, err) {
		return
	}
	defer sl.Unsubscribe("rw-sub", ch)

	err = sl.RLock(ctx, "sub-owner")
	if !assert.NoError(t, err) {
		return
	}

DrainRLock:
	for {
		select {
		case info := <-ch:
			assert.Equal(t, "rw-sub", info.LockId)
			assert.Equal(t, "sub-owner", info.OwnerId)
			assert.True(t, info.ReadLock)
			break DrainRLock
		case <-time.After(time.Second):
			t.Fatal("subscribe rlock notification timeout")
		}
	}

	err = sl.RUnLock(ctx, "sub-owner")
	assert.NoError(t, err)
}

func TestStorageLock_WLock_Alias(t *testing.T) {
	sl, _, _ := testRWStorageLock(t, "rw-wlock-alias")
	ctx := context.Background()

	err := sl.WLock(ctx, "alias-owner")
	if !assert.NoError(t, err) {
		return
	}

	err = sl.WUnLock(ctx, "alias-owner")
	assert.NoError(t, err)
}

func TestStorageLock_RLock_ErrorPropagation(t *testing.T) {
	sl, _, _ := testRWStorageLock(t, "rw-error")
	ctx := context.Background()

	// invalid unlock should return error
	err := sl.RUnLock(ctx, "no-owner")
	assert.Error(t, err)

	err = sl.WUnLock(ctx, "no-owner")
	assert.Error(t, err)
}
