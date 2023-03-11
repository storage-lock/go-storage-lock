#! /bin/bash

docker run -itd --name mongo -p 27017:27017 mongo --auth -e MONGO_INITDB_ROOT_USERNAME=admin -e MONGO_INITDB_ROOT_PASSWORD=123456

docker run --name tidb-server -d -p 4000:4000 -p 10080:10080 pingcap/tidb:latest
