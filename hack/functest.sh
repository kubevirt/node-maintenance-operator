#!/bin/bash -ex

if [ -n "${IMAGE_FORMAT}" ]; then
    echo "Running functest on OpenshiftCI"
    export CLUSTER_COMMAND="oc"
else
    # We are not on OpenshiftCI
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

    export CLUSTER_COMMAND="${KUBEVIRTCI_PATH}/kubectl.sh"

    if [[ $KUBEVIRT_PROVIDER != "external" ]]; then
        export KUBECONFIG=$(${KUBEVIRTCI_PATH}/kubeconfig.sh)
    fi

    if [[ $KUBEVIRT_PROVIDER = k8s* ]]; then
        # on k8s we don' t have a etcd-quorum-guard running
        # we need it for testing the master quorum validation
        # so we create a fake etcd-quorum-guard PDB with maxUnavailable = 0, which will always result in disruptionsAllowed = 0 without a corresponding deployment
        # that will make node maintenance requests for master nodes always fail
        $CLUSTER_COMMAND apply -f test/manifests/fake-etcd-quorum-guard.yaml
    fi

    OPERATOR_NS=node-maintenance

fi

# Run tests
# let's track errors on our own here for being able to write a nice comment afterwards
set +e
# FIXME cleanup test namespace
export TEST_NAMESPACE=node-maintenance-test
$CLUSTER_COMMAND create ns "$TEST_NAMESPACE"
GOFLAGS="-mod=vendor" go test -count=1 -v ./test/e2e/...

if [[ $? != 0 ]]; then
    echo "E2e tests FAILED"
    exit 1
fi

echo "E2e tests passed"
