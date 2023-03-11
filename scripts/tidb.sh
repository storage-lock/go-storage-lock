#!/usr/bin/env bash

# 启动TiDB实例，默认的用户名为root，密码为空，监听在4000端口
docker run --name tidb-server -d -p 4000:4000 -p 10080:10080 pingcap/tidb:latest
