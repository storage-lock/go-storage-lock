package storage_lock

import (
	"context"

	"github.com/storage-lock/go-events"
)

// publishSemanticEvent 供核心库内部发出携带语义类型的事件。该事件作为独立子事件
// （无 parent）发布，不与具体操作的事件树绑定：下游观测组件（审计、指标、链路）
// 直接按 EventType 过滤即可识别「过期」「竞争」「续租」「丢失」「队列变化」这类
// 高价值信号，而不必解析散落在 action 文本里的字段。
//
// 语义事件是可观测性增强，不改变任何锁语义；静默失败（无 listener 或空 ctx 时）
// 不影响调用方。
func (x *StorageLock) publishSemanticEvent(ctx context.Context, eventType events.EventType, lockId, ownerId string) {
	if len(x.options.EventListeners) == 0 {
		return
	}
	// 语义事件本身也是普通事件：带 lock/owner/storage 上下文 + 类型，方便 filter/审计。
	e := events.NewEvent(lockId).
		SetType(eventType).
		SetOwnerId(ownerId).
		SetStorageName(x.storage.GetName()).
		SetListeners(x.options.EventListeners)
	e.Publish(ctx)
}

// publishExpired 锁记录过期（或为墓碑）被抢占时发出。
func (x *StorageLock) publishExpired(ctx context.Context, ownerId string) {
	x.publishSemanticEvent(ctx, events.EventTypeExpired, x.options.LockId, ownerId)
}

// publishBusy 竞争锁失败、需要等待重试时发出。
func (x *StorageLock) publishBusy(ctx context.Context, ownerId string) {
	x.publishSemanticEvent(ctx, events.EventTypeBusy, x.options.LockId, ownerId)
}

// publishRenewed 锁租约被看门狗续期成功时发出。
func (x *StorageLock) publishRenewed(ctx context.Context, ownerId string) {
	x.publishSemanticEvent(ctx, events.EventTypeRenewed, x.options.LockId, ownerId)
}

// publishLost 看门狗确认持有权丢失（他人抢占、租约过期、锁被删）时发出。
func (x *StorageLock) publishLost(ctx context.Context, ownerId string) {
	x.publishSemanticEvent(ctx, events.EventTypeLost, x.options.LockId, ownerId)
}

// publishWaitChanged 锁记录上的等待队列发生变化时发出。
func (x *StorageLock) publishWaitChanged(ctx context.Context, ownerId string) {
	x.publishSemanticEvent(ctx, events.EventTypeWaitChanged, x.options.LockId, ownerId)
}
