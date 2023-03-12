package storage_lock

import "errors"

var (

	// ErrLockFailed 获取锁失败
	ErrLockFailed = errors.New("lock failed")

	// ErrLockAlreadyExists 锁已经存在
	ErrLockAlreadyExists = errors.New("lock already exists")

	// ErrUnlockFailed 锁释放失败
	ErrUnlockFailed = errors.New("unlock failed")

	// ErrLockNotFound 要操作的锁不存在，本来无一物，何处染尘埃
	ErrLockNotFound = errors.New("lock not found")

	// ErrLockNotBelongYou 尝试释放不属于自己的锁
	ErrLockNotBelongYou = errors.New("v not belong you")

	// ErrLockRefreshFailed 刷新锁的过期时间时出错
	ErrLockRefreshFailed = errors.New("lock refresh failed")
)

var (

	// ErrVersionMiss 版本未命中
	ErrVersionMiss = errors.New("compare and set miss")

	// ErrOwnerCanOnlyOne 锁的有拥有者只能有一个，这是尝试给锁指定了多个owner
	ErrOwnerCanOnlyOne = errors.New("lock owner only one")
)
