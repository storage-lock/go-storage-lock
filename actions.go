package storage_lock

// 通用
const (
	ActionLockNotFoundError       = "lock-not-found-error"
	ActionNotLockOwner            = "not-lock-owner"
	ActionGetLockInformationError = "get-lock-information-error"
	ActionTimeout                 = "timeout"
	ActionSleepRetry              = "sleep-retry"
	ActionGetLeaseExpireTimeError = "getLeaseExpireTime-error"
)

// 获取锁
const (
	ActionLockSuccess = "lock-success"
	ActionLockError   = "lock-error"
)

// 释放锁
const (
	ActionUnlockSuccess = "unlock-success"
	ActionUnlockError   = "unlock-error"
)

// 看门狗
const (
	ActionWatchDogRefreshLease = "refresh-lease"
	ActionWatchDogCreate         = "watch-dog-create"
	ActionWatchDogStop         = "watch-dog-stop"
	ActionWatchDogStart        = "watch-dog-start"
)


