#!/bin/bash

docker tag kubevirtci/os-3.11.0-crio:latest docker.io/kubevirtci/os-3.11.0-crio:latest
docker push docker.io/kubevirtci/os-3.11.0-crio:latest
