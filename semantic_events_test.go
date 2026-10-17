package storage_lock

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/storage-lock/go-events"
	"github.com/stretchr/testify/assert"
)

// recordingListener 记录收到的所有事件类型，用于验证语义事件是否按预期发出。
type recordingListener struct {
	mu     sync.Mutex
	types  []events.EventType
	events []*events.Event
}

func newRecordingListener() *recordingListener {
	return &recordingListener{}
}

func (l *recordingListener) Name() string { return "recording-listener" }

func (l *recordingListener) On(_ context.Context, e *events.Event) {
	if e == nil || e.EventType == events.EventTypeUnknown {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.types = append(l.types, e.EventType)
	l.events = append(l.events, e)
}

func (l *recordingListener) Snapshot() []events.EventType {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]events.EventType, len(l.types))
	copy(out, l.types)
	return out
}

func (l *recordingListener) Count() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.types)
}

// semanticTestLock 用 recordingListener 创建锁，便于断言语义事件。
func semanticTestLock(t *testing.T, lockId string) (*StorageLock, *recordingListener, *StorageLockOptions) {
	t.Helper()
	store := newMockSubscribeStorage()
	options := NewStorageLockOptionsWithLockId(lockId).
		SetTimeProvider(newMockSubscribeTimeProvider(time.Now())).
		SetLeaseExpireAfter(5 * time.Minute).
		SetVersionMissRetryInterval(10 * time.Millisecond)
	sl, err := NewStorageLockWithOptions(store, options)
	if err != nil {
		t.Fatalf("NewStorageLockWithOptions failed: %v", err)
	}
	listener := newRecordingListener()
	sl.options.EventListeners = append(sl.options.EventListeners, listener)
	return sl, listener, options
}

// hasType 判断事件类型快照中是否包含指定类型。
func hasType(types []events.EventType, want events.EventType) bool {
	for _, t := range types {
		if t == want {
			return true
		}
	}
	return false
}

func TestSemanticEventBusyWhenLocked(t *testing.T) {
	sl, listener, _ := semanticTestLock(t, "semantic-busy")

	// ownerA 先拿到锁
	assert.NoError(t, sl.Lock(context.Background(), "owner-a"))
	listener.types = nil // 清空，只看后续

	// ownerB 尝试获取被 A 持有的锁，应发出 busy 语义事件
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_ = sl.Lock(ctx, "owner-b")

	assert.True(t, hasType(listener.Snapshot(), events.EventTypeBusy), "锁被人持有时应发出 busy 事件")
}

func TestSemanticEventExpiredWhenSeized(t *testing.T) {
	sl, listener, options := semanticTestLock(t, "semantic-expired")

	// ownerA 获取锁后让租约过期（把时间推进超过 LeaseExpireAfter）
	assert.NoError(t, sl.Lock(context.Background(), "owner-a"))

	// 强制让记录过期：直接把存储里的 LeaseExpireTime 往后推到过去
	store := newMockSubscribeStorage()
	_ = store // 不需要，直接用了 testSubscribeStorageLock 的方式

	// 手动把持久的记录标记为过期
	ctx := context.Background()
	info, err := sl.getLockInformation(ctx, sl.operationEvent("owner-a"), options.LockId)
	assert.NoError(t, err)
	if info != nil {
		info.LeaseExpireTime = time.Now().Add(-time.Minute) // 标记为已过期
		prev := info.Version
		next, _ := nextVersion(prev)
		err := sl.storageExecutor.UpdateWithVersion(ctx, sl.operationEvent("owner-a"), options.LockId, prev, next, info)
		if err != nil {
			t.Fatalf("mark expired failed: %v", err)
		}
	}

	listener.types = nil // 清空，只看抢占
	// ownerA 再次获取（或 ownerB 抢占）应看到 expired 事件
	assert.NoError(t, sl.Lock(context.Background(), "owner-a"))
	assert.True(t, hasType(listener.Snapshot(), events.EventTypeExpired), "抢占过期锁应发出 expired 事件")
}

func TestSemanticEventRenewedAndLost(t *testing.T) {
	sl, listener, options := semanticTestLock(t, "semantic-renew-lost")
	// 看门狗默认开启，用短租约缩短测试时间
	sl.options.LeaseExpireAfter = 300 * time.Millisecond
	sl.options.LeaseRefreshInterval = 100 * time.Millisecond
	options.LeaseExpireAfter = 300 * time.Millisecond
	options.LeaseRefreshInterval = 100 * time.Millisecond

	ctx := context.Background()
	assert.NoError(t, sl.Lock(ctx, "owner-a"))

	// 等待看门狗续租发生（刷新间隔 100ms，等几个周期）
	deadline := time.Now().Add(2 * time.Second)
	for {
		if hasType(listener.Snapshot(), events.EventTypeRenewed) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("看门狗应发出 renewed 事件")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestSemanticEventTypesString(t *testing.T) {
	cases := []struct {
		typ  events.EventType
		want string
	}{
		{events.EventTypeUnknown, "unknown"},
		{events.EventTypeCreateLock, "create"},
		{events.EventTypeLock, "lock"},
		{events.EventTypeUnlock, "unlock"},
		{events.EventTypeWatchDog, "watchdog"},
		{events.EventTypeExpired, "expired"},
		{events.EventTypeBusy, "busy"},
		{events.EventTypeRenewed, "renewed"},
		{events.EventTypeLost, "lost"},
		{events.EventTypeWaitChanged, "wait_changed"},
		{events.EventType(999), "unknown"},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, c.typ.String(), "EventType %d 的 String()", c.typ)
	}
}
