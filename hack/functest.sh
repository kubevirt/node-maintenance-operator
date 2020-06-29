#!/bin/bash -ex

if [ -z "$KUBEVIRTCI_PATH" ]; then
    KUBEVIRTCI_PATH="$(
        cd "$(dirname "$BASH_SOURCE[0]")/"
        readlink -f ../kubevirtci/cluster-up
    )"
fi

if [ -z "$KUBEVIRTCI_CONFIG_PATH" ]; then
    KUBEVIRTCI_CONFIG_PATH="$(
        cd "$(dirname "$BASH_SOURCE[0]")/"
        readlink -f ../_ci-configs
    )"
fi

KUBECTL_CMD="${KUBEVIRTCI_PATH}/kubectl.sh"

function new_test() {
    name=$1

    printf "%0.s=" {1..80}
    echo
    echo ${name}
}

new_test 'Test e2e Node Mainenance'

# Run tests
<<<<<<< HEAD
export KUBECONFIG=${KUBECONFIG:-$(${KUBEVIRTCI_PATH}/kubeconfig.sh)}
TEST_NAMESPACE=node-maintenance-operator GOFLAGS="-mod=vendor" go test ./test/e2e/...
=======

find . -name .kubeconfig || true
KUBE_CONFIG=${KUBECONFIG:-$(${KUBEVIRTCI_PATH}/kubeconfig.sh)}

TEST_NAMESPACE=node-maintenance-operator GOFLAGS="-mod=vendor" go test -v ./test/e2e/... -root=$(pwd) -kubeconfig=${KUBE_CONFIG} -globalMan _out/nodemaintenance_crd.yaml --namespacedMan _out/namespace-init.yaml -singleNamespace
>>>>>>> e2e test: always show logs at the end of a test run

echo "E2e tests passed"
