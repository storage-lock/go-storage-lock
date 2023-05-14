package storage_lock

// TODO 2023-5-14 15:47:06 基于事件实现对日志的支持

type Logger interface {
	Error(message string, args ...any)
	Debug(message string, args ...any)
}
