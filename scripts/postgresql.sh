#!/usr/bin/env bash

# 启动测试使用的Postgresql数据库
docker run -d --name storage-lock-postgres -p 5432:5432 -e POSTGRES_PASSWORD=123456 postgres:14

export STORAGE_LOCK_POSTGRESQL_DSN="host=192.168.128.206 user=postgres password=123456 port=5432 dbname=postgres sslmode=disable"

