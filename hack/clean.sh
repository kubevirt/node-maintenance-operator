#!/bin/bash

echo 'Cleaning up ...'


./kubevirtci/cluster-up/kubectl.sh delete --ignore-not-found -f _out/namespace-init.yaml
./kubevirtci/cluster-up/kubectl.sh delete --ignore-not-found -f _out/nodemaintenance_crd.yaml
./kubevirtci/cluster-up/kubectl.sh delete --ignore-not-found -f deploy/namespace.yaml

