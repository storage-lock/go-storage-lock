# Storage Lock 

# 一、这是什么？

提出了一种通用的基于存储介质（比如数据库）的分布式锁算法并对进行了编码实现。

提供的锁时抢占式的非公平锁，并提供可重入的特性。



# 二、 存储介质的支持

- [x] [MySQL](#3.1%20MySQL)
- [x] [MariaDB](./#3.6%20MariaDB)
- [ ] TiDB
- [x] [PostgreSQL](#3.2%20Postgresql)
- [x] [SQLServer](#3.3%20SQLServer)
- [x] [Mongo ](#3.4%20Mongo)
- [ ] Redis
- [ ] Splunk
- [ ] Oracle
- [ ] Microsoft Azure SQL Database
- [ ] etcd
- [ ] Elasticsearch
- [ ] Cassandra
- [ ] Amazon DynamoDB

更多存储介质的支持请提Issues或pr。

# 三、每种存储介质的API详解

## 3.1 MySQL

### 3.1.1 快速开始

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

	// Docker启动MySQL：
	// docker run -itd --name storage-lock-mysql -p 3306:3306 -e MYSQL_ROOT_PASSWORD=UeGqAm8CxYGldMDLoNNt mysql:5.7

	// DSN的写法参考驱动的支持：github.com/go-sql-driver/mysql
	dsn := "root:UeGqAm8CxYGldMDLoNNt@tcp(192.168.128.206:3306)/storage_lock_test"

	// 这个是最为重要的，通常是要锁住的资源的名称
	lockId := "must-serial-operation-resource-foo"

	// 第一步创建一把分布式锁
	lock, err := storage_lock.NewMySQLStorageLock(context.Background(), lockId, dsn)
	if err != nil {
		fmt.Printf("[ %s ] Create Lock Failed: %v\n", time.Now().Format("2006-01-02 15:04:05"), err)
		return
	}

	// 第二步使用这把锁，这里就模拟多个节点竞争执行的情况，他们会线程安全的往resource里写数据
	resource := strings.Builder{}
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		workerId := fmt.Sprintf("worker-%d", i)
		wg.Add(1)
		go func() {
			defer wg.Done()

			// 获取锁
			err := lock.Lock(context.Background(), workerId)
			if err != nil {
				fmt.Printf("[ %s ] workerId = %s, lock failed: %v \n", time.Now().Format("2006-01-02 15:04:05"), workerId, err)
				return
			}
			// 退出的时候释放锁
			defer func() {
				err := lock.UnLock(context.Background(), workerId)
				if err != nil {
					fmt.Printf("[ %s ] workerId = %s, unlock failed: %v \n", time.Now().Format("2006-01-02 15:04:05"), workerId, err)
					return
				}
			}()

			// 假装有耗时的操作
			fmt.Printf("[ %s ] workerId = %s, begin write resource \n", time.Now().Format("2006-01-02 15:04:05"), workerId)
			time.Sleep(time.Second * 3)
			// 接下来是操作竞态资源
			resource.WriteString(workerId)
			fmt.Printf("[ %s ] workerId = %s, write resource done \n", time.Now().Format("2006-01-02 15:04:05"), workerId)
			resource.WriteString("\n")

		}()
	}
	wg.Wait()

	// 观察最终的输出是否和日志一致
	fmt.Printf("[ %s ] Resource: \n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Println(resource.String())

	// Output:
	// [ 2023-03-13 01:01:09 ] workerId = worker-0, begin write resource
	//[ 2023-03-13 01:01:09 ] workerId = worker-0, begin write resource
	//[ 2023-03-13 01:01:12 ] workerId = worker-0, write resource done
	//[ 2023-03-13 01:01:12 ] workerId = worker-6, begin write resource
	//[ 2023-03-13 01:01:15 ] workerId = worker-6, write resource done
	//[ 2023-03-13 01:01:15 ] workerId = worker-9, begin write resource
	//[ 2023-03-13 01:01:18 ] workerId = worker-9, write resource done
	//[ 2023-03-13 01:01:19 ] workerId = worker-2, begin write resource
	//[ 2023-03-13 01:01:22 ] workerId = worker-2, write resource done
	//[ 2023-03-13 01:01:22 ] workerId = worker-8, begin write resource
	//[ 2023-03-13 01:01:25 ] workerId = worker-8, write resource done
	//[ 2023-03-13 01:01:27 ] workerId = worker-4, begin write resource
	//[ 2023-03-13 01:01:30 ] workerId = worker-4, write resource done
	//[ 2023-03-13 01:01:32 ] workerId = worker-7, begin write resource
	//[ 2023-03-13 01:01:35 ] workerId = worker-7, write resource done
	//[ 2023-03-13 01:01:36 ] workerId = worker-1, begin write resource
	//[ 2023-03-13 01:01:39 ] workerId = worker-1, write resource done
	//[ 2023-03-13 01:01:40 ] workerId = worker-3, begin write resource
	//[ 2023-03-13 01:01:43 ] workerId = worker-3, write resource done
	//[ 2023-03-13 01:01:46 ] workerId = worker-5, begin write resource
	//[ 2023-03-13 01:01:49 ] workerId = worker-5, write resource done
	//[ 2023-03-13 01:01:49 ] Resource:
	//worker-0
	//worker-6
	//worker-9
	//worker-2
	//worker-8
	//worker-4
	//worker-7
	//worker-1
	//worker-3
	//worker-5

}

```

### 3.1.2 详细配置

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

	// Docker启动MySQL：
	// docker run -itd --name storage-lock-mysql -p 3306:3306 -e MYSQL_ROOT_PASSWORD=UeGqAm8CxYGldMDLoNNt mysql:5.7

	// DSN的写法参考驱动的支持：github.com/go-sql-driver/mysql
	dsn := "root:UeGqAm8CxYGldMDLoNNt@tcp(192.168.128.206:3306)/storage_lock_test"

	// 第一步先配置存储介质相关的参数，包括如何连接到这个数据库，连接上去之后锁的信息存储到哪里等等
	// 配置如何连接到数据库
	connectionGetter := storage_lock.NewMySQLStorageConnectionGetterFromDSN(dsn)
	storageOptions := &storage_lock.MySQLStorageOptions{
		// 数据库连接获取方式，可以使用内置的从DSN获取连接，也可以自己实现接口决定如何连接
		ConnectionGetter: connectionGetter,
		// 锁的信息是存储在哪张表中的，不设置的话默认为storage_lock
		TableName: "storage_lock_table",
	}
	storage, err := storage_lock.NewMySQLStorage(context.Background(), storageOptions)
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
	for i := 0; i < 10; i++ {
		workerId := fmt.Sprintf("worker-%d", i)
		wg.Add(1)
		go func() {
			defer wg.Done()

			// 获取锁
			err := lock.Lock(context.Background(), workerId)
			if err != nil {
				fmt.Printf("[ %s ] workerId = %s, lock failed: %v \n", time.Now().Format("2006-01-02 15:04:05"), workerId, err)
				return
			}
			// 退出的时候释放锁
			defer func() {
				err := lock.UnLock(context.Background(), workerId)
				if err != nil {
					fmt.Printf("[ %s ] workerId = %s, unlock failed: %v \n", time.Now().Format("2006-01-02 15:04:05"), workerId, err)
					return
				}
			}()

			// 假装有耗时的操作
			fmt.Printf("[ %s ] workerId = %s, begin write resource \n", time.Now().Format("2006-01-02 15:04:05"), workerId)
			time.Sleep(time.Second * 3)
			// 接下来是操作竞态资源
			resource.WriteString(workerId)
			fmt.Printf("[ %s ] workerId = %s, write resource done \n", time.Now().Format("2006-01-02 15:04:05"), workerId)
			resource.WriteString("\n")

		}()
	}
	wg.Wait()

	// 观察最终的输出是否和日志一致
	fmt.Printf("[ %s ] Resource: \n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Println(resource.String())

	// Output:
	// [ 2023-03-13 01:02:51 ] workerId = worker-9, begin write resource
	//[ 2023-03-13 01:02:54 ] workerId = worker-9, write resource done
	//[ 2023-03-13 01:02:55 ] workerId = worker-2, begin write resource
	//[ 2023-03-13 01:02:58 ] workerId = worker-2, write resource done
	//[ 2023-03-13 01:02:59 ] workerId = worker-8, begin write resource
	//[ 2023-03-13 01:03:02 ] workerId = worker-8, write resource done
	//[ 2023-03-13 01:03:02 ] workerId = worker-0, begin write resource
	//[ 2023-03-13 01:03:05 ] workerId = worker-0, write resource done
	//[ 2023-03-13 01:03:05 ] workerId = worker-3, begin write resource
	//[ 2023-03-13 01:03:08 ] workerId = worker-3, write resource done
	//[ 2023-03-13 01:03:09 ] workerId = worker-5, begin write resource
	//[ 2023-03-13 01:03:12 ] workerId = worker-5, write resource done
	//[ 2023-03-13 01:03:14 ] workerId = worker-6, begin write resource
	//[ 2023-03-13 01:03:17 ] workerId = worker-6, write resource done
	//[ 2023-03-13 01:03:18 ] workerId = worker-1, begin write resource
	//[ 2023-03-13 01:03:21 ] workerId = worker-1, write resource done
	//[ 2023-03-13 01:03:24 ] workerId = worker-7, begin write resource
	//[ 2023-03-13 01:03:27 ] workerId = worker-7, write resource done
	//[ 2023-03-13 01:03:29 ] workerId = worker-4, begin write resource
	//[ 2023-03-13 01:03:32 ] workerId = worker-4, write resource done
	//[ 2023-03-13 01:03:32 ] Resource:
	//worker-9
	//worker-2
	//worker-8
	//worker-0
	//worker-3
	//worker-5
	//worker-6
	//worker-1
	//worker-7
	//worker-4

}

```


## 3.2 Postgresql

### 3.2.1 快速开始 

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

	// Docker启动Postgresql：
	// docker run -d --name storage-lock-postgres -p 5432:5432 -e POSTGRES_PASSWORD=UeGqAm8CxYGldMDLoNNt postgres:14

	// DSN的写法参考驱动的支持：https://github.com/lib/pq
	dsn := "host=192.168.128.206 user=postgres password=UeGqAm8CxYGldMDLoNNt port=5432 dbname=postgres sslmode=disable"

	// 这个是最为重要的，通常是要锁住的资源的名称
	lockId := "must-serial-operation-resource-foo"

	// 第一步创建一把分布式锁
	lock, err := storage_lock.NewPostgreSQLStorageLock(context.Background(), lockId, dsn)
	if err != nil {
		fmt.Printf("[ %s ] Create Lock Failed: %v\n", time.Now().Format("2006-01-02 15:04:05"), err)
		return
	}

	// 第二步使用这把锁，这里就模拟多个节点竞争执行的情况，他们会线程安全的往resource里写数据
	resource := strings.Builder{}
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		workerId := fmt.Sprintf("worker-%d", i)
		wg.Add(1)
		go func() {
			defer wg.Done()

			// 获取锁
			err := lock.Lock(context.Background(), workerId)
			if err != nil {
				fmt.Printf("[ %s ] workerId = %s, lock failed: %v \n", time.Now().Format("2006-01-02 15:04:05"), workerId, err)
				return
			}
			// 退出的时候释放锁
			defer func() {
				err := lock.UnLock(context.Background(), workerId)
				if err != nil {
					fmt.Printf("[ %s ] workerId = %s, unlock failed: %v \n", time.Now().Format("2006-01-02 15:04:05"), workerId, err)
					return
				}
			}()

			// 假装有耗时的操作
			fmt.Printf("[ %s ] workerId = %s, begin write resource \n", time.Now().Format("2006-01-02 15:04:05"), workerId)
			time.Sleep(time.Second * 3)
			// 接下来是操作竞态资源
			resource.WriteString(workerId)
			fmt.Printf("[ %s ] workerId = %s, write resource done \n", time.Now().Format("2006-01-02 15:04:05"), workerId)
			resource.WriteString("\n")

		}()
	}
	wg.Wait()

	// 观察最终的输出是否和日志一致
	fmt.Printf("[ %s ] Resource: \n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Println(resource.String())

	// Output:
	// [ 2023-03-13 00:29:37 ] workerId = worker-3, begin write resource
	// [ 2023-03-13 00:29:40 ] workerId = worker-3, write resource done
	// [ 2023-03-13 00:29:40 ] workerId = worker-5, begin write resource
	// [ 2023-03-13 00:29:43 ] workerId = worker-5, write resource done
	// [ 2023-03-13 00:29:43 ] workerId = worker-8, begin write resource
	// [ 2023-03-13 00:29:46 ] workerId = worker-8, write resource done
	// [ 2023-03-13 00:29:46 ] workerId = worker-6, begin write resource
	// [ 2023-03-13 00:29:49 ] workerId = worker-6, write resource done
	// [ 2023-03-13 00:29:50 ] workerId = worker-2, begin write resource
	// [ 2023-03-13 00:29:53 ] workerId = worker-2, write resource done
	// [ 2023-03-13 00:29:56 ] workerId = worker-0, begin write resource
	// [ 2023-03-13 00:29:59 ] workerId = worker-0, write resource done
	// [ 2023-03-13 00:30:00 ] workerId = worker-1, begin write resource
	// [ 2023-03-13 00:30:03 ] workerId = worker-1, write resource done
	// [ 2023-03-13 00:30:04 ] workerId = worker-4, begin write resource
	// [ 2023-03-13 00:30:07 ] workerId = worker-4, write resource done
	// [ 2023-03-13 00:30:08 ] workerId = worker-9, begin write resource
	// [ 2023-03-13 00:30:11 ] workerId = worker-9, write resource done
	// [ 2023-03-13 00:30:14 ] workerId = worker-7, begin write resource
	// [ 2023-03-13 00:30:18 ] workerId = worker-7, write resource done
	// [ 2023-03-13 00:30:18 ] Resource:
	// worker-3
	// worker-5
	// worker-8
	// worker-6
	// worker-2
	// worker-0
	// worker-1
	// worker-4
	// worker-9
	// worker-7

}

```

### 3.2.2 详细配置
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

	// Docker启动Postgresql：
	// docker run -d --name storage-lock-postgres -p 5432:5432 -e POSTGRES_PASSWORD=UeGqAm8CxYGldMDLoNNt postgres:14

	// DSN的写法参考驱动的支持：https://github.com/lib/pq
	dsn := "host=192.168.128.206 user=postgres password=UeGqAm8CxYGldMDLoNNt port=5432 dbname=postgres sslmode=disable"

	// 第一步先配置存储介质相关的参数，包括如何连接到这个数据库，连接上去之后锁的信息存储到哪里等等
	// 配置如何连接到数据库
	connectionGetter := storage_lock.NewPostgreSQLStorageConnectionGetterFromDSN(dsn)
	storageOptions := &storage_lock.PostgreSQLStorageOptions{
		// 数据库连接获取方式，可以使用内置的从DSN获取连接，也可以自己实现接口决定如何连接
		ConnectionGetter: connectionGetter,
		// 选择锁信息存放在哪个schema下，默认为public
		Schema: "public",
		// 锁的信息是存储在哪张表中的，不设置的话默认为storage_lock
		TableName: "storage_lock_table",
	}
	storage, err := storage_lock.NewPostgreSQLStorage(context.Background(), storageOptions)
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
	for i := 0; i < 10; i++ {
		workerId := fmt.Sprintf("worker-%d", i)
		wg.Add(1)
		go func() {
			defer wg.Done()

			// 获取锁
			err := lock.Lock(context.Background(), workerId)
			if err != nil {
				fmt.Printf("[ %s ] workerId = %s, lock failed: %v \n", time.Now().Format("2006-01-02 15:04:05"), workerId, err)
				return
			}
			// 退出的时候释放锁
			defer func() {
				err := lock.UnLock(context.Background(), workerId)
				if err != nil {
					fmt.Printf("[ %s ] workerId = %s, unlock failed: %v \n", time.Now().Format("2006-01-02 15:04:05"), workerId, err)
					return
				}
			}()

			// 假装有耗时的操作
			fmt.Printf("[ %s ] workerId = %s, begin write resource \n", time.Now().Format("2006-01-02 15:04:05"), workerId)
			time.Sleep(time.Second * 3)
			// 接下来是操作竞态资源
			resource.WriteString(workerId)
			fmt.Printf("[ %s ] workerId = %s, write resource done \n", time.Now().Format("2006-01-02 15:04:05"), workerId)
			resource.WriteString("\n")

		}()
	}
	wg.Wait()

	// 观察最终的输出是否和日志一致
	fmt.Printf("[ %s ] Resource: \n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Println(resource.String())

	// Output:
	// [ 2023-03-13 00:33:38 ] workerId = worker-0, begin write resource
	// [ 2023-03-13 00:33:41 ] workerId = worker-0, write resource done
	// [ 2023-03-13 00:33:42 ] workerId = worker-3, begin write resource
	// [ 2023-03-13 00:33:45 ] workerId = worker-3, write resource done
	// [ 2023-03-13 00:33:45 ] workerId = worker-6, begin write resource
	// [ 2023-03-13 00:33:48 ] workerId = worker-6, write resource done
	// [ 2023-03-13 00:33:49 ] workerId = worker-5, begin write resource
	// [ 2023-03-13 00:33:52 ] workerId = worker-5, write resource done
	// [ 2023-03-13 00:33:53 ] workerId = worker-2, begin write resource
	// [ 2023-03-13 00:33:56 ] workerId = worker-2, write resource done
	// [ 2023-03-13 00:33:57 ] workerId = worker-8, begin write resource
	// [ 2023-03-13 00:34:00 ] workerId = worker-8, write resource done
	// [ 2023-03-13 00:34:01 ] workerId = worker-4, begin write resource
	// [ 2023-03-13 00:34:04 ] workerId = worker-4, write resource done
	// [ 2023-03-13 00:34:04 ] workerId = worker-1, begin write resource
	// [ 2023-03-13 00:34:07 ] workerId = worker-1, write resource done
	// [ 2023-03-13 00:34:08 ] workerId = worker-9, begin write resource
	// [ 2023-03-13 00:34:11 ] workerId = worker-9, write resource done
	// [ 2023-03-13 00:34:11 ] workerId = worker-7, begin write resource
	// [ 2023-03-13 00:34:14 ] workerId = worker-7, write resource done
	// [ 2023-03-13 00:34:14 ] Resource:
	// worker-0
	// worker-3
	// worker-6
	// worker-5
	// worker-2
	// worker-8
	// worker-4
	// worker-1
	// worker-9
	// worker-7

}

```


## 3.3 SQLServer

### 3.3.1 快速开始

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

	// Docker快速启动SQLServer：
	// docker run -e "ACCEPT_EULA=Y" -e "MSSQL_SA_PASSWORD=UeGqAm8CxYGldMDLoNNt" \
	//   -p 1433:1433 --name storage-lock-sql1 --hostname sql1 \
	//   -d \
	//   mcr.microsoft.com/mssql/server:2022-latest

	// DSN的写法参考驱动的支持：https://github.com/denisenkom/go-mssqldb
	dsn := "sqlserver://sa:UeGqAm8CxYGldMDLoNNt@192.168.128.206:1433?database=storage_lock_test&connection+timeout=30"

	// 这个是最为重要的，通常是要锁住的资源的名称
	lockId := "must-serial-operation-resource-foo"

	// 第一步创建一把分布式锁
	lock, err := storage_lock.NewSqlServerStorageLock(context.Background(), lockId, dsn)
	if err != nil {
		fmt.Printf("[ %s ] Create Lock Failed: %v\n", time.Now().Format("2006-01-02 15:04:05"), err)
		return
	}

	// 第二步使用这把锁，这里就模拟多个节点竞争执行的情况，他们会线程安全的往resource里写数据
	resource := strings.Builder{}
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		workerId := fmt.Sprintf("worker-%d", i)
		wg.Add(1)
		go func() {
			defer wg.Done()

			// 获取锁
			err := lock.Lock(context.Background(), workerId)
			if err != nil {
				fmt.Printf("[ %s ] workerId = %s, lock failed: %v \n", time.Now().Format("2006-01-02 15:04:05"), workerId, err)
				return
			}
			// 退出的时候释放锁
			defer func() {
				err := lock.UnLock(context.Background(), workerId)
				if err != nil {
					fmt.Printf("[ %s ] workerId = %s, unlock failed: %v \n", time.Now().Format("2006-01-02 15:04:05"), workerId, err)
					return
				}
			}()

			// 假装有耗时的操作
			fmt.Printf("[ %s ] workerId = %s, begin write resource \n", time.Now().Format("2006-01-02 15:04:05"), workerId)
			time.Sleep(time.Second * 3)
			// 接下来是操作竞态资源
			resource.WriteString(workerId)
			fmt.Printf("[ %s ] workerId = %s, write resource done \n", time.Now().Format("2006-01-02 15:04:05"), workerId)
			resource.WriteString("\n")

		}()
	}
	wg.Wait()

	// 观察最终的输出是否和日志一致
	fmt.Printf("[ %s ] Resource: \n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Println(resource.String())

	// Output:
	// [ 2023-03-13 00:00:05 ] workerId = worker-2, begin write resource
	// [ 2023-03-13 00:00:08 ] workerId = worker-2, write resource done
	// [ 2023-03-13 00:00:08 ] workerId = worker-8, begin write resource
	// [ 2023-03-13 00:00:11 ] workerId = worker-8, write resource done
	// [ 2023-03-13 00:00:11 ] workerId = worker-7, begin write resource
	// [ 2023-03-13 00:00:14 ] workerId = worker-7, write resource done
	// [ 2023-03-13 00:00:15 ] workerId = worker-1, begin write resource
	// [ 2023-03-13 00:00:18 ] workerId = worker-1, write resource done
	// [ 2023-03-13 00:00:18 ] workerId = worker-3, begin write resource
	// [ 2023-03-13 00:00:21 ] workerId = worker-3, write resource done
	// [ 2023-03-13 00:00:23 ] workerId = worker-4, begin write resource
	// [ 2023-03-13 00:00:26 ] workerId = worker-4, write resource done
	// [ 2023-03-13 00:00:28 ] workerId = worker-6, begin write resource
	// [ 2023-03-13 00:00:31 ] workerId = worker-6, write resource done
	// [ 2023-03-13 00:00:32 ] workerId = worker-0, begin write resource
	// [ 2023-03-13 00:00:35 ] workerId = worker-0, write resource done
	// [ 2023-03-13 00:00:35 ] workerId = worker-9, begin write resource
	// [ 2023-03-13 00:00:38 ] workerId = worker-9, write resource done
	// [ 2023-03-13 00:00:38 ] workerId = worker-5, begin write resource
	// [ 2023-03-13 00:00:41 ] workerId = worker-5, write resource done
	// [ 2023-03-13 00:00:41 ] Resource:
	// worker-2
	// worker-8
	// worker-7
	// worker-1
	// worker-3
	// worker-4
	// worker-6
	// worker-0
	// worker-9
	// worker-5

}

```

### 3.3.2 详细配置

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

	// Docker快速启动SQLServer：
	// docker run -e "ACCEPT_EULA=Y" -e "MSSQL_SA_PASSWORD=UeGqAm8CxYGldMDLoNNt" \
	//   -p 1433:1433 --name storage-lock-sql1 --hostname sql1 \
	//   -d \
	//   mcr.microsoft.com/mssql/server:2022-latest

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
	for i := 0; i < 10; i++ {
		workerId := fmt.Sprintf("worker-%d", i)
		wg.Add(1)
		go func() {
			defer wg.Done()

			// 获取锁
			err := lock.Lock(context.Background(), workerId)
			if err != nil {
				fmt.Printf("[ %s ] workerId = %s, lock failed: %v \n", time.Now().Format("2006-01-02 15:04:05"), workerId, err)
				return
			}
			// 退出的时候释放锁
			defer func() {
				err := lock.UnLock(context.Background(), workerId)
				if err != nil {
					fmt.Printf("[ %s ] workerId = %s, unlock failed: %v \n", time.Now().Format("2006-01-02 15:04:05"), workerId, err)
					return
				}
			}()

			// 假装有耗时的操作
			fmt.Printf("[ %s ] workerId = %s, begin write resource \n", time.Now().Format("2006-01-02 15:04:05"), workerId)
			time.Sleep(time.Second * 3)
			// 接下来是操作竞态资源
			resource.WriteString(workerId)
			fmt.Printf("[ %s ] workerId = %s, write resource done \n", time.Now().Format("2006-01-02 15:04:05"), workerId)
			resource.WriteString("\n")

		}()
	}
	wg.Wait()

	// 观察最终的输出是否和日志一致
	fmt.Printf("[ %s ] Resource: \n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Println(resource.String())

	// Output:
	// [ 2023-03-13 00:01:32 ] workerId = worker-0, begin write resource
	// [ 2023-03-13 00:01:35 ] workerId = worker-0, write resource done
	// [ 2023-03-13 00:01:35 ] workerId = worker-5, begin write resource
	// [ 2023-03-13 00:01:38 ] workerId = worker-5, write resource done
	// [ 2023-03-13 00:01:38 ] workerId = worker-1, begin write resource
	// [ 2023-03-13 00:01:41 ] workerId = worker-1, write resource done
	// [ 2023-03-13 00:01:42 ] workerId = worker-9, begin write resource
	// [ 2023-03-13 00:01:45 ] workerId = worker-9, write resource done
	// [ 2023-03-13 00:01:46 ] workerId = worker-4, begin write resource
	// [ 2023-03-13 00:01:49 ] workerId = worker-4, write resource done
	// [ 2023-03-13 00:01:50 ] workerId = worker-3, begin write resource
	// [ 2023-03-13 00:01:53 ] workerId = worker-3, write resource done
	// [ 2023-03-13 00:01:55 ] workerId = worker-2, begin write resource
	// [ 2023-03-13 00:01:58 ] workerId = worker-2, write resource done
	// [ 2023-03-13 00:01:58 ] workerId = worker-6, begin write resource
	// [ 2023-03-13 00:02:01 ] workerId = worker-6, write resource done
	// [ 2023-03-13 00:02:02 ] workerId = worker-7, begin write resource
	// [ 2023-03-13 00:02:05 ] workerId = worker-7, write resource done
	// [ 2023-03-13 00:02:06 ] workerId = worker-8, begin write resource
	// [ 2023-03-13 00:02:09 ] workerId = worker-8, write resource done
	// [ 2023-03-13 00:02:09 ] Resource:
	// worker-0
	// worker-5
	// worker-1
	// worker-9
	// worker-4
	// worker-3
	// worker-2
	// worker-6
	// worker-7
	// worker-8

}

```

## 3.4 Mongo

### 3.4.1 快速开始

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

	// Docker启动Mongo：
	// docker run -d -p 27017:27017 --name storage-lock-mongodb -e MONGO_INITDB_ROOT_USERNAME=root -e MONGO_INITDB_ROOT_PASSWORD=UeGqAm8CxYGldMDLoNNt mongo

	uri := "mongodb://root:UeGqAm8CxYGldMDLoNNt@192.168.128.206:27017/?connectTimeoutMS=300000"

	// 这个是最为重要的，通常是要锁住的资源的名称
	lockId := "must-serial-operation-resource-foo"

	// 第一步创建一把分布式锁
	lock, err := storage_lock.NewMongoStorageLock(context.Background(), lockId, uri)
	if err != nil {
		fmt.Printf("[ %s ] Create Lock Failed: %v\n", time.Now().Format("2006-01-02 15:04:05"), err)
		return
	}

	// 第二步使用这把锁，这里就模拟多个节点竞争执行的情况，他们会线程安全的往resource里写数据
	resource := strings.Builder{}
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		workerId := fmt.Sprintf("worker-%d", i)
		wg.Add(1)
		go func() {
			defer wg.Done()

			// 获取锁
			err := lock.Lock(context.Background(), workerId)
			if err != nil {
				fmt.Printf("[ %s ] workerId = %s, lock failed: %v \n", time.Now().Format("2006-01-02 15:04:05"), workerId, err)
				return
			}
			// 退出的时候释放锁
			defer func() {
				err := lock.UnLock(context.Background(), workerId)
				if err != nil {
					fmt.Printf("[ %s ] workerId = %s, unlock failed: %v \n", time.Now().Format("2006-01-02 15:04:05"), workerId, err)
					return
				}
			}()

			// 假装有耗时的操作
			fmt.Printf("[ %s ] workerId = %s, begin write resource \n", time.Now().Format("2006-01-02 15:04:05"), workerId)
			time.Sleep(time.Second * 3)
			// 接下来是操作竞态资源
			resource.WriteString(workerId)
			fmt.Printf("[ %s ] workerId = %s, write resource done \n", time.Now().Format("2006-01-02 15:04:05"), workerId)
			resource.WriteString("\n")

		}()
	}
	wg.Wait()

	// 观察最终的输出是否和日志一致
	fmt.Printf("[ %s ] Resource: \n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Println(resource.String())

	// Output:
	// [ 2023-03-14 01:40:17 ] workerId = worker-1, begin write resource
	//[ 2023-03-14 01:40:20 ] workerId = worker-1, write resource done
	//[ 2023-03-14 01:40:20 ] workerId = worker-2, begin write resource
	//[ 2023-03-14 01:40:23 ] workerId = worker-2, write resource done
	//[ 2023-03-14 01:40:23 ] workerId = worker-9, begin write resource
	//[ 2023-03-14 01:40:26 ] workerId = worker-9, write resource done
	//[ 2023-03-14 01:40:27 ] workerId = worker-5, begin write resource
	//[ 2023-03-14 01:40:30 ] workerId = worker-5, write resource done
	//[ 2023-03-14 01:40:30 ] workerId = worker-8, begin write resource
	//[ 2023-03-14 01:40:33 ] workerId = worker-8, write resource done
	//[ 2023-03-14 01:40:35 ] workerId = worker-4, begin write resource
	//[ 2023-03-14 01:40:38 ] workerId = worker-4, write resource done
	//[ 2023-03-14 01:40:40 ] workerId = worker-0, begin write resource
	//[ 2023-03-14 01:40:43 ] workerId = worker-0, write resource done
	//[ 2023-03-14 01:40:44 ] workerId = worker-6, begin write resource
	//[ 2023-03-14 01:40:47 ] workerId = worker-6, write resource done
	//[ 2023-03-14 01:40:47 ] workerId = worker-3, begin write resource
	//[ 2023-03-14 01:40:50 ] workerId = worker-3, write resource done
	//[ 2023-03-14 01:40:50 ] workerId = worker-7, begin write resource
	//[ 2023-03-14 01:40:53 ] workerId = worker-7, write resource done
	//[ 2023-03-14 01:40:53 ] Resource:
	//worker-1
	//worker-2
	//worker-9
	//worker-5
	//worker-8
	//worker-4
	//worker-0
	//worker-6
	//worker-3
	//worker-7

}

```

### 3.4.2 详细配置

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

	// Docker启动Mongo：
	// docker run -d -p 27017:27017 --name storage-lock-mongodb -e MONGO_INITDB_ROOT_USERNAME=root -e MONGO_INITDB_ROOT_PASSWORD=UeGqAm8CxYGldMDLoNNt mongo

	uri := "mongodb://root:UeGqAm8CxYGldMDLoNNt@192.168.128.206:27017/?connectTimeoutMS=300000"

	// 第一步先配置存储介质相关的参数，包括如何连接到这个数据库，连接上去之后锁的信息存储到哪里等等
	// 配置如何连接到数据库
	connectionGetter := storage_lock.NewMongoConfigurationConnectionGetter(uri)
	storageOptions := &storage_lock.MongoStorageOptions{
		// 数据库连接获取方式，可以使用内置的从DSN获取连接，也可以自己实现接口决定如何连接
		ConnectionGetter: connectionGetter,
		// 锁的信息是存放在哪个数据库中
		DatabaseName: "storage_lock_table",
		// 锁的信息是存储在哪张表中的，不设置的话默认为storage_lock
		CollectionName: "storage_lock_table",
	}
	storage, err := storage_lock.NewMongoStorage(context.Background(), storageOptions)
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
	for i := 0; i < 10; i++ {
		workerId := fmt.Sprintf("worker-%d", i)
		wg.Add(1)
		go func() {
			defer wg.Done()

			// 获取锁
			err := lock.Lock(context.Background(), workerId)
			if err != nil {
				fmt.Printf("[ %s ] workerId = %s, lock failed: %v \n", time.Now().Format("2006-01-02 15:04:05"), workerId, err)
				return
			}
			// 退出的时候释放锁
			defer func() {
				err := lock.UnLock(context.Background(), workerId)
				if err != nil {
					fmt.Printf("[ %s ] workerId = %s, unlock failed: %v \n", time.Now().Format("2006-01-02 15:04:05"), workerId, err)
					return
				}
			}()

			// 假装有耗时的操作
			fmt.Printf("[ %s ] workerId = %s, begin write resource \n", time.Now().Format("2006-01-02 15:04:05"), workerId)
			time.Sleep(time.Second * 3)
			// 接下来是操作竞态资源
			resource.WriteString(workerId)
			fmt.Printf("[ %s ] workerId = %s, write resource done \n", time.Now().Format("2006-01-02 15:04:05"), workerId)
			resource.WriteString("\n")

		}()
	}
	wg.Wait()

	// 观察最终的输出是否和日志一致
	fmt.Printf("[ %s ] Resource: \n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Println(resource.String())

	// Output:
	// [ 2023-03-14 01:42:33 ] workerId = worker-8, write resource done
	//[ 2023-03-14 01:42:34 ] workerId = worker-6, begin write resource
	//[ 2023-03-14 01:42:37 ] workerId = worker-6, write resource done
	//[ 2023-03-14 01:42:37 ] workerId = worker-7, begin write resource
	//[ 2023-03-14 01:42:40 ] workerId = worker-7, write resource done
	//[ 2023-03-14 01:42:41 ] workerId = worker-5, begin write resource
	//[ 2023-03-14 01:42:44 ] workerId = worker-5, write resource done
	//[ 2023-03-14 01:42:44 ] workerId = worker-2, begin write resource
	//[ 2023-03-14 01:42:47 ] workerId = worker-2, write resource done
	//[ 2023-03-14 01:42:48 ] workerId = worker-0, begin write resource
	//[ 2023-03-14 01:42:51 ] workerId = worker-0, write resource done
	//[ 2023-03-14 01:42:54 ] workerId = worker-4, begin write resource
	//[ 2023-03-14 01:42:57 ] workerId = worker-4, write resource done
	//[ 2023-03-14 01:42:57 ] workerId = worker-1, begin write resource
	//[ 2023-03-14 01:43:00 ] workerId = worker-1, write resource done
	//[ 2023-03-14 01:43:00 ] workerId = worker-3, begin write resource
	//[ 2023-03-14 01:43:03 ] workerId = worker-3, write resource done
	//[ 2023-03-14 01:43:07 ] workerId = worker-9, begin write resource
	//[ 2023-03-14 01:43:10 ] workerId = worker-9, write resource done
	//[ 2023-03-14 01:43:10 ] Resource:
	//worker-8
	//worker-6
	//worker-7
	//worker-5
	//worker-2
	//worker-0
	//worker-4
	//worker-1
	//worker-3
	//worker-9

}

```



## 3.5 TIDB

TODO 

## 3.6 MariaDB

### 3.6.1 快速开始

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

	// Docker启动Maria：
	// docker run -p 3306:3306  --name storage-lock-mariadb -e MARIADB_ROOT_PASSWORD=UeGqAm8CxYGldMDLoNNt -d mariadb:latest

	// DSN的写法参考驱动的支持：github.com/go-sql-driver/mysql
	dsn := "root:UeGqAm8CxYGldMDLoNNt@tcp(192.168.128.206:3306)/storage_lock_test"

	// 这个是最为重要的，通常是要锁住的资源的名称
	lockId := "must-serial-operation-resource-foo"

	// 第一步创建一把分布式锁
	lock, err := storage_lock.NewMariaDBStorageLock(context.Background(), lockId, dsn)
	if err != nil {
		fmt.Printf("[ %s ] Create Lock Failed: %v\n", time.Now().Format("2006-01-02 15:04:05"), err)
		return
	}

	// 第二步使用这把锁，这里就模拟多个节点竞争执行的情况，他们会线程安全的往resource里写数据
	resource := strings.Builder{}
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		workerId := fmt.Sprintf("worker-%d", i)
		wg.Add(1)
		go func() {
			defer wg.Done()

			// 获取锁
			err := lock.Lock(context.Background(), workerId)
			if err != nil {
				fmt.Printf("[ %s ] workerId = %s, lock failed: %v \n", time.Now().Format("2006-01-02 15:04:05"), workerId, err)
				return
			}
			// 退出的时候释放锁
			defer func() {
				err := lock.UnLock(context.Background(), workerId)
				if err != nil {
					fmt.Printf("[ %s ] workerId = %s, unlock failed: %v \n", time.Now().Format("2006-01-02 15:04:05"), workerId, err)
					return
				}
			}()

			// 假装有耗时的操作
			fmt.Printf("[ %s ] workerId = %s, begin write resource \n", time.Now().Format("2006-01-02 15:04:05"), workerId)
			time.Sleep(time.Second * 3)
			// 接下来是操作竞态资源
			resource.WriteString(workerId)
			fmt.Printf("[ %s ] workerId = %s, write resource done \n", time.Now().Format("2006-01-02 15:04:05"), workerId)
			resource.WriteString("\n")

		}()
	}
	wg.Wait()

	// 观察最终的输出是否和日志一致
	fmt.Printf("[ %s ] Resource: \n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Println(resource.String())

	// Output:
	// [ 2023-03-13 01:11:02 ] workerId = worker-3, begin write resource
	//[ 2023-03-13 01:11:02 ] workerId = worker-3, begin write resource
	//[ 2023-03-13 01:11:02 ] workerId = worker-3, begin write resource
	//[ 2023-03-13 01:11:02 ] workerId = worker-3, begin write resource
	//[ 2023-03-13 01:11:02 ] workerId = worker-3, begin write resource
	//[ 2023-03-13 01:11:02 ] workerId = worker-3, begin write resource
	//[ 2023-03-13 01:11:05 ] workerId = worker-3, write resource done
	//[ 2023-03-13 01:11:05 ] workerId = worker-4, begin write resource
	//[ 2023-03-13 01:11:08 ] workerId = worker-4, write resource done
	//[ 2023-03-13 01:11:08 ] workerId = worker-5, begin write resource
	//[ 2023-03-13 01:11:11 ] workerId = worker-5, write resource done
	//[ 2023-03-13 01:11:11 ] workerId = worker-6, begin write resource
	//[ 2023-03-13 01:11:14 ] workerId = worker-6, write resource done
	//[ 2023-03-13 01:11:15 ] workerId = worker-1, begin write resource
	//[ 2023-03-13 01:11:18 ] workerId = worker-1, write resource done
	//[ 2023-03-13 01:11:18 ] workerId = worker-8, begin write resource
	//[ 2023-03-13 01:11:21 ] workerId = worker-8, write resource done
	//[ 2023-03-13 01:11:22 ] workerId = worker-9, begin write resource
	//[ 2023-03-13 01:11:25 ] workerId = worker-9, write resource done
	//[ 2023-03-13 01:11:26 ] workerId = worker-7, begin write resource
	//[ 2023-03-13 01:11:29 ] workerId = worker-7, write resource done
	//[ 2023-03-13 01:11:31 ] workerId = worker-0, begin write resource
	//[ 2023-03-13 01:11:34 ] workerId = worker-0, write resource done
	//[ 2023-03-13 01:11:39 ] workerId = worker-2, begin write resource
	//[ 2023-03-13 01:11:42 ] workerId = worker-2, write resource done
	//[ 2023-03-13 01:11:42 ] Resource:
	//worker-3
	//worker-4
	//worker-5
	//worker-6
	//worker-1
	//worker-8
	//worker-9
	//worker-7
	//worker-0
	//worker-2

}

```

### 3.6.2 详细配置

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

	// Docker启动Maria：
	// docker run -p 3306:3306  --name storage-lock-mariadb -e MARIADB_ROOT_PASSWORD=UeGqAm8CxYGldMDLoNNt -d mariadb:latest

	// DSN的写法参考驱动的支持：github.com/go-sql-driver/mysql
	dsn := "root:UeGqAm8CxYGldMDLoNNt@tcp(192.168.128.206:3306)/storage_lock_test"

	// 第一步先配置存储介质相关的参数，包括如何连接到这个数据库，连接上去之后锁的信息存储到哪里等等
	// 配置如何连接到数据库
	connectionGetter := storage_lock.NewMariaStorageConnectionGetterFromDSN(dsn)
	storageOptions := &storage_lock.MariaStorageOptions{
		// 数据库连接获取方式，可以使用内置的从DSN获取连接，也可以自己实现接口决定如何连接
		ConnectionGetter: connectionGetter,
		// 锁的信息是存储在哪张表中的，不设置的话默认为storage_lock
		TableName: "storage_lock_table",
	}
	storage, err := storage_lock.NewMariaDbStorage(context.Background(), storageOptions)
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
	for i := 0; i < 10; i++ {
		workerId := fmt.Sprintf("worker-%d", i)
		wg.Add(1)
		go func() {
			defer wg.Done()

			// 获取锁
			err := lock.Lock(context.Background(), workerId)
			if err != nil {
				fmt.Printf("[ %s ] workerId = %s, lock failed: %v \n", time.Now().Format("2006-01-02 15:04:05"), workerId, err)
				return
			}
			// 退出的时候释放锁
			defer func() {
				err := lock.UnLock(context.Background(), workerId)
				if err != nil {
					fmt.Printf("[ %s ] workerId = %s, unlock failed: %v \n", time.Now().Format("2006-01-02 15:04:05"), workerId, err)
					return
				}
			}()

			// 假装有耗时的操作
			fmt.Printf("[ %s ] workerId = %s, begin write resource \n", time.Now().Format("2006-01-02 15:04:05"), workerId)
			time.Sleep(time.Second * 3)
			// 接下来是操作竞态资源
			resource.WriteString(workerId)
			fmt.Printf("[ %s ] workerId = %s, write resource done \n", time.Now().Format("2006-01-02 15:04:05"), workerId)
			resource.WriteString("\n")

		}()
	}
	wg.Wait()

	// 观察最终的输出是否和日志一致
	fmt.Printf("[ %s ] Resource: \n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Println(resource.String())

	// Output:
	// [ 2023-03-13 01:12:29 ] workerId = worker-1, begin write resource
	//[ 2023-03-13 01:12:32 ] workerId = worker-1, write resource done
	//[ 2023-03-13 01:12:32 ] workerId = worker-6, begin write resource
	//[ 2023-03-13 01:12:35 ] workerId = worker-6, write resource done
	//[ 2023-03-13 01:12:35 ] workerId = worker-0, begin write resource
	//[ 2023-03-13 01:12:38 ] workerId = worker-0, write resource done
	//[ 2023-03-13 01:12:39 ] workerId = worker-4, begin write resource
	//[ 2023-03-13 01:12:42 ] workerId = worker-4, write resource done
	//[ 2023-03-13 01:12:42 ] workerId = worker-2, begin write resource
	//[ 2023-03-13 01:12:45 ] workerId = worker-2, write resource done
	//[ 2023-03-13 01:12:47 ] workerId = worker-3, begin write resource
	//[ 2023-03-13 01:12:50 ] workerId = worker-3, write resource done
	//[ 2023-03-13 01:12:52 ] workerId = worker-5, begin write resource
	//[ 2023-03-13 01:12:55 ] workerId = worker-5, write resource done
	//[ 2023-03-13 01:12:56 ] workerId = worker-9, begin write resource
	//[ 2023-03-13 01:12:59 ] workerId = worker-9, write resource done
	//[ 2023-03-13 01:12:59 ] workerId = worker-7, begin write resource
	//[ 2023-03-13 01:13:02 ] workerId = worker-7, write resource done
	//[ 2023-03-13 01:13:03 ] workerId = worker-8, begin write resource
	//[ 2023-03-13 01:13:06 ] workerId = worker-8, write resource done
	//[ 2023-03-13 01:13:06 ] Resource:
	//worker-1
	//worker-6
	//worker-0
	//worker-4
	//worker-2
	//worker-3
	//worker-5
	//worker-9
	//worker-7
	//worker-8

}

```





# 五、Storage Lock分布式锁算法原理详解

TODO 



# 六、TODO

- 增加分布式锁日志表以备查问题 
- 



