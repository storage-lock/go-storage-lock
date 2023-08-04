package cassandra_storage

//import (
//	"database/sql"
//	"fmt"
//	"github.com/gocql/gocql"
//	"github.com/storage-lock/go-storage-lock/pkg/storage"
//	"sync"
//	"time"
//)
//
//// ------------------------------------------------- --------------------------------------------------------------------
//
//// ConnectionManager 创建一个MySQL的连接
//type ConnectionManager struct {
//
//	// 主机的名字
//	Host string
//
//	// 主机的端口
//	Port uint
//
//	// 用户名
//	User string
//
//	// 密码
//	Passwd string
//
//	DatabaseName string
//
//	// DSN
//	// Example: "root:UeGqAm8CxYGldMDLoNNt@tcp(192.168.128.206:3306)/storage_lock_test"
//	DSN string
//
//	// 初始化好的数据库实例
//	db   *sql.DB
//	err  error
//	once sync.Once
//}
//
//var _ storage.ConnectionManager[*sql.DB] = &ConnectionManager{}
//
//// NewMySQLStorageConnectionGetterFromDSN 从DSN创建MySQL连接
//func NewMySQLStorageConnectionGetterFromDSN(dsn string) *ConnectionManager {
//	return &ConnectionManager{
//		DSN: dsn,
//	}
//}
//
//// NewMySQLStorageConnectionGetter 从服务器属性创建数据库连接
//func NewMySQLStorageConnectionGetter(host string, port uint, user, passwd, database string) *ConnectionManager {
//	return &ConnectionManager{
//		Host:         host,
//		Port:         port,
//		User:         user,
//		Passwd:       passwd,
//		DatabaseName: database,
//	}
//}
//
//// Take 获取到数据库的连接
//func (x *ConnectionManager) Take(ctx context.Context) (*sql.DB, error) {
//	x.once.Do(func() {
//		//db, err := sql.Open("mysql", x.GetDSN())
//		//if err != nil {
//		//	x.err = err
//		//	return
//		//}
//		//x.db = db
//
//		cluster := gocql.NewCluster(x.GetDSN())
//		cluster.Keyspace = ""
//		cluster.Consistency = gocql.Quorum
//		//设置连接池的数量,默认是2个（针对每一个host,都建立起NumConns个连接）
//		cluster.NumConns = 3
//
//		session, _ := cluster.CreateSession()
//		time.Sleep(1 * time.Second) //Sleep so the fillPool can complete.
//		fmt.Println(session.Pool.Size())
//		defer session.Close()
//
//	})
//	return x.db, x.err
//}
//
//func (x *ConnectionManager) GetDSN() string {
//	if x.DSN != "" {
//		return x.DSN
//	}
//	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s", x.User, x.Passwd, x.Host, x.Port, x.DatabaseName)
//}
//
//// ------------------------------------------------- --------------------------------------------------------------------
//

