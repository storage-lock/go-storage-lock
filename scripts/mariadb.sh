#!/usr/bin/env bash

docker run -p 3306:3306  --name storage-lock-mariadb -e MARIADB_ROOT_PASSWORD=UeGqAm8CxYGldMDLoNNt -d mariadb:latest

export STORAGE_LOCK_MARIA_DSN="root:UeGqAm8CxYGldMDLoNNt@tcp(192.168.128.206:3306)/storage_lock_test"