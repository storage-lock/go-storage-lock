package storage_lock

import "errors"

// ErrLockIdEmpty 参数相关的参数检查
var (
	ErrLockIdEmpty          = errors.New("lock id can not empty")
	ErrLeaseExpireAfter     = errors.New("LeaseExpireAfter must > time.Second * 3 ")
	ErrLeaseRefreshInterval = errors.New("LeaseRefreshInterval must < ErrLeaseExpireAfter ")
	// ErrLeaseRefreshIntervalTooClose 续租刷新间隔与租约过期时间过于接近。
	// 若 LeaseExpireAfter - LeaseRefreshInterval 的余量过小，一次续租的网络抖动/存储延迟
	// 就可能让租约在下一次刷新完成前过期，被他人合法抢占，破坏互斥性（漏洞 I）。
	ErrLeaseRefreshIntervalTooClose = errors.New("LeaseRefreshInterval too close to LeaseExpireAfter, leave enough margin for renewal jitter")
)

var (

	// ErrLockFailed 尝试加锁失败
	ErrLockFailed = errors.New("lock failed")

	// ErrLockBusy 锁在被其它人持有着
	ErrLockBusy = errors.New("lock busy")

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

var (

	// ErrStorageCapabilityMissing 存储实现缺少分布式锁的必要能力
	// 当 Storage 实现的 Capabilities() 方法返回值中缺少 CapabilityCAS 或 CapabilityReliableTime 时会返回此错误
	// 这意味着该存储实现无法保证锁的正确性，不应被用于生产环境
	ErrStorageCapabilityMissing = errors.New("storage missing required capabilities for distributed lock")
)
