package storage_lock

type Logger interface {
	Error(message string, args ...any)
	Debug(message string, args ...any)
}
