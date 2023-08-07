# Storage Lock

# 一、这是什么

抽象了一套分布式锁的模型定义和算法，可以基于任何存储介质实现分布式锁！只要此存储介质可以被分布式访问即可，比如以数据库为存储介质，以KV为存储介质，以对象存储为存储介质，以任何可读写的服务为存储介质等等。

目前已经可以使用：

- 暂无

测试中，即将上线：

- 数据库
  - 关系型
    - [MySQL](https://github.com/storage-lock/go-mysql-locks)
    - [PostgreSQL](https://github.com/storage-lock/go-postgresql-locks)
    - [SQL Server](https://github.com/storage-lock/go-sqlserver-locks)
    - [MariaDB](https://github.com/storage-lock/go-mariadb-locks)
    - [TiDB](https://github.com/storage-lock/go-tidb-locks)
  - NoSQL
    - [MongoDB](https://github.com/storage-lock/go-mongodb-locks)
- ORM框架等
  - [GORM](https://github.com/storage-lock/go-gorm-locks)
  - [sqlx](https://github.com/storage-lock/go-sqlx-locks)
  - [gorp](https://github.com/storage-lock/go-gorp-locks)
  - [beego](https://github.com/storage-lock/go-beego-locks)
  - [xorm](https://github.com/storage-lock/go-xorm-locks)
- Golang通用
  - [*sql.DB](https://github.com/storage-lock/go-sqldb-locks)
- 内存
  - [memory](https://github.com/storage-lock/go-memory-locks)

开发中：

- FileSystem 

- Redis
- Oracle
- SQLite
- Zookeeper
- etcd
- Elasticsearch 

# 二、安装依赖

```bash
go get -u github.com/storage-lock/go-storage-lock
```

# 三、模型与算法介绍

TODO 2023-8-7 02:04:09 
