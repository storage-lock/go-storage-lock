package storage_lock

// 通用
const (
	ActionLockNotFoundError       = "lock-not-found-error"
	ActionNotLockOwner            = "not-lock-owner"
	ActionGetLockInformationError = "get-lock-information-error"
	ActionTimeout                 = "timeout"
	ActionSleepRetry              = "sleep-retry"
	ActionSleep                   = "Sleep"
	ActionGetLeaseExpireTimeError = "getLeaseExpireTime-error"
)

// 获取锁
const (
	ActionLockBegin   = "StorageLock.Lock.Begin"
	ActionLockFinish  = "StorageLock.Lock.Finish"
	ActionLockSuccess = "StorageLock.Lock.Begin.Success"
	ActionLockError   = "StorageLock.Lock.Begin.Error"

	ActionTryLockBegin = "StorageLock.Lock.Try.Begin"

	ActionLockExists    = "StorageLock.Lock.Exists"
	ActionLockNotExists = "StorageLock.Lock.NotExists"
	ActionLockReleased  = "StorageLock.Lock.Released"
	ActionLockExpired   = "StorageLock.Lock.Expired"
	ActionLockReentry   = "StorageLock.Lock.Reentry"

	ActionLockBusy        = "StorageLock.Lock.Begin.Busy"
	ActionLockVersionMiss = "StorageLock.Lock.VersionMiss"

	ActionLockRollback        = "StorageLock.Lock.Rollback"
	ActionLockRollbackSuccess = "StorageLock.Lock.Rollback.Success"
	ActionLockRollbackError   = "StorageLock.Lock.Rollback.Error"
)

// 释放锁
const (
	ActionUnlock        = "StorageLock.Unlock"
	ActionUnlockFinish  = "StorageLock.Unlock.Finish"
	ActionUnlockSuccess = "StorageLock.Unlock.Success"
	ActionUnlockError   = "StorageLock.Unlock.Error"

	ActionUnlockRelease = "StorageLock.Unlock.Release"
	ActionUnlockReentry = "StorageLock.Unlock.Reentry"

	ActionUnlockVersionMiss = "StorageLock.Unlock.VersionMiss"
)

// 看门狗相关的事件
const (
	ActionWatchDogRefresh        = "WatchDog.Refresh"
	ActionWatchDogRefreshBegin   = "WatchDog.Refresh.Begin"
	ActionWatchDogRefreshSuccess = "WatchDog.Refresh.Success"

	ActionWatchDogCreate        = "WatchDog.Create"
	ActionWatchDogCreateSuccess = "WatchDog.Create.Success"
	ActionWatchDogCreateError   = "WatchDog.Create.Error"

	ActionWatchDogStart        = "WatchDog.Start"
	ActionWatchDogStartSuccess = "WatchDog.Start.Success"
	ActionWatchDogStartError   = "WatchDog.Start.Error"

	ActionWatchDogStop        = "WatchDog.Stop"
	ActionWatchDogStopSuccess = "WatchDog.Stop.success"
	ActionWatchDogStopError   = "WatchDog.Stop.error"

	ActionWatchDogExit               = "WatchDog.Exit"
	ActionWatchDogExitByTooManyError = "WatchDog.Exit.TooManyError"
)

const (
	PayloadLastVersion      = "lastVersion"
	PayloadVersionMissCount = "versionMissCount"
	PayloadSleep            = "sleep"
)
