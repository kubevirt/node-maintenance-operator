#!/bin/bash -ex

if [ -n "${OPENSHIFT_CI}" ]; then
    echo "Running functest on OpenshiftCI"
    export CLUSTER_COMMAND="oc"
    OPERATOR_NS="default"
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
        OPERATOR_NS=node-maintenance
    fi

fi

# no colors in CI
NO_COLOR=""
set +e
if ! which tput &>/dev/null 2>&1 || [[ $(tput -T$TERM colors) -lt 8 ]]; then
    echo "Terminal does not seem to support colored output, disabling it"
    NO_COLOR="-noColor"
fi

# never colors in OpenshiftCI?
if [ -n "${OPENSHIFT_CI}" ]; then
    NO_COLOR="-noColor"
fi

export TEST_NAMESPACE=node-maintenance-test

# -v: print out the text and location for each spec before running it and flush output to stdout in realtime
# -r: run suites recursively
# --keepGoing: don't stop on failing suite
# -requireSuite: fail if tests are not executed because of missing suite
go run github.com/onsi/ginkgo/ginkgo $NO_COLOR -v -r --keepGoing -requireSuite ./test/e2e

if [[ $? != 0 ]]; then
    echo "E2e tests FAILED"
    exit 1
fi

echo "E2e tests passed"
