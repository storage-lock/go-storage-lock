package storage_lock

import (
	"context"
	"github.com/storage-lock/go-storage"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func TestStorageLock_Subscribe_ReturnsChannel(t *testing.T) {
	sl, _, _ := testSubscribeStorageLock(t, "sub-return-channel")
	ctx := context.Background()
	ch, err := sl.Subscribe(ctx, "sub-return-channel")
	if !assert.NoError(t, err) {
		return
	}
	assert.NotNil(t, ch)
	sl.Unsubscribe("sub-return-channel", ch)
}

func TestStorageLock_Subscribe_EmptyLockId(t *testing.T) {
	sl, _, _ := testSubscribeStorageLock(t, "sub-empty")
	ctx := context.Background()
	ch, err := sl.Subscribe(ctx, "")
	assert.Error(t, err)
	assert.Nil(t, ch)
}

func TestStorageLock_Subscribe_NotifyOnLock(t *testing.T) {
	sl, _, _ := testSubscribeStorageLock(t, "sub-notify")
	ctx := context.Background()
	ch, err := sl.Subscribe(ctx, "sub-notify")
	if !assert.NoError(t, err) {
		return
	}
	defer sl.Unsubscribe("sub-notify", ch)

	done := make(chan *storage.LockInformation, 1)
	go func() {
		info := <-ch
		done <- info
	}()

	err = sl.Lock(ctx, "owner-1")
	if !assert.NoError(t, err) {
		return
	}

	select {
	case info := <-done:
		assert.Equal(t, "sub-notify", info.LockId)
		assert.Equal(t, "owner-1", info.OwnerId)
		assert.True(t, info.ReadLock || info.LockCount > 0, "expected held lock state, got: %+v", info)
	case <-time.After(time.Second):
		t.Fatal("subscribe notification timeout")
	}

	err = sl.UnLock(ctx, "owner-1")
	assert.NoError(t, err)
}

func TestStorageLock_Subscribe_NotifyOnUnlock(t *testing.T) {
	sl, _, _ := testSubscribeStorageLock(t, "sub-unlock")
	ctx := context.Background()
	ch, err := sl.Subscribe(ctx, "sub-unlock")
	if !assert.NoError(t, err) {
		return
	}
	defer sl.Unsubscribe("sub-unlock", ch)

	err = sl.Lock(ctx, "owner-2")
	if !assert.NoError(t, err) {
		return
	}

	// Drain all notifications generated during lock acquisition.
	for {
		select {
		case <-ch:
		case <-time.After(200 * time.Millisecond):
			goto AfterDrain
		}
	}
AfterDrain:

	done := make(chan *storage.LockInformation, 1)
	go func() {
		info := <-ch
		done <- info
	}()

	err = sl.UnLock(ctx, "owner-2")
	if !assert.NoError(t, err) {
		return
	}

	select {
	case info := <-done:
		assert.Equal(t, "sub-unlock", info.LockId)
		assert.Equal(t, "owner-2", info.OwnerId)
		assert.NotNil(t, info, "expected unlock notification")
	case <-time.After(time.Second):
		t.Fatal("subscribe unlock notification timeout")
	}
}

func TestStorageLock_Subscribe_UnsubscribeStopsNotifications(t *testing.T) {
	sl, _, _ := testSubscribeStorageLock(t, "sub-stop")
	ctx := context.Background()
	ch, err := sl.Subscribe(ctx, "sub-stop")
	if !assert.NoError(t, err) {
		return
	}

	// drain possible initial snapshot
	select {
	case <-ch:
	default:
	}

	sl.Unsubscribe("sub-stop", ch)

	// 给订阅 goroutine 留一点时间，避免在读取订阅者列表时发生竞态
	time.Sleep(50 * time.Millisecond)

	err = sl.Lock(ctx, "owner-3")
	if !assert.NoError(t, err) {
		return
	}

	// Unsubscribe 现在会 close(ch)，因此对 ch 的接收会立即返回零值而不是阻塞到超时；
	// 用 ok 区分"channel 已关闭"（预期）和"确实收到了一条通知"（不应该发生）。
	select {
	case info, ok := <-ch:
		if ok {
			t.Fatalf("should not receive notification after unsubscribe, got: %+v", info)
		}
		// ok == false：channel 已被 Unsubscribe 关闭，符合预期
	case <-time.After(300 * time.Millisecond):
		t.Fatal("expected channel to be closed after unsubscribe")
	}

	sl.UnLock(ctx, "owner-3")
}

func TestStorageLock_Subscribe_NilChannel(t *testing.T) {
	sl, _, _ := testSubscribeStorageLock(t, "sub-nil")
	sl.Unsubscribe("sub-nil", nil)
	sl.Unsubscribe("", nil)
}

// TestStorageLock_Subscribe_ForwardsStorageNativeWatch 验证 Subscribe 会把存储层原生
// Watch（LockWatchable）观察到的变更转发给订阅者，即使这个变更不是通过本 StorageLock
// 实例自己的 Lock/RLock/WLock/UnLock 调用产生的——模拟"另一个进程通过同一份存储修改了锁状态"
// 这一 Subscribe 本应支持的跨实例场景（见 read-write-lock-subscribe-implementation 中
// "Subscribe 是跨实例状态观察的必要通道"的设计意图）。
func TestStorageLock_Subscribe_ForwardsStorageNativeWatch(t *testing.T) {
	lockId := "sub-native-watch"
	sl, store, _ := testSubscribeStorageLock(t, lockId)
	ctx := context.Background()

	ch, err := sl.Subscribe(ctx, lockId)
	if !assert.NoError(t, err) {
		return
	}
	defer sl.Unsubscribe(lockId, ch)

	// 给 Subscribe 内部启动的后台转发协程一点时间完成 watchable.Watch(...) 注册，
	// 确保下面 PushWatchEvent 时订阅者已经就位。
	time.Sleep(50 * time.Millisecond)

	external := &storage.LockInformation{
		LockId:  lockId,
		OwnerId: "external-process-owner",
	}
	store.PushWatchEvent(lockId, external)

	select {
	case info := <-ch:
		assert.Equal(t, lockId, info.LockId)
		assert.Equal(t, "external-process-owner", info.OwnerId)
	case <-time.After(time.Second):
		t.Fatal("expected to receive externally-pushed watch event via Subscribe")
	}
}

// TestStorageLock_Subscribe_StopsWatchOnLastUnsubscribe 验证最后一个订阅者取消订阅后，
// 后台的存储原生 Watch 转发协程会停止（通过其内部 ctx 被取消体现：mockSubscribeStorage.Watch
// 返回的 channel 在 ctx.Done() 后会被 close，PushWatchEvent 不会 panic 也不会有消费者残留）。
func TestStorageLock_Subscribe_StopsWatchOnLastUnsubscribe(t *testing.T) {
	lockId := "sub-native-watch-stop"
	sl, store, _ := testSubscribeStorageLock(t, lockId)
	ctx := context.Background()

	ch, err := sl.Subscribe(ctx, lockId)
	if !assert.NoError(t, err) {
		return
	}
	time.Sleep(50 * time.Millisecond)

	sl.Unsubscribe(lockId, ch)
	time.Sleep(50 * time.Millisecond)

	store.mu.Lock()
	remaining := len(store.watchSubs[lockId])
	store.mu.Unlock()
	assert.Equal(t, 0, remaining, "expected storage-level watch subscription to be torn down after last Unsubscribe")
}
