#!/usr/bin/env bash

# 启动测试使用的Postgresql数据库
docker run -d --name storage-lock-postgres -p 5432:5432 -e POSTGRES_PASSWORD=123456 postgres:14
