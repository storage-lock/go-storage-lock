#!/usr/bin/env bash

# 先创建一个网络
docker network create storage-lock-cassandra-network

# 然后再启动Docker容器
docker run --name storage-lock-cassandra --network storage-lock-cassandra-network -d cassandra
