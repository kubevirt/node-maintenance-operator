#!/bin/bash -e

function new_test() {
    name=$1

    printf "%0.s=" {1..80}
    echo
    echo ${name}
}

new_test 'Test e2e Node Mainenance'

if [ -z "$KUBEVIRTCI_PATH" ]; then
    KUBEVIRTCI_PATH="$(
        cd "$(dirname "$BASH_SOURCE[0]")/"
        echo "$(pwd)/"
    )"
fi

source ${KUBEVIRTCI_PATH}/hack/common.sh
source ${KUBEVIRTCI_CLUSTER_PATH}/$KUBEVIRT_PROVIDER/provider.sh
source $KUBEVIRTCI_CONFIG_PATH/$KUBEVIRT_PROVIDER/config-provider-$KUBEVIRT_PROVIDER.sh

# Run tests
TEST_NAMESPACE=node-maintenance-operator go test ./test/e2e/... -root=$(pwd) -kubeconfig=${kubeconfig} -globalMan _out/nodemaintenance_crd.yaml --namespacedMan _out/namespace-init.yaml -singleNamespace -v

echo "E2e tests passed"

