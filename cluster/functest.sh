#!/bin/bash -e

function new_test() {
    name=$1

    printf "%0.s=" {1..80}
    echo
    echo ${name}
}

new_test 'Test e2e Node Mainenance'
# Run tests
go test ./test/e2e/... -root=$(pwd) -kubeconfig=cluster/.kubeconfig -globalMan deploy/crds/nodemaintenance_crd.yaml --namespacedMan deploy/namespace-init.yaml -namespace=node-maintenance-operator


