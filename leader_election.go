package storage_lock

import (
	"context"
	"fmt"
	"sync"

	go_storage "github.com/storage-lock/go-storage"
)

// LeaderElection 领导者选举，基于 StorageLock 实现。
// 多个参与者竞争同一把锁，持有锁者成为 leader；通过 Subscribe 观察 leader 变化。
type LeaderElection struct {
	lock          *StorageLock
	participantId string

	// leaderCh 推送 leader 变化通知（participantId）
	leaderCh chan string

	// isLeader 标记自己是否是当前 leader
	isLeader bool

	mu sync.RWMutex

	// 订阅 channel，用于观察锁状态变化
	subCh <-chan *go_storage.LockInformation

	// 取消订阅的函数
	unsubscribe func()
	// forwardWG 等待订阅转发协程退出，避免 Close 返回后后台协程仍访问实例状态。
	forwardWG sync.WaitGroup
	closed    chan struct{}
	closeOnce sync.Once
}

// NewLeaderElection 创建领导者选举实例。
// electionId: 选举的唯一标识，所有参与者使用相同的 electionId 竞争 leader
// participantId: 参与者唯一标识，必须全局唯一（如 UUID、节点 ID）
func NewLeaderElection(storage go_storage.Storage, electionId string, participantId string) (*LeaderElection, error) {
	if storage == nil {
		return nil, ErrStorageNil
	}
	if electionId == "" {
		return nil, fmt.Errorf("electionId is empty")
	}
	if participantId == "" {
		return nil, fmt.Errorf("participantId is empty")
	}

	lock, err := NewStorageLock(storage, electionId)
	if err != nil {
		return nil, err
	}

	return &LeaderElection{
		lock:          lock,
		participantId: participantId,
		leaderCh:      make(chan string, 16),
		closed:        make(chan struct{}),
	}, nil
}

// Campaign 参与竞选，阻塞直到成为 leader 或 ctx 取消。
// 成为 leader 后会自动通过 Observe() channel 通知。
func (le *LeaderElection) Campaign(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	// 确保已订阅 leader 变化
	le.ensureSubscribed()

	// 尝试获取锁，成功即成为 leader
	err := le.lock.Lock(ctx, le.participantId)
	if err != nil {
		return err
	}

	le.mu.Lock()
	le.isLeader = true
	le.mu.Unlock()

	// 通知自己成为 leader
	select {
	case le.leaderCh <- le.participantId:
	default:
	}

	return nil
}

// Resign 主动让位，释放 leader 身份。
// 让位后其他参与者会通过 Observe() 收到新的 leader 通知。
func (le *LeaderElection) Resign(ctx context.Context) error {
	le.mu.Lock()
	defer le.mu.Unlock()

	if !le.isLeader {
		return nil // 自己不是 leader，无需让位
	}

	err := le.lock.UnLock(ctx, le.participantId)
	if err != nil {
		return err
	}

	le.isLeader = false
	return nil
}

// Leader 返回当前 leader 的 participantId。
// 如果当前没有 leader，返回 ErrLockNotFound。
func (le *LeaderElection) Leader(ctx context.Context) (string, error) {
	owner, err := le.lock.GetOwner(ctx)
	if err != nil {
		return "", err
	}
	return owner, nil
}

// Observe 返回 leader 变化的只读 channel。
// 每次 leader 变化时（新 leader 当选、老 leader 让位或过期），会推送新的 leader participantId。
// 调用方负责读取或超时退出。
func (le *LeaderElection) Observe() <-chan string {
	le.ensureSubscribed()
	return le.leaderCh
}

// IsLeader 判断自己是否是当前 leader。
func (le *LeaderElection) IsLeader() bool {
	le.mu.RLock()
	defer le.mu.RUnlock()
	return le.isLeader
}

// ensureSubscribed 确保已订阅锁状态变化（惰性初始化）。
func (le *LeaderElection) ensureSubscribed() {
	le.mu.Lock()
	defer le.mu.Unlock()

	if le.subCh != nil {
		return // 已经订阅
	}

	ch, err := le.lock.Subscribe(context.Background(), le.lock.options.LockId)
	if err != nil {
		// 订阅失败，静默忽略（不影响 Campaign 的核心功能）
		return
	}

	le.subCh = ch
	le.unsubscribe = func() {
		le.lock.Unsubscribe(le.lock.options.LockId, ch)
	}

	// 启动后台协程转发 leader 变化
	le.forwardWG.Add(1)
	go le.forwardLeaderChanges()
}

// forwardLeaderChanges 消费订阅 channel，提取 leader 变化并转发到 leaderCh。
func (le *LeaderElection) forwardLeaderChanges() {
	defer le.forwardWG.Done()
	le.mu.RLock()
	subCh := le.subCh
	closed := le.closed
	le.mu.RUnlock()
	for {
		select {
		case <-closed:
			return
		case info, ok := <-subCh:
			if !ok {
				return
			}
			if info == nil {
				continue
			}
			le.forwardLeaderInfo(info)
		}
	}
}

func (le *LeaderElection) forwardLeaderInfo(info *go_storage.LockInformation) {
	if info == nil {
		return
	}
	// 只有当 LockCount > 0 时才有有效 leader
	if info.LockCount > 0 {
		select {
		case le.leaderCh <- info.OwnerId:
		default:
		}
	} else {
		select {
		case le.leaderCh <- "":
		default:
		}
	}
}

// Close 关闭选举实例，释放资源。
func (le *LeaderElection) Close() error {
	le.closeOnce.Do(func() { close(le.closed) })
	le.mu.Lock()
	if le.unsubscribe != nil {
		le.unsubscribe()
		le.unsubscribe = nil
	}
	le.subCh = nil
	wasLeader := le.isLeader
	le.isLeader = false
	le.mu.Unlock()

	// Unsubscribe 会关闭 subCh；等待转发协程退出时不能持有 le.mu，
	// 否则转发协程可能正等待 RLock 来复制 channel，形成死锁。
	le.forwardWG.Wait()

	if wasLeader {
		_ = le.lock.UnLock(context.Background(), le.participantId)
	}
	return nil
}
