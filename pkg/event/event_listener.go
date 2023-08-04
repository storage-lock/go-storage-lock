package event

import "context"

// EventListener 事件监听器
type EventListener interface {
	On(ctx context.Context, e *Event)
}
