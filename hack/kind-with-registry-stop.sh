#!/bin/bash

set -ex

./bin/kind delete cluster

REG=$(docker ps | grep registry:2[[:space:]] | awk '{ print $1 }')
docker stop $REG
docker rm $REG
docker network rm kind

