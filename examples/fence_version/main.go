package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/golang-infrastructure/go-iterator"
	"github.com/storage-lock/go-storage"
	storage_lock "github.com/storage-lock/go-storage-lock"
)

// fenceStore 是这个示例自己的条件写存储。消费者只通过公开的锁接口获取 fencing 版本。
type fenceStore struct {
	mu   sync.Mutex
	data map[string]fenceValue
}

type fenceValue struct {
	version storage.Version
	raw     string
}

func main() {
	ctx := context.Background()
	store := &fenceStore{data: map[string]fenceValue{}}
	options := storage_lock.NewStorageLockOptionsWithLockId("fence-consumer").SetDisableWatchDog(true)
	lock, err := storage_lock.NewStorageLockWithOptions(store, options)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	if err := lock.Lock(ctx, "consumer"); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	token, err := lock.GetFencingToken(ctx, "consumer")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	fmt.Printf("fencing version %d\n", token)
	if token != 1 {
		os.Exit(1)
	}
}

func (s *fenceStore) GetName() string { return "fence-consumer" }

func (s *fenceStore) Capabilities() []storage.StorageCapability {
	return []storage.StorageCapability{storage.CapabilityCAS, storage.CapabilityReliableTime}
}

func (s *fenceStore) Init(context.Context) error { return nil }

func (s *fenceStore) Close(context.Context) error { return nil }

func (s *fenceStore) GetTime(context.Context) (time.Time, error) { return time.Now(), nil }

func (s *fenceStore) Get(_ context.Context, lockID string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.data[lockID]
	if !ok {
		return "", storage_lock.ErrLockNotFound
	}
	return value.raw, nil
}

func (s *fenceStore) CreateWithVersion(_ context.Context, lockID string, version storage.Version, info *storage.LockInformation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.data[lockID]; ok {
		return storage_lock.ErrLockAlreadyExists
	}
	s.data[lockID] = fenceValue{version: version, raw: info.ToJsonString()}
	return nil
}

func (s *fenceStore) UpdateWithVersion(_ context.Context, lockID string, expected, next storage.Version, info *storage.LockInformation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.data[lockID]
	if !ok || value.version != expected {
		return storage_lock.ErrVersionMiss
	}
	s.data[lockID] = fenceValue{version: next, raw: info.ToJsonString()}
	return nil
}

func (s *fenceStore) DeleteWithVersion(_ context.Context, lockID string, expected storage.Version, _ *storage.LockInformation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.data[lockID]
	if !ok {
		return storage_lock.ErrLockNotFound
	}
	if value.version != expected {
		return storage_lock.ErrVersionMiss
	}
	delete(s.data, lockID)
	return nil
}

func (s *fenceStore) List(context.Context) (iterator.Iterator[*storage.LockInformation], error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := make([]*storage.LockInformation, 0, len(s.data))
	for _, value := range s.data {
		info := &storage.LockInformation{}
		if err := json.Unmarshal([]byte(value.raw), info); err != nil {
			return nil, err
		}
		items = append(items, info)
	}
	return iterator.FromSlice(items), nil
}
