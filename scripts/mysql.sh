#!/usr/bin/env bash

# 启动MySQL实例，默认的用户名为root，密码为123456，监听在3306端口
docker run -itd --name db-storage-mysql -p 3306:3306 -e MYSQL_ROOT_PASSWORD=123456 mysql:5.7

export STORAGE_LOCK_MYSQL_DSN="root:123456@tcp(192.168.128.206:3306)/storage_lock_test"
