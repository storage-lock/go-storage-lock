package storage_lock

// 通用的事件
const (
	ActionLockNotFoundError       = "lock-not-found-error"
	ActionNotLockOwner            = "not-lock-owner"
	ActionGetLockInformationError = "get-lock-information-error"
	ActionTimeout                 = "timeout"
	ActionSleepRetry              = "sleep-retry"
	ActionSleep                   = "Sleep"
	ActionGetLeaseExpireTimeError = "getLeaseExpireTime-error"
)

// 获取锁相关的事件
const (
	ActionLockBegin   = "StorageLock.Lock.Begin"
	ActionLockFinish  = "StorageLock.Lock.Finish"
	ActionLockSuccess = "StorageLock.Lock.Begin.Success"
	ActionLockError   = "StorageLock.Lock.Begin.Error"

	ActionTryLockBegin = "StorageLock.Lock.Try.Begin"

	ActionLockExists    = "StorageLock.Lock.Exists"
	ActionLockNotExists = "StorageLock.Lock.NotExists"
	ActionLockExpired   = "StorageLock.Lock.Expired"
	ActionLockReentry   = "StorageLock.Lock.Reentry"

	ActionLockBusy        = "StorageLock.Lock.Begin.Busy"
	ActionLockVersionMiss = "StorageLock.Lock.VersionMiss"

	ActionLockRollback        = "StorageLock.Lock.Rollback"
	ActionLockRollbackSuccess = "StorageLock.Lock.Rollback.Success"
	ActionLockRollbackError   = "StorageLock.Lock.Rollback.Error"
)

// 释放锁相关的事件
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
	ActionWatchDogRefreshError   = "WatchDog.Refresh.Error"

	ActionWatchDogCreate        = "WatchDog.Create"
	ActionWatchDogCreateSuccess = "WatchDog.Create.Success"
	ActionWatchDogCreateError   = "WatchDog.Create.Error"

	ActionWatchDogStart        = "WatchDog.Start"
	ActionWatchDogStartSuccess = "WatchDog.Start.Success"
	ActionWatchDogStartError   = "WatchDog.Start.Error"

	ActionWatchDogStop        = "WatchDog.Stop"
	ActionWatchDogStopSuccess = "WatchDog.Stop.success"
	ActionWatchDogStopError   = "WatchDog.Stop.error"

	ActionWatchDogExit = "WatchDog.Exit"
	//ActionWatchDogExitByTooManyError = "WatchDog.Exit.TooManyError"

	ActionWatchDogSetEvent = "WatchDog.SetEvent"

	ActionWatchDogOwnerIdMismatch = "WatchDog.OwnerIdMismatch"
)

// Payload的名字
const (
	PayloadLastVersion         = "lastVersion"
	PayloadVersionMissCount    = "versionMissCount"
	PayloadLockBusyCount       = "lockBusyCount"
	PayloadSleep               = "sleep"
	PayloadRefreshSuccessCount = "refreshSuccessCount"
	PayloadContinueErrorCount  = "continueErrorCount"
)

// 读写锁相关的事件
const (
	ActionRLockBegin       = "StorageLock.RLock.Begin"
	ActionRLockSuccess     = "StorageLock.RLock.Success"
	ActionRLockError       = "StorageLock.RLock.Error"
	ActionRLockVersionMiss = "StorageLock.RLock.VersionMiss"
	ActionRLockBusy        = "StorageLock.RLock.Busy"

	ActionRUnlockBegin       = "StorageLock.RUnlock.Begin"
	ActionRUnlockSuccess     = "StorageLock.RUnlock.Success"
	ActionRUnlockError       = "StorageLock.RUnlock.Error"
	ActionRUnlockVersionMiss = "StorageLock.RUnlock.VersionMiss"
	ActionRUnlockRelease     = "StorageLock.RUnlock.Release"

	ActionWLockBegin   = "StorageLock.WLock.Begin"
	ActionWLockSuccess = "StorageLock.WLock.Success"
	ActionWLockError   = "StorageLock.WLock.Error"

	ActionWUnlockBegin   = "StorageLock.WUnlock.Begin"
	ActionWUnlockSuccess = "StorageLock.WUnlock.Success"
	ActionWUnlockError   = "StorageLock.WUnlock.Error"

	ActionUpgradeBegin   = "StorageLock.Upgrade.Begin"
	ActionUpgradeSuccess = "StorageLock.Upgrade.Success"
	ActionUpgradeError   = "StorageLock.Upgrade.Error"
)
