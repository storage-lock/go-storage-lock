package storage_lock

import (
	"context"
	"sync/atomic"

	"github.com/storage-lock/go-events"
)

// LockMetrics 统计当前进程观察到的锁事件。
// 它只做计数，不连接外部监控系统；Prometheus 或 OpenTelemetry 可以读取 Snapshot 后再导出。
// 计数按事件到达累加，不跨进程，也不代表存储里的全局次数。
type LockMetrics struct {
	acquired   atomic.Uint64
	busy       atomic.Uint64
	expired    atomic.Uint64
	renewed    atomic.Uint64
	lost       atomic.Uint64
	waitChange atomic.Uint64
}

// LockMetricsSnapshot 是某一时刻的计数副本。
type LockMetricsSnapshot struct {
	Acquired        uint64
	Busy            uint64
	Expired         uint64
	Renewed         uint64
	Lost            uint64
	WaitQueueChange uint64
}

// NewLockMetrics 创建一个空计数器。把它的 Listener 放进 StorageLockOptions.EventListeners 后才开始计数。
func NewLockMetrics() *LockMetrics {
	return &LockMetrics{}
}

// Snapshot 返回当前计数。读取与事件回调并发时，各项都是原子的，但各项之间不是同一个瞬间。
func (m *LockMetrics) Snapshot() LockMetricsSnapshot {
	return LockMetricsSnapshot{
		Acquired:        m.acquired.Load(),
		Busy:            m.busy.Load(),
		Expired:         m.expired.Load(),
		Renewed:         m.renewed.Load(),
		Lost:            m.lost.Load(),
		WaitQueueChange: m.waitChange.Load(),
	}
}

// Listener 返回可挂到锁选项上的事件监听器。
func (m *LockMetrics) Listener() events.Listener {
	return events.NewListenerWrapper("storage-lock-metrics", m.On)
}

// On 按事件类型累加。不认识的事件忽略。
func (m *LockMetrics) On(_ context.Context, e *events.Event) {
	if m == nil || e == nil {
		return
	}
	for _, action := range e.Actions {
		if action != nil && (action.Name == ActionLockSuccess || action.Name == ActionRLockSuccess || action.Name == ActionWLockSuccess) {
			m.acquired.Add(1)
		}
	}
	switch e.EventType {
	case events.EventTypeBusy:
		m.busy.Add(1)
	case events.EventTypeExpired:
		m.expired.Add(1)
	case events.EventTypeRenewed:
		m.renewed.Add(1)
	case events.EventTypeLost:
		m.lost.Add(1)
	case events.EventTypeWaitChanged:
		m.waitChange.Add(1)
	}
}
