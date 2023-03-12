#!/usr/bin/env bash

docker run -e "ACCEPT_EULA=Y" -e "MSSQL_SA_PASSWORD=UeGqAm8CxYGldMDLoNNt" \
   -p 1433:1433 --name storage-lock-sql1 --hostname sql1 \
   -d \
   mcr.microsoft.com/mssql/server:2022-latest

export STORAGE_LOCK_SQLSERVER_DSN="sqlserver://sa:UeGqAm8CxYGldMDLoNNt@192.168.128.206:1433?database=storage_lock_test&connection+timeout=30"


