package main

import (
	"context"
	"fmt"
	"github.com/storage-lock/go-storage-lock/pkg/locks"
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
	lock, err := locks.NewMariaDBStorageLock(context.Background(), lockId, dsn)
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
