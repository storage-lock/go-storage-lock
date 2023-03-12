# Storage Lock 

# 一、这是什么？

提出了一种通用的基于存储介质（比如数据库）的分布式锁算法并对进行了编码实现。

提供的锁时抢占式的非公平锁，并提供可重入的特性。



# 二、 存储介质的支持

- [x] MySQL
- [x] Maria
- [x] TiDB
- [x] PostgreSQL
- [x] SqlServer
- [ ] Mongo 
- [ ] Redis
- [ ] Splunk
- [ ] Oracle
- [ ] Microsoft Azure SQL Database
- [ ] etcd
- [ ] Elasticsearch
- [ ] Cassandra
- [ ] Amazon DynamoDB

更多存储介质的支持请提Issues或pr。

# 三、基础使用

# 四、每种存储介质的API详解

## MySQL

## Maria

## TIDB


## Postgresql

## 快速开始 

```go

```

## 详细配置
```go

```


## SQLServer

### 快速开始

```go
package main

import (
	"context"
	"fmt"
	storage_lock "github.com/golang-infrastructure/go-storage-lock"
	"strings"
	"sync"
	"time"
)

func main() {

	// DSN的写法参考驱动的支持：https://github.com/denisenkom/go-mssqldb
	dsn := "sqlserver://sa:UeGqAm8CxYGldMDLoNNt@192.168.128.206:1433?database=storage_lock_test&connection+timeout=30"

	// 这个是最为重要的，通常是要锁住的资源的名称
	lockId := "must-serial-operation-resource-foo"

	// 第一步创建一把分布式锁
	lock, err := storage_lock.NewSqlServerStorageLock(context.Background(), lockId, dsn)
	if err != nil {
		fmt.Printf("Create Lock Failed: %v\n", err.Error())
		return
	}

	// 第二步使用这把锁，这里就模拟多个节点竞争执行的情况，他们会线程安全的往resource里写数据
	resource := strings.Builder{}
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		workerId := fmt.Sprintf("worker-%d", i)
		wg.Add(1)
		go func() {
			defer wg.Done()

			// 获取锁
			err := lock.Lock(context.Background(), workerId)
			if err != nil {
				fmt.Printf("workerId = %s, lock failed: %s \n", workerId, err.Error())
				return
			}
			// 退出的时候释放锁
			defer func() {
				err := lock.UnLock(context.Background(), workerId)
				if err != nil {
					fmt.Printf("workerId = %s, unlock failed: %s \n", workerId, err.Error())
					return
				}
			}()

			// 假装有耗时的操作
			fmt.Printf("workerId = %s, begin write resource \n", workerId)
			time.Sleep(time.Second * 3)
			// 接下来是操作竞态资源
			resource.WriteString(workerId)
			fmt.Printf("workerId = %s, write resource done \n", workerId)
			resource.WriteString("\n")

		}()
	}
	wg.Wait()

	// 观察最终的输出是否和日志一致
	fmt.Println("Resource: ")
	fmt.Println(resource.String())

}
```

### 详细配置

```go
package main

import (
	"context"
	"fmt"
	storage_lock "github.com/golang-infrastructure/go-storage-lock"
	"strings"
	"sync"
	"time"
)

func main() {

	// DSN的写法参考驱动的支持：https://github.com/denisenkom/go-mssqldb
	dsn := "sqlserver://sa:UeGqAm8CxYGldMDLoNNt@192.168.128.206:1433?database=storage_lock_test&connection+timeout=30"

	// 第一步先配置存储介质相关的参数，包括如何连接到这个数据库，连接上去之后锁的信息存储到哪里等等
	// 配置如何连接到数据库
	connectionGetter := storage_lock.NewSqlServerStorageConnectionGetterFromDSN(dsn)
	storageOptions := &storage_lock.SqlServerStorageOptions{
		// 数据库连接获取方式，可以使用内置的从DSN获取连接，也可以自己实现接口决定如何连接
		ConnectionGetter: connectionGetter,
		// 锁的信息是存储在哪张表中的，不设置的话默认为storage_lock
		TableName: "storage_lock_table",
	}
	storage, err := storage_lock.NewSqlServerStorage(context.Background(), storageOptions)
	if err != nil {
		fmt.Println("Create Storage Failed： " + err.Error())
		return
	}

	// 第二步配置锁的参数，在上面创建的Storage的上创建一把锁
	lockOptions := &storage_lock.StorageLockOptions{
		// 这个是最为重要的，通常是要锁住的资源的名称
		LockId:                "must-serial-operation-resource-foo",
		LeaseExpireAfter:      time.Second * 30,
		LeaseRefreshInterval:  time.Second * 5,
		VersionMissRetryTimes: 3,
	}
	lock := storage_lock.NewStorageLock(storage, lockOptions)

	// 第三步开始使用锁，模拟多个节点竞争同一个锁使用的情况
	resource := strings.Builder{}
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		workerId := fmt.Sprintf("worker-%d", i)
		wg.Add(1)
		go func() {
			defer wg.Done()

			// 获取锁
			err := lock.Lock(context.Background(), workerId)
			if err != nil {
				fmt.Printf("workerId = %s, lock failed: %s \n", workerId, err.Error())
				return
			}
			// 退出的时候释放锁
			defer func() {
				err := lock.UnLock(context.Background(), workerId)
				if err != nil {
					fmt.Printf("workerId = %s, unlock failed: %s \n", workerId, err.Error())
					return
				}
			}()

			// 假装有耗时的操作
			fmt.Printf("workerId = %s, begin write resource \n", workerId)
			time.Sleep(time.Second * 3)
			// 接下来是操作竞态资源
			resource.WriteString(workerId)
			fmt.Printf("workerId = %s, write resource done \n", workerId)
			resource.WriteString("\n")

		}()
	}
	wg.Wait()

	// 观察最终的输出是否和日志一致
	fmt.Println("Resource: ")
	fmt.Println(resource.String())

}
```

## Mongo 

## Redis

## Splunk

## Oracle

## Microsoft Azure SQL Database

## etcd

## Elasticsearch

## Cassandra

## Amazon DynamoDB



# 五、Storage Lock分布式锁算法原理详解







