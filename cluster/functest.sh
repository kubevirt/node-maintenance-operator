#!/bin/bash -e

function new_test() {
    name=$1

    printf "%0.s=" {1..80}
    echo
    echo ${name}
}

new_test 'Test e2e Node Mainenance'

# Run tests
TEST_NAMESPACE=node-maintenance-operator go test ./test/e2e/... -root=$(pwd) -kubeconfig=cluster/.kubeconfig -globalMan _out/nodemaintenance_crd.yaml --namespacedMan _out/namespace-init.yaml -singleNamespace

echo "E2e tests passed"

