package storage_lock

import "errors"

var (

	// ErrLockFailed 尝试加锁失败
	ErrLockFailed = errors.New("lock failed")

	// ErrLockAlreadyExists 锁已经存在，无法继续进行给定操作
	ErrLockAlreadyExists = errors.New("lock already exists")

	// ErrUnlockFailed 锁释放失败，锁是自己的，但由于种种原因释放失败了
	ErrUnlockFailed = errors.New("unlock failed")

	// ErrLockNotFound 要操作的锁不存在，本来无一物，何处染尘埃，无法在不存在的锁上施加操作
	ErrLockNotFound = errors.New("lock not found")

	// ErrLockNotBelongYou 尝试释放不属于自己的锁，一般情况下不应该有这个错误，除非发生了越权操作
	ErrLockNotBelongYou = errors.New("v not belong you")

	// ErrLockRefreshFailed 刷新锁的过期时间时出错
	ErrLockRefreshFailed = errors.New("lock refresh failed")
)

var (

	// ErrVersionMiss 版本未命中，基于版本操作时比较常见的问题
	ErrVersionMiss = errors.New("compare and set miss")

	// ErrOwnerCanOnlyOne 锁的有拥有者只能有一个，这是尝试给锁指定了多个owner时会返回的错误
	ErrOwnerCanOnlyOne = errors.New("lock owner only one")
)

