package storage_lock

import (
	"github.com/storage-lock/go-events"
	"time"
)

// 这里使用了var而不是const，是给用户能够修改全局默认值的机会如果他需要的话
var (

	// DefaultLeaseExpireAfter 默认的锁的租约有效时间
	// 这个时间不适合设置太长，如果太长的话可能会导致上次持有锁的owner异常退出时锁不能及时得到释放，导致假性持有的时间过长其它需要锁的人不能及时获取到锁
	// 这个值也不适合设置太短，太短的话可能会导致锁来不及续租就过期了从而导致一个锁同时被多个人同时获取，这就导致锁失去了互斥性
	// 这个值的上限不做约定，但是下限不能小于 3 秒
	DefaultLeaseExpireAfter = time.Minute * 5

	// DefaultLeaseRefreshInterval 默认的租约过期时间刷新间隔，如果并发比较高或者网络不是很好的话可以适当的把这个值调大
	// 需要注意的是这个租约的刷新间隔不能超过 (DefaultLeaseExpireAfter - time.Second)
	DefaultLeaseRefreshInterval = time.Second * 30

	// DefaultVersionMissRetryTimes 默认的版本乐观锁未命中时的重试次数，这里为了防止高并发或者网络不好等情况导致失败率过高就把值设置的稍微大了一些，
	// 用户可以根据自己的实际情况调整重试次数
	//DefaultVersionMissRetryTimes = 100
)

// 检查参数配置是否正确
func checkStorageLockOptions(options *StorageLockOptions) error {

	// 锁的ID不允许为空
	if options.LockId == "" {
		return ErrLockIdEmpty
	}

	// 每次刷新租约的时候把有效期往后推动的时间不能小于3秒
	if options.LeaseExpireAfter < time.Second*3 {
		return ErrLeaseExpireAfter
	}

	// 刷新间隔必须小于租约有效时间，不然租约都过期了再刷新还有个毛用啊
	if options.LeaseRefreshInterval >= options.LeaseExpireAfter {
		return ErrLeaseRefreshInterval
	}

	// 如果没有设置看门狗factory的话，则为其设置上默认的
	if options.WatchDogFactory == nil {
		options.WatchDogFactory = NewWatchDogFactoryCommonsImpl()
	}

	return nil
}

// StorageLockOptions 创建存储锁的相关选项
type StorageLockOptions struct {

	// 创建锁的时候可以指定锁ID，如果指定的话会使用给定的值作为锁的ID，未指定的话则会生成一个默认的ID
	// 要想达到分布式协调资源的效果必须手动指定锁的ID（一般是根据业务场景生成一个ID），自动生成的锁ID仅能用于同一个进程内协调资源
	// 比如要分布式操作一个用户的资源，则可以将这个用户的ID作为锁的ID
	// @see:
	//     ErrLockIdEmpty
	LockId string

	// 获取到锁之后期望的租约有效期，比如获取到锁之后租约有效期五分钟，则五分钟锁只有自己才能操作，在获取成功锁之后会有个协程专门负责刷新租约时间
	// @see:
	//     DefaultLeaseExpireAfter
	//     ErrLeaseExpireAfter
	LeaseExpireAfter time.Duration

	// 租约刷新间隔，当获取锁成功时会有一个协程专门负责续约租约，这个参数就决定它每隔多久发起一次续约操作，这个用来保证不会在锁使用的期间突然过期
	LeaseRefreshInterval time.Duration

	// 这个放弃时机感觉以具体的时间长度更为合适
	//// 乐观锁的版本未命中的时候的重试次数
	// 当传入的值等于0的时候表示不进行重试
	// 当传入的值小于0的时候表示无限重试不成功永远不结束
	//VersionMissRetryTimes uint

	// 用于监听观测锁使用过程中的各种事件，如果需要的话自行设置
	EventListeners []events.Listener

	// 用于创建看门狗
	WatchDogFactory WatchDogFactory

	// 版本未命中时的重试间隔
	VersionMissRetryInterval time.Duration
}

// NewStorageLockOptions 使用默认值创建锁的配置项
func NewStorageLockOptions() *StorageLockOptions {
	return &StorageLockOptions{
		LeaseExpireAfter:     DefaultLeaseExpireAfter,
		LeaseRefreshInterval: DefaultLeaseRefreshInterval,
		//VersionMissRetryTimes: DefaultVersionMissRetryTimes,
	}
}

// NewStorageLockOptionsWithLockId 使用默认值创建配置项，同时指定锁的ID
func NewStorageLockOptionsWithLockId(lockId string) *StorageLockOptions {
	return NewStorageLockOptions().SetLockId(lockId)
}

func (x *StorageLockOptions) SetLockId(lockId string) *StorageLockOptions {
	x.LockId = lockId
	return x
}

func (x *StorageLockOptions) SetLeaseExpireAfter(leaseExpireAfter time.Duration) *StorageLockOptions {
	x.LeaseExpireAfter = leaseExpireAfter
	return x
}

func (x *StorageLockOptions) SetLeaseRefreshInterval(leaseRefreshInterval time.Duration) *StorageLockOptions {
	x.LeaseRefreshInterval = leaseRefreshInterval
	return x
}

//func (x *StorageLockOptions) WithVersionMissRetryTimes(versionMissRetryTimes uint) *StorageLockOptions {
//	x.VersionMissRetryTimes = versionMissRetryTimes
//	return x
//}
