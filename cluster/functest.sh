#!/bin/bash -ex

function new_test() {
    name=$1

    printf "%0.s=" {1..80}
    echo
    echo ${name}
}

new_test 'Test e2e Node Mainenance'


echo "***globalMan***"
cat _out/nodemaintenance_crd.yaml

echo "***namespacedMan***"
cat _out/namespace-init.yaml
# Run tests

find . -name .kubeconfig || true
KUBE_CONFIG=$(./cluster-up/kubeconfig.sh)


TEST_NAMESPACE=node-maintenance-operator go test ./test/e2e/... -root=$(pwd) -kubeconfig=${KUBE_CONFIG} -globalMan _out/nodemaintenance_crd.yaml --namespacedMan _out/namespace-init.yaml -singleNamespace

echo "E2e tests passed"

