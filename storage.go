package storage_lock

import (
	"context"
	"time"
)

// Version 表示一个版本号
type Version uint64

// Storage 表示一个存储介质的实现，要实现四个增删改查的方法和一个初始化的方法，以及能够提供Storage的日期，
// 因为在分布式系统中日期很重要，必须保证参与分布式运算的各个节点使用相同的时间
type Storage interface {

	// Init 初始化操作，比如创建存储锁的表，需要支持多次调用，每次创建Storage的时候会调用此方法初始化
	Init(ctx context.Context) error

	// UpdateWithVersion 如果存储的是指定版本的话，则将其更新
	// lockId 表示锁的ID
	// exceptedValue 仅当老的值为这个时才进行更新
	// newValue 更新为的新的值
	UpdateWithVersion(ctx context.Context, lockId string, exceptedVersion, newVersion Version, lockInformationJsonString string) error

	// InsertWithVersion 尝试将锁的信息插入到存储介质中，返回是否插入成功
	InsertWithVersion(ctx context.Context, lockId string, version Version, lockInformationJsonString string) error

	// DeleteWithVersion 如果锁的当前版本是期望的版本，则将其删除
	DeleteWithVersion(ctx context.Context, lockId string, exceptedVersion Version) error

	// Get 获取锁之前存储的值，如果没有的话则返回空字符串，如果发生了错误则返回对应的错误信息
	Get(ctx context.Context, lockId string) (string, error)

	// GetTime 分布式锁的话时间必须统一使用Storage的时间，所以Storage要能够提供时间查询的功能
	// 这是因为分布式锁的算法需要根据时间来协调推进，而当时间不准确的时候算法可能会失效从而导致锁失效
	GetTime(ctx context.Context) (time.Time, error)

	// Close 关闭此存储介质
	Close(ctx context.Context) error
}
