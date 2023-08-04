package storage_lock

import (
	"github.com/storage-lock/go-storage-lock/pkg/event"
	"time"
)

const (

	// DefaultLeaseExpireAfter 默认的租约有效时间
	// 这个时间不适合设置太长，如果太长的话可能会导致上次持有锁的owner异常退出时锁不能及时得到释放
	// 这个值也不适合设置太短，太短的话可能会导致锁来不及续租就过期了从而导致一个锁同时被多个人同时获取
	DefaultLeaseExpireAfter = time.Minute * 5

	// DefaultLeaseRefreshInterval 默认的租约过期时间刷新间隔
	DefaultLeaseRefreshInterval = time.Second * 10

	// DefaultVersionMissRetryTimes 默认的版本乐观锁未命中时的重试次数
	DefaultVersionMissRetryTimes = 100
)

// StorageLockOptions 创建存储锁的相关选项
type StorageLockOptions struct {

	// 创建锁的时候可以指定锁ID，如果指定的话会使用给定的值作为锁的ID，未指定的话则会生成一个默认的ID
	// 要想达到分布式协调资源的效果必须手动指定锁的ID（一般是根据业务场景生成一个ID），自动生成的锁ID仅能用于同一个进程内协调资源
	// 比如要分布式操作一个用户的资源，则可以将这个用户的ID作为锁的ID
	LockId string

	// 获取到锁之后期望的租约有效期，比如获取到锁之后租约有效期五分钟，则五分钟锁只有自己才能操作，在获取成功锁之后会有个协程专门负责刷新租约时间
	LeaseExpireAfter time.Duration

	// 租约刷新间隔，当获取锁成功时会有一个协程专门负责续约租约，这个参数就决定它每隔多久发起一次续约操作，这个用来保证不会在锁使用的期间突然过期
	LeaseRefreshInterval time.Duration

	// 2023-6-21 21:28:05 修改为版本未命中的时候就锁死一直等待，而不设置具体的放弃时间
	// 这个放弃时机感觉以具体的时间长度更为合适
	//// 乐观锁的版本未命中的时候的重试次数
	//VersionMissRetryTimes uint

	// 用于监听观测锁使用过程中的各种事件
	EventListeners []event.EventListener
}

// NewStorageLockOptions 使用默认值创建配置项
func NewStorageLockOptions() *StorageLockOptions {
	return &StorageLockOptions{
		LeaseExpireAfter:     DefaultLeaseExpireAfter,
		LeaseRefreshInterval: DefaultLeaseRefreshInterval,
		//VersionMissRetryTimes: DefaultVersionMissRetryTimes,
	}
}

// NewStorageLockOptionsWithLockId 使用默认值创建配置项，同时指定锁的ID
func NewStorageLockOptionsWithLockId(lockId string) *StorageLockOptions {
	return NewStorageLockOptions().WithLockId(lockId)
}

func (x *StorageLockOptions) WithLockId(lockId string) *StorageLockOptions {
	x.LockId = lockId
	return x
}

func (x *StorageLockOptions) WithLeaseExpireAfter(leaseExpireAfter time.Duration) *StorageLockOptions {
	x.LeaseExpireAfter = leaseExpireAfter
	return x
}

func (x *StorageLockOptions) WithLeaseRefreshInterval(leaseRefreshInterval time.Duration) *StorageLockOptions {
	x.LeaseRefreshInterval = leaseRefreshInterval
	return x
}

//func (x *StorageLockOptions) WithVersionMissRetryTimes(versionMissRetryTimes uint) *StorageLockOptions {
//	x.VersionMissRetryTimes = versionMissRetryTimes
//	return x
//}
