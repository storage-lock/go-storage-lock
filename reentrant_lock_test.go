package storage_lock

import (
	"context"
	"errors"
	go_iterator "github.com/golang-infrastructure/go-iterator"
	go_storage "github.com/storage-lock/go-storage"
	"github.com/stretchr/testify/assert"
	"sync"
	"testing"
	"time"
)

// mockReentrantStorage 用于可重入锁测试的 mock 存储
type mockReentrantStorage struct {
	data    map[string]*go_storage.LockInformation
	watchCh map[string]chan *go_storage.LockInformation
	mu      sync.RWMutex
	closed  bool
	caps    []go_storage.StorageCapability
}

func newMockReentrantStorage() *mockReentrantStorage {
	return &mockReentrantStorage{
		data:    make(map[string]*go_storage.LockInformation),
		watchCh: make(map[string]chan *go_storage.LockInformation),
		caps: []go_storage.StorageCapability{
			go_storage.CapabilityCAS,
			go_storage.CapabilityReliableTime,
			go_storage.CapabilityWatch,
			go_storage.CapabilityAtomicDelete,
		},
	}
}

func (m *mockReentrantStorage) Capabilities() []go_storage.StorageCapability {
	return m.caps
}

func (m *mockReentrantStorage) CreateWithVersion(ctx context.Context, lockId string, version go_storage.Version, info *go_storage.LockInformation) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.data[lockId]; exists {
		return ErrLockAlreadyExists
	}
	m.data[lockId] = info
	return nil
}

func (m *mockReentrantStorage) UpdateWithVersion(ctx context.Context, lockId string, expectedVersion, newVersion go_storage.Version, info *go_storage.LockInformation) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	existing, exists := m.data[lockId]
	if !exists {
		return ErrLockNotFound
	}
	if existing.Version != expectedVersion {
		return ErrVersionMiss
	}
	m.data[lockId] = info
	return nil
}

func (m *mockReentrantStorage) DeleteWithVersion(ctx context.Context, lockId string, expectedVersion go_storage.Version, info *go_storage.LockInformation) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	existing, exists := m.data[lockId]
	if !exists {
		return ErrLockNotFound
	}
	if existing.Version != expectedVersion {
		return ErrVersionMiss
	}
	delete(m.data, lockId)
	return nil
}

func (m *mockReentrantStorage) Get(ctx context.Context, lockId string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	info, exists := m.data[lockId]
	if !exists {
		return "", ErrLockNotFound
	}
	return info.ToJsonString(), nil
}

func (m *mockReentrantStorage) GetTime(ctx context.Context) (time.Time, error) {
	return time.Now(), nil
}

func (m *mockReentrantStorage) List(ctx context.Context) (go_iterator.Iterator[*go_storage.LockInformation], error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	items := make([]*go_storage.LockInformation, 0, len(m.data))
	for _, info := range m.data {
		items = append(items, info)
	}
	return go_iterator.FromSlice(items), nil
}

func (m *mockReentrantStorage) Close(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

func (m *mockReentrantStorage) Init(ctx context.Context) error {
	return nil
}

func (m *mockReentrantStorage) GetName() string {
	return "mock-reentrant-storage"
}

func (m *mockReentrantStorage) Watch(ctx context.Context, lockId string) (<-chan *go_storage.LockInformation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ch, exists := m.watchCh[lockId]
	if !exists {
		ch = make(chan *go_storage.LockInformation, 1)
		m.watchCh[lockId] = ch
	}
	return ch, nil
}

// --------------------------------------------------------------------
// ReentrantLock 测试
// --------------------------------------------------------------------

func TestReentrantLock_SingleReentry(t *testing.T) {
	storage := newMockReentrantStorage()
	lock, err := NewStorageLock(storage, "test-reentrant-single")
	assert.NoError(t, err)

	ctx := context.Background()
	ownerId := "owner-1"

	// 第一次加锁
	err = lock.Lock(ctx, ownerId)
	assert.NoError(t, err)

	infoStr, _ := storage.Get(ctx, "test-reentrant-single")
	info, _ := go_storage.LockInformationFromJsonString(infoStr)
	assert.Equal(t, 1, info.LockCount)

	// 同一 owner 重入
	err = lock.Lock(ctx, ownerId)
	assert.NoError(t, err)

	infoStr, _ = storage.Get(ctx, "test-reentrant-single")
	info, _ = go_storage.LockInformationFromJsonString(infoStr)
	assert.Equal(t, 2, info.LockCount)

	// 第一次释放
	err = lock.UnLock(ctx, ownerId)
	assert.NoError(t, err)

	infoStr, _ = storage.Get(ctx, "test-reentrant-single")
	info, _ = go_storage.LockInformationFromJsonString(infoStr)
	assert.Equal(t, 1, info.LockCount)

	// 第二次释放，锁彻底释放
	err = lock.UnLock(ctx, ownerId)
	assert.NoError(t, err)

	infoStr, err = storage.Get(ctx, "test-reentrant-single")
	assert.NoError(t, err)
	info, err = go_storage.LockInformationFromJsonString(infoStr)
	assert.NoError(t, err)
	assert.Equal(t, 0, info.LockCount)
}

func TestReentrantLock_MultiLevelNesting(t *testing.T) {
	storage := newMockReentrantStorage()
	lock, err := NewStorageLock(storage, "test-reentrant-multi")
	assert.NoError(t, err)

	ctx := context.Background()
	ownerId := "owner-1"

	const depth = 5
	for i := 0; i < depth; i++ {
		err = lock.Lock(ctx, ownerId)
		assert.NoError(t, err, "第 %d 次重入失败", i+1)

		infoStr, _ := storage.Get(ctx, "test-reentrant-multi")
		info, _ := go_storage.LockInformationFromJsonString(infoStr)
		assert.Equal(t, i+1, info.LockCount, "第 %d 次重入后 LockCount 应为 %d", i+1, i+1)
	}

	// 逐层释放
	for i := depth; i > 0; i-- {
		err = lock.UnLock(ctx, ownerId)
		assert.NoError(t, err, "第 %d 次释放失败", i)

		if i > 1 {
			infoStr, _ := storage.Get(ctx, "test-reentrant-multi")
			info, _ := go_storage.LockInformationFromJsonString(infoStr)
			assert.Equal(t, i-1, info.LockCount, "第 %d 次释放后 LockCount 应为 %d", i, i-1)
		} else {
			infoStr, err := storage.Get(ctx, "test-reentrant-multi")
			assert.NoError(t, err)
			info, err := go_storage.LockInformationFromJsonString(infoStr)
			assert.NoError(t, err)
			assert.Equal(t, 0, info.LockCount, "最后一次释放后应写入墓碑记录")
		}
	}
}

func TestReentrantLock_NonOwnerReleaseFailure(t *testing.T) {
	storage := newMockReentrantStorage()
	lock, err := NewStorageLock(storage, "test-reentrant-nonowner")
	assert.NoError(t, err)

	ctx := context.Background()
	ownerId := "owner-1"

	// owner-1 重入两次
	err = lock.Lock(ctx, ownerId)
	assert.NoError(t, err)
	err = lock.Lock(ctx, ownerId)
	assert.NoError(t, err)

	// owner-2 尝试释放，应失败
	err = lock.UnLock(ctx, "owner-2")
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrLockNotBelongYou))

	// owner-1 的锁状态应不变
	infoStr, _ := storage.Get(ctx, "test-reentrant-nonowner")
	info, _ := go_storage.LockInformationFromJsonString(infoStr)
	assert.Equal(t, ownerId, info.OwnerId)
	assert.Equal(t, 2, info.LockCount)
}

func TestReentrantLock_WatchdogInteraction(t *testing.T) {
	storage := newMockReentrantStorage()
	lock, err := NewStorageLock(storage, "test-reentrant-watchdog")
	assert.NoError(t, err)

	ctx := context.Background()
	ownerId := "owner-1"

	// 第一次加锁
	err = lock.Lock(ctx, ownerId)
	assert.NoError(t, err)

	// 等待看门狗启动并续租
	time.Sleep(100 * time.Millisecond)

	// 重入
	err = lock.Lock(ctx, ownerId)
	assert.NoError(t, err)

	// 看门狗应继续运行
	time.Sleep(100 * time.Millisecond)

	infoStr, _ := storage.Get(ctx, "test-reentrant-watchdog")
	info, _ := go_storage.LockInformationFromJsonString(infoStr)
	assert.Equal(t, 2, info.LockCount)
	assert.True(t, info.LeaseExpireTime.After(time.Now()), "租约应已续期")

	// 释放一层
	err = lock.UnLock(ctx, ownerId)
	assert.NoError(t, err)

	// 看门狗应继续运行，锁仍存在
	infoStr, err = storage.Get(ctx, "test-reentrant-watchdog")
	info, err = go_storage.LockInformationFromJsonString(infoStr)
	assert.NoError(t, err)
	assert.Equal(t, 1, info.LockCount)

	// 再释放一层，锁彻底释放
	err = lock.UnLock(ctx, ownerId)
	assert.NoError(t, err)

	// 看门狗应停止，锁应彻底释放
	time.Sleep(100 * time.Millisecond)

	infoStr, err = storage.Get(ctx, "test-reentrant-watchdog")
	assert.NoError(t, err)
	info, err = go_storage.LockInformationFromJsonString(infoStr)
	assert.NoError(t, err)
	assert.Equal(t, 0, info.LockCount)
}
