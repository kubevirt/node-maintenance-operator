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


echo "testing presence of admissionregistration api"
./kubevirtci/cluster-up/kubectl.sh api-versions | grep admissionregistration

function cleanup {

	# delete webhook service
	./kubevirtci/cluster-up/kubectl.sh delete service webhooknodemaintenance-kubevirt-iov1beta1 -n node-maintenance-operator

	# delete webhook configuration
	./kubevirtci/cluster-up/kubectl.sh delete validatingwebhookconfiguration  nodemaintenance.kubevirt.io.v1beta1
}

trap cleanup EXIT ERR SIGINT SIGTERM SIGQUIT

TEST_NAMESPACE=node-maintenance-operator go test ./test/e2e/... -root=$(pwd) -kubeconfig=${KUBE_CONFIG} -globalMan _out/nodemaintenance_crd.yaml --namespacedMan _out/namespace-init.yaml -singleNamespace

echo "E2e tests passed"

