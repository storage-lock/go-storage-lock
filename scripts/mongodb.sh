#!/usr/bin/env bash

docker run -d -p 27017:27017 --name storage-lock-mongodb -e MONGO_INITDB_ROOT_USERNAME=root -e MONGO_INITDB_ROOT_PASSWORD=123456 mongo
