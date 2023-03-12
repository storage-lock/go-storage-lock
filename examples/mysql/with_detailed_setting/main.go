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
