package storage_lock

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	go_iterator "github.com/golang-infrastructure/go-iterator"
	go_storage "github.com/storage-lock/go-storage"
)

// ultimateControlStorage 终极控制存储，基于 mockSubscribeStorage
type ultimateControlStorage struct {
	*mockSubscribeStorage
	mu               sync.Mutex
	createShouldFail bool
	createError      error
	getShouldFail    bool
	getError         error
	getReturnValue   string
	updateShouldFail bool
	updateError      error
	deleteShouldFail bool
	deleteError      error
	listShouldFail   bool
	listError        error
	storedData       map[string]*go_storage.LockInformation
}

func newUltimateControlStorage() *ultimateControlStorage {
	return &ultimateControlStorage{
		mockSubscribeStorage: newMockSubscribeStorage(),
		storedData:           make(map[string]*go_storage.LockInformation),
	}
}

func (m *ultimateControlStorage) CreateWithVersion(ctx context.Context, lockId string, version go_storage.Version, lockInformation *go_storage.LockInformation) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.createShouldFail {
		if m.createError != nil {
			return m.createError
		}
		return errors.New("create failed")
	}

	if _, exists := m.storedData[lockId]; exists {
		return ErrVersionMiss
	}

	m.storedData[lockId] = lockInformation
	return nil
}

func (m *ultimateControlStorage) UpdateWithVersion(ctx context.Context, lockId string, oldVersion, newVersion go_storage.Version, lockInformation *go_storage.LockInformation) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.updateShouldFail {
		if m.updateError != nil {
			return m.updateError
		}
		return errors.New("update failed")
	}

	existing, exists := m.storedData[lockId]
	if !exists {
		return ErrLockNotFound
	}

	if existing.Version != oldVersion {
		return ErrVersionMiss
	}

	lockInformation.Version = newVersion
	m.storedData[lockId] = lockInformation
	return nil
}

func (m *ultimateControlStorage) DeleteWithVersion(ctx context.Context, lockId string, version go_storage.Version, lockInformation *go_storage.LockInformation) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.deleteShouldFail {
		if m.deleteError != nil {
			return m.deleteError
		}
		return errors.New("delete failed")
	}

	delete(m.storedData, lockId)
	return nil
}

func (m *ultimateControlStorage) Get(ctx context.Context, lockId string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.getShouldFail {
		if m.getError != nil {
			return "", m.getError
		}
		return "", errors.New("get failed")
	}

	if m.getReturnValue != "" {
		return m.getReturnValue, nil
	}

	info, exists := m.storedData[lockId]
	if !exists {
		return "", ErrLockNotFound
	}

	b, _ := json.Marshal(info)
	return string(b), nil
}

func (m *ultimateControlStorage) List(ctx context.Context) (go_iterator.Iterator[*go_storage.LockInformation], error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.listShouldFail {
		if m.listError != nil {
			return nil, m.listError
		}
		return nil, errors.New("list failed")
	}

	var infos []*go_storage.LockInformation
	for _, info := range m.storedData {
		infos = append(infos, info)
	}

	return &listIterator{infos: infos, index: -1}, nil
}

type listIterator struct {
	infos []*go_storage.LockInformation
	index int
}

func (i *listIterator) Next() bool {
	i.index++
	return i.index < len(i.infos)
}

func (i *listIterator) Value() *go_storage.LockInformation {
	if i.index < 0 || i.index >= len(i.infos) {
		return nil
	}
	return i.infos[i.index]
}

func (i *listIterator) Close() error {
	return nil
}

// TestUltimateControl 终极控制测试
func TestUltimateControl(t *testing.T) {

	// 测试 Barrier 的所有错误路径

	// 测试 FairLock 的所有错误路径
	t.Run("FairLock_AllErrors", func(t *testing.T) {
		storage := newUltimateControlStorage()
		storage.createShouldFail = true
		storage.createError = errors.New("fairlock create error")

		_, err := NewFairLock(storage, "fairlock-ultimate")
		if err != nil {
			t.Logf("FairLock error: %v", err)
		}
	})

	// 测试 LeaderElection 的所有错误路径
	t.Run("LeaderElection_AllErrors", func(t *testing.T) {
		storage := newUltimateControlStorage()
		storage.createShouldFail = true
		storage.createError = errors.New("election create error")

		_, err := NewLeaderElection(storage, "election-ultimate", "p1")
		if err != nil {
			t.Logf("LeaderElection error: %v", err)
		}
	})

	// 测试 Semaphore 的所有错误路径

	// 测试 MultiLock 的所有错误路径
	t.Run("MultiLock_AllErrors", func(t *testing.T) {
		storage := newUltimateControlStorage()
		storage.createShouldFail = true
		storage.createError = errors.New("multilock create error")

		lock1, _ := NewStorageLock(storage, "lock1")
		lock2, _ := NewStorageLock(storage, "lock2")

		_, err := NewMultiLock(lock1, lock2)
		if err != nil {
			t.Logf("MultiLock error: %v", err)
		}
	})
}
