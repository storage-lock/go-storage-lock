package storage

import (
	"context"
	"github.com/golang-infrastructure/go-iterator"
	"time"
)

// Version 表示一个锁的版本号，锁在每次被更改状态，比如每次被持有释放的时候都会增加版本号
type Version uint64

// Storage 表示一个存储介质的实现，要实现四个增删改查的方法和一个初始化的方法，以及能够提供Storage的日期，
// 因为在分布式系统中日期很重要，必须保证参与分布式运算的各个节点使用相同的时间
type Storage interface {

	// GetName Storage的名称，用于区分不同的Storage的实现
	// Returns:
	//     string: Storage的名字，应该返回一个有辨识度并且简单易懂的名字，名字不能为空，否则认为是不合法的Storage实现
	GetName() string

	// Init 初始化操作，比如创建存储锁的表，需要支持多次调用，每次创建Storage的时候会调用此方法初始化
	// Params:
	//     ctx:
	// Returns:
	//    error: 初始化发生错误时返回对应的错误
	Init(ctx context.Context) error

	// UpdateWithVersion 如果存储的是指定版本的话，则将其更新
	// Params:
	//     lockId 表示锁的ID
	//     exceptedValue 仅当老的值为这个时才进行更新
	//     newValue 更新为的新的值
	// Returns:
	//    error: 如果是版本不匹配，则返回错误 ErrVersionMiss，如果是其它类型的错误，依据情况自行返回
	UpdateWithVersion(ctx context.Context, lockId string, exceptedVersion, newVersion Version, lockInformation *LockInformation) error

	// InsertWithVersion 尝试将锁的信息插入到存储介质中，返回是否插入成功，底层存储的时候应该将锁的ID作为唯一ID，不能重复存储
	// 也就是说这个方法仅在锁不存在的时候才能执行成功，其它情况应该插入失败返回对应的错误
	// Params:
	//     ctx:
	//     lockId:
	//     version:
	//     lockInformation:
	// Returns:
	//     error:
	InsertWithVersion(ctx context.Context, lockId string, version Version, lockInformation *LockInformation) error

	// DeleteWithVersion 如果锁的当前版本是期望的版本，则将其删除
	// 如果是版本不匹配，则返回错误 ErrVersionMiss，如果是其它类型的错误，依据情况自行返回
	//
	DeleteWithVersion(ctx context.Context, lockId string, exceptedVersion Version, lockInformation *LockInformation) error

	// Get 获取锁之前存储的值，如果没有的话则返回空字符串，如果发生了错误则返回对应的错误信息，如果正常返回则是LockInformation的JSON字符串
	// Params:
	//     ctx: 用来做超时控制之类的
	//     lockId: 要查询的锁的ID
	// Returns:
	//    string:
	//    error:
	Get(ctx context.Context, lockId string) (string, error)

	// GetTime 分布式锁的话时间必须使用统一的时间，这个时间推荐是以Storage的时间为准，Storage要能够提供时间查询的功能
	// 这是因为分布式锁的算法需要根据时间来协调推进，而当时间不准确的时候算法可能会失效从而导致锁失效
	// TODO 2023-5-15 01:48:19 基于实例的时间在分布式数据库中可能会失效，单实例没问题
	// TODO 2023-8-3 21:53:35 在文档中用实际例子演示分布式情况下可能会存在的问题
	// Params:
	//     ctx: 用来做超时控制之类的
	// Returns:
	//     time.Time: 返回Storage的当前时间
	//     error: 获取时间失败时则返回对应的错误
	GetTime(ctx context.Context) (time.Time, error)

	// Close 关闭此存储介质，一般在系统退出释放资源的时候调用一下
	// Params:
	//     ctx: 用来做超时控制之类的
	// Returns:
	//     error: 如果关闭失败，则返回对应的错误
	Close(ctx context.Context) error

	// List 列出当前的Storage所持有的所有的锁的信息，因为数量可能会比较多，所以这里使用了一个迭代器模式
	// 虽然实际上可能用channel会更Golang一些，但是迭代器会比较易于实现并能够绑定一些内置的方法便于操作
	// Params:
	//     ctx: 用来做超时控制之类的
	// Returns:
	//     iterator.Iterator[*LockInformation]: 迭代器用来承载当前所有的锁
	//     error: 如果列出失败，则返回对应类型的错误
	List(ctx context.Context) (iterator.Iterator[*LockInformation], error)
}
