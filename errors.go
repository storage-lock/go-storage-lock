package storage_lock

import "errors"

var (
	ErrLockFailed        = errors.New("storageLock failed")
	ErrLockAlreadyExists = errors.New("storageLock already exists")
	ErrUnlockFailed      = errors.New("unlock failed")
	ErrLockNotFound      = errors.New("storageLock not found")
	ErrLockNotBelongYou  = errors.New("storageLock not belong you")
	ErrLockRefreshFailed = errors.New("storageLock refresh failed")
)

var (
	ErrVersionMiss = errors.New("compare and set miss")
)
