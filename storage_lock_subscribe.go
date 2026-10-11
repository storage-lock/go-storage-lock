package storage_lock

import (
	"context"
	"fmt"

	"github.com/storage-lock/go-events"
	go_storage "github.com/storage-lock/go-storage"
)

// Subscribe 订阅指定锁的状态变化通知。
// 每次调用返回一个专属 channel；调用方负责读取或超时退出。
// 当锁的状态发生变化（创建、获取、释放、过期、被盗）时，当前信息会推入 channel。
func (x *StorageLock) Subscribe(ctx context.Context, lockId string) (<-chan *go_storage.LockInformation, error) {
	if lockId == "" {
		return nil, fmt.Errorf("lockId is empty")
	}

	ch := make(chan *go_storage.LockInformation, 16)

	x.subscribeMu.Lock()
	if x.subscribers == nil {
		x.subscribers = make(map[string][]chan *go_storage.LockInformation)
	}
	isFirstSubscriber := len(x.subscribers[lockId]) == 0
	x.subscribers[lockId] = append(x.subscribers[lockId], ch)
	// 若底层存储实现了 LockWatchable（原生 Watch/Notify 能力，或降级的 PollingLockWatcher），
	// 在该 lockId 的第一个订阅者到来时启动一个后台转发协程，把存储层观察到的变更（可能来自
	// 其它进程/其它 StorageLock 实例）转发给本实例的所有订阅者。此前 Subscribe 只在本实例自己
	// 的 Lock/RLock/WLock/UnLock 等方法内部调用 notifySubscribers，完全不具备跨实例/跨进程的
	// 状态观察能力，与 LockWatchable 接口注释里声明的设计意图（"上层 Lock 可通过类型断言使用存储
	// 原生 Watch 能力"）及本仓库 Subscribe 的既定目标（"跨实例状态观察的必要通道"）不符。
	if watchable, ok := x.storage.(go_storage.LockWatchable); ok && isFirstSubscriber {
		if x.watchCancels == nil {
			x.watchCancels = make(map[string]context.CancelFunc)
		}
		watchCtx, cancel := context.WithCancel(context.Background())
		x.watchCancels[lockId] = cancel
		go x.forwardStorageWatch(watchCtx, watchable, lockId)
	}
	x.subscribeMu.Unlock()

	// 立即推送当前锁状态（如果存在）。
	// 发送前必须持有 subscribeMu 并确认 ch 仍在订阅列表中：Subscribe 与 Unsubscribe
	// 可能并发执行，若 Unsubscribe 已经 close(ch)，此处若不加锁检查直接发送会
	// panic（send on closed channel）。发送本身用非阻塞 select，不会长时间占锁。
	go func() {
		e := events.NewEvent(lockId)
		info, err := x.getLockInformation(ctx, e, lockId)
		if err != nil || info == nil {
			return
		}

		x.subscribeMu.Lock()
		defer x.subscribeMu.Unlock()
		if !x.isSubscribedLocked(lockId, ch) {
			return
		}
		select {
		case ch <- info:
		default:
		}
	}()

	return ch, nil
}

// forwardStorageWatch 消费存储层原生 Watch 通道，将收到的变更转发给该 lockId 的所有订阅者。
// watchCtx 取消（最后一个订阅者 Unsubscribe）或存储层通道关闭时结束。
func (x *StorageLock) forwardStorageWatch(watchCtx context.Context, watchable go_storage.LockWatchable, lockId string) {
	watchCh, err := watchable.Watch(watchCtx, lockId)
	if err != nil {
		// 存储原生 Watch 启动失败，静默降级为仅本实例内的本地通知，不影响主路径。
		return
	}
	for {
		select {
		case <-watchCtx.Done():
			return
		case info, ok := <-watchCh:
			if !ok {
				return
			}
			x.notifySubscribers(lockId, info)
		}
	}
}

// isSubscribedLocked 检查指定 channel 是否仍在 lockId 的订阅列表中，调用方必须持有 subscribeMu。
func (x *StorageLock) isSubscribedLocked(lockId string, ch chan *go_storage.LockInformation) bool {
	for _, c := range x.subscribers[lockId] {
		if c == ch {
			return true
		}
	}
	return false
}

// Unsubscribe 取消订阅指定锁的状态变化通知，并关闭 channel。
// 关闭操作与 notifySubscribers/Subscribe 的发送操作共享 subscribeMu，
// 因此不会出现"关闭后仍有协程向其发送"而导致的 panic。
func (x *StorageLock) Unsubscribe(lockId string, ch <-chan *go_storage.LockInformation) {
	if lockId == "" || ch == nil {
		return
	}

	x.subscribeMu.Lock()
	defer x.subscribeMu.Unlock()

	if x.subscribers == nil {
		return
	}

	list := x.subscribers[lockId]
	for i, c := range list {
		if c == ch {
			list = append(list[:i], list[i+1:]...)
			close(c)
			break
		}
	}
	if len(list) == 0 {
		delete(x.subscribers, lockId)
		// 最后一个订阅者已离开，停止（如果存在）为该 lockId 转发存储原生 Watch 通知的后台协程，
		// 避免无人消费时继续空跑。
		if cancel, ok := x.watchCancels[lockId]; ok {
			cancel()
			delete(x.watchCancels, lockId)
		}
	} else {
		x.subscribers[lockId] = list
	}
}

// notifySubscribers 向指定锁的所有订阅者推送状态变更。
// 发送在持有 subscribeMu 的情况下进行，与 Unsubscribe 的 close(ch) 互斥，
// 避免向已关闭的 channel 发送导致 panic；发送本身是非阻塞的，不会长时间占锁。
func (x *StorageLock) notifySubscribers(lockId string, info *go_storage.LockInformation) {
	x.subscribeMu.Lock()
	defer x.subscribeMu.Unlock()

	for _, ch := range x.subscribers[lockId] {
		select {
		case ch <- info:
		default:
			// 订阅者 channel 满时丢弃，避免阻塞锁操作路径
		}
	}
}
