#!/usr/bin/env bash

# 启动MySQL实例，默认的用户名为root，密码为123456，监听在3306端口
docker run -itd --name db-storage-mysql -p 3306:3306 -e MYSQL_ROOT_PASSWORD=123456 mysql:5.7
