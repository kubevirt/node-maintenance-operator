#!/bin/bash -ex

if [ -z "$KUBEVIRTCI_PATH" ]; then
    # sets the path of the CI scripts
    KUBEVIRTCI_PATH="$(
        cd "$(dirname "$BASH_SOURCE[0]")/"
        readlink -f ../kubevirtci/cluster-up
    )"
fi


if [ -z "$KUBEVIRTCI_CONFIG_PATH" ]; then
    # set location of ci configuration.
    # need to override this variable, as the file common.sh is now one level deeper in the directory tree, so it wouldn't find the _ci_config directory
    # set in the root directory of the git repo for this project.
     KUBEVIRTCI_CONFIG_PATH="$(
        cd "$(dirname "$BASH_SOURCE[0]")/"
        readlink -f ../_ci-configs
    )"
fi



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
KUBE_CONFIG=$(${KUBEVIRTCI_PATH}/kubeconfig.sh)


TEST_NAMESPACE=node-maintenance-operator go test ./test/e2e/... -root=$(pwd) -kubeconfig=${KUBE_CONFIG} -globalMan _out/nodemaintenance_crd.yaml --namespacedMan _out/namespace-init.yaml -singleNamespace

echo "E2e tests passed"

