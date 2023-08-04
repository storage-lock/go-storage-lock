package event

type EventType int

const (
	EventTypeUnknown EventType = iota
	EventTypeLock
	EventTypeUnlock
	EventTypeVersionMiss
	EventTypeCreateLock
)
