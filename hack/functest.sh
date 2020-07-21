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
if [[ $KUBEVIRT_PROVIDER != "external" ]]; then
    export KUBECONFIG=$(${KUBEVIRTCI_PATH}/kubeconfig.sh)
fi

# let's track errors on our own here for being able to write a nice comment afterwards
set +e
# FIXME use a different namespace for test deployments, and create / destroy it before / after test execution
TEST_NAMESPACE=node-maintenance GOFLAGS="-mod=vendor" go test -v ./test/e2e/...

if [[ $? != 0 ]]; then
    echo "E2e tests FAILED"
    exit 1
fi

echo "E2e tests passed"
