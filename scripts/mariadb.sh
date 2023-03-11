#!/usr/bin/env bash

docker run -p 3306:3306  --name storage-lock-mariadb -e MARIADB_ROOT_PASSWORD=123456 -d mariadb:latest

