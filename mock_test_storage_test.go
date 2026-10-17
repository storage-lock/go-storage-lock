package storage_lock

import (
	"context"
	"encoding/json"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/golang-infrastructure/go-iterator"
	"github.com/storage-lock/go-storage"
)

// mockSubscribeStorage 是一个用于 Subscribe / ReadLock 单元测试的内存存储实现，
// 不依赖任何外部模块，直接在 go-storage-lock 内部提供。
type mockSubscribeStorage struct {
	data map[string]*storage.LockInformation
	mu   sync.Mutex

	// watchSubs 模拟存储介质原生的 Watch/Notify 通道；测试可以通过 PushWatchEvent
	// 直接向这里推送事件，模拟"其它进程修改了存储，本进程通过存储原生能力感知到"，
	// 而不经过本 StorageLock 实例自己的 Lock/RLock/WLock/UnLock 调用路径。
	watchSubs map[string][]chan *storage.LockInformation
}

func newMockSubscribeStorage() *mockSubscribeStorage {
	return &mockSubscribeStorage{
		data:      make(map[string]*storage.LockInformation),
		watchSubs: make(map[string][]chan *storage.LockInformation),
	}
}

// Watch 实现 go_storage.LockWatchable，供测试验证 StorageLock.Subscribe 是否正确
// 转发存储原生 Watch 通知（跨实例/跨进程场景）。
func (m *mockSubscribeStorage) Watch(ctx context.Context, lockId string) (<-chan *storage.LockInformation, error) {
	ch := make(chan *storage.LockInformation, 4)
	m.mu.Lock()
	m.watchSubs[lockId] = append(m.watchSubs[lockId], ch)
	m.mu.Unlock()

	go func() {
		<-ctx.Done()
		m.mu.Lock()
		defer m.mu.Unlock()
		subs := m.watchSubs[lockId]
		for i, c := range subs {
			if c == ch {
				m.watchSubs[lockId] = append(subs[:i], subs[i+1:]...)
				break
			}
		}
		close(ch)
	}()

	return ch, nil
}

// PushWatchEvent 模拟存储层原生 Watch 观察到的一次外部变更（例如另一个进程通过
// 同一份存储做的 Lock/UnLock），推送给所有当前通过 Watch 订阅了 lockId 的消费者。
func (m *mockSubscribeStorage) PushWatchEvent(lockId string, info *storage.LockInformation) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, ch := range m.watchSubs[lockId] {
		ch <- info
	}
}

func (m *mockSubscribeStorage) Get(ctx context.Context, lockId string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	info, ok := m.data[lockId]
	if !ok {
		return "", ErrLockNotFound
	}
	b, _ := json.Marshal(info)
	return string(b), nil
}

func (m *mockSubscribeStorage) CreateWithVersion(ctx context.Context, lockId string, version storage.Version, information *storage.LockInformation) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.data[lockId]; ok {
		return ErrLockAlreadyExists
	}
	information.Version = version
	m.data[lockId] = information
	return nil
}

func (m *mockSubscribeStorage) UpdateWithVersion(ctx context.Context, lockId string, oldVersion storage.Version, newVersion storage.Version, information *storage.LockInformation) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	existing, ok := m.data[lockId]
	if !ok {
		return ErrLockNotFound
	}
	if existing.Version != oldVersion {
		return ErrVersionMiss
	}
	information.Version = newVersion
	m.data[lockId] = information
	return nil
}

func (m *mockSubscribeStorage) DeleteWithVersion(ctx context.Context, lockId string, version storage.Version, information *storage.LockInformation) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	existing, ok := m.data[lockId]
	if !ok {
		return ErrLockNotFound
	}
	if existing.Version != version {
		return ErrVersionMiss
	}
	delete(m.data, lockId)
	return nil
}

func (m *mockSubscribeStorage) GetName() string {
	return "mock-subscribe-storage"
}

func (m *mockSubscribeStorage) Close(ctx context.Context) error {
	return nil
}

func (m *mockSubscribeStorage) Init(ctx context.Context) error {
	return nil
}

func (m *mockSubscribeStorage) Capabilities() []storage.StorageCapability {
	return []storage.StorageCapability{storage.CapabilityCAS, storage.CapabilityReliableTime, storage.CapabilityAtomicDelete, storage.CapabilityWatch}
}

func (m *mockSubscribeStorage) GetTime(ctx context.Context) (time.Time, error) {
	return time.Now(), nil
}

func (m *mockSubscribeStorage) List(ctx context.Context) (iterator.Iterator[*storage.LockInformation], error) {
	return &emptyLockIterator{}, nil
}

type emptyLockIterator struct{}

func (i *emptyLockIterator) Next() bool                      { return false }
func (i *emptyLockIterator) Value() *storage.LockInformation { return nil }
func (i *emptyLockIterator) Close() error                    { return nil }

// mockSubscribeTimeProvider 为测试提供可控时间源，避免 GetTime 抖动导致误判。
type mockSubscribeTimeProvider struct {
	now time.Time
	mu  sync.Mutex
}

func newMockSubscribeTimeProvider(start time.Time) *mockSubscribeTimeProvider {
	return &mockSubscribeTimeProvider{now: start}
}

func (m *mockSubscribeTimeProvider) GetTime(ctx context.Context) (time.Time, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.now, nil
}

func (m *mockSubscribeTimeProvider) Advance(d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.now = m.now.Add(d)
}
// testSubscribeStorageLock 用 mock 存储创建用于订阅测试的 StorageLock。
func testSubscribeStorageLock(t *testing.T, lockId string) (*StorageLock, *mockSubscribeStorage, *mockSubscribeTimeProvider) {
	t.Helper()
	store := newMockSubscribeStorage()
	start := time.Now()
	timeProvider := newMockSubscribeTimeProvider(start)
	options := NewStorageLockOptionsWithLockId(lockId).
		SetTimeProvider(timeProvider).
		SetLeaseExpireAfter(5 * time.Minute).
		SetVersionMissRetryInterval(10 * time.Millisecond)
	sl, err := NewStorageLockWithOptions(store, options)
	if err != nil {
		t.Fatalf("NewStorageLockWithOptions failed: %v", err)
	}
	return sl, store, timeProvider
}

// testRWStorageLock 用 mock 存储创建用于读写锁测试的 StorageLock。
func testRWStorageLock(t *testing.T, lockId string) (*StorageLock, *mockSubscribeStorage, *mockSubscribeTimeProvider) {
	t.Helper()
	store := newMockSubscribeStorage()
	start := time.Now()
	timeProvider := newMockSubscribeTimeProvider(start)
	options := NewStorageLockOptionsWithLockId(lockId).
		SetTimeProvider(timeProvider).
		SetLeaseExpireAfter(5 * time.Minute).
		SetVersionMissRetryInterval(10 * time.Millisecond)
	sl, err := NewStorageLockWithOptions(store, options)
	if err != nil {
		t.Fatalf("NewStorageLockWithOptions failed: %v", err)
	}
	return sl, store, timeProvider
}

// randomMicroOffset 返回 [50us, 999us] 的随机偏移，与生产实现保持一致。
func randomMicroOffset() time.Duration {
	return time.Duration(rand.Intn(950)+50) * time.Microsecond
}
