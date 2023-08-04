package storage_lock

// 用于统计Storage上的方法调用行为
const (
	ActionStorageGetName           = "Storage.GetName"
	ActionStorageInit              = "Storage.Init"
	ActionStorageUpdateWithVersion = "Storage.UpdateWithVersion"
	ActionStorageInsertWithVersion = "Storage.InsertWithVersion"
	ActionStorageDeleteWithVersion = "Storage.DeleteWithVersion"
	ActionStorageGetTime           = "Storage.GetTime"
	ActionStorageGet               = "Storage.Get"
	ActionStorageClose             = "Storage.Close"
	ActionStorageList              = "Storage.List"
)

// 通用
const (
	ActionLockNotFoundError       = "lock-not-found-error"
	ActionNotLockOwner            = "not-lock-owner"
	ActionGetLockInformationError = "get-lock-information-error"
)

// 看门狗
const (
	ActionWatchDogRefreshLease = "refresh-lease"
	ActionWatchDogStop         = "stop-watch-dog"
)

// time provider
const (
	ActionNtpError = "ntp-error"
	ActionNtpZero  = "ntp-zero"
)
