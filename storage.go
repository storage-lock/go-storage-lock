package storage_lock

import "context"

type Version uint64

// Storage 表示一个存储介质的实现，要实现四个增删改查的方法和一个初始化的方法
type Storage interface {

	// Init 初始化操作，比如创建存储锁的表，需要支持多次调用
	Init() error

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
}
