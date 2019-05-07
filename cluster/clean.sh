#!/bin/bash -e

echo 'Cleaning up ...'

./cluster/kubectl.sh delete --ignore-not-found -f _out/namespace-init.yaml
./cluster/kubectl.sh delete --ignore-not-found -f _out/nodemaintenance_crd.yaml
./cluster/kubectl.sh delete --ignore-not-found  -f deploy/namespace.yaml

sleep 2

echo 'Done'
