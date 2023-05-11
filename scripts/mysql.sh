#!/usr/bin/env bash

# 删除可能存在的容器
docker rm -f storage-lock-mysql

# 启动MySQL实例，默认的用户名为root，密码为123456，监听在3306端口
docker run -itd --name storage-lock-mysql -p 3306:3306 -e MYSQL_ROOT_PASSWORD=UeGqAm8CxYGldMDLoNNt mysql:5.7

export STORAGE_LOCK_MYSQL_DSN="root:UeGqAm8CxYGldMDLoNNt@tcp(192.168.128.206:3306)/storage_lock_test"

