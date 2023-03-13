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
