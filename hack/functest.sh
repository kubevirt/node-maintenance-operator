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
export KUBECONFIG=${KUBECONFIG:-$(${KUBEVIRTCI_PATH}/kubeconfig.sh)}
TEST_NAMESPACE=node-maintenance-operator GOFLAGS="-mod=vendor" go test ./test/e2e/...

echo "E2e tests passed"
