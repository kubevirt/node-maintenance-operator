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


echo "***globalMan***"
cat _out/nodemaintenance_crd.yaml

echo "***namespacedMan***"
cat _out/namespace-init.yaml
# Run tests

find . -name .kubeconfig || true
KUBE_CONFIG=$(${KUBEVIRTCI_PATH}/kubeconfig.sh)


TEST_NAMESPACE=node-maintenance-operator go test ./test/e2e/... -root=$(pwd) -kubeconfig=${KUBE_CONFIG} -globalMan _out/nodemaintenance_crd.yaml --namespacedMan _out/namespace-init.yaml -singleNamespace

echo "E2e tests passed"

echo "check validation of openaAPIV3Schema"

$KUBECTL_CMD create -f _out/namespace-init.yaml

$KUBECTL_CMD create -f _out/nodemaintenance_crd.yaml

echo "validate CRD"

VALIDATE_CRD=$($KUBECTL_CMD get -o yaml crd nodemaintenances.kubevirt.io)
if [[ $VALIDATE_CRD == "" ]]; then
	echo "can't validate CRD, check if deployment is running"
	exit 1
fi

set +e
VALIDATION_ERRORS=$(echo "$VALIDATE_CRD" | grep -c "spec.validation.openAPIV3Schema")
set -e

if [[ $VALIDATION_ERRORS != "0" ]]; then
	echo "validation of CRD failed"
	exit 1
fi

echo "check validation of openaAPIV3Schema passed"



