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

if [[ $KUBEVIRT_PROVIDER != "external" ]]; then
    export KUBECONFIG=$(${KUBEVIRTCI_PATH}/kubeconfig.sh)
fi

source $KUBEVIRTCI_PATH/hack/common.sh

# Deploy OLM if needed
if [[ $KUBEVIRT_PROVIDER = k8s* ]]; then
    SELF=$(realpath $0)
    BASEPATH=$(dirname $SELF)
    . "${BASEPATH}/get-operator-sdk.sh"

    set +e
    ${OPERATOR_SDK} olm status
    if [[ $? != 0 ]]; then
        ${OPERATOR_SDK} olm install --verbose --timeout 5m
        if [[ $? != 0 ]]; then
            echo "Failed to install OLM!"
            exit 1
        fi
    fi
    set -e

    OLM_NS=olm
    OPERATOR_NS=node-maintenance

    # use "latest" olm-operator, containing this fix: https://github.com/operator-framework/operator-lifecycle-manager/issues/1573
    # TODO remove this as soon as OLM > v0.15.1 is released
    ./kubevirtci/cluster-up/kubectl.sh patch -n olm deployment olm-operator --patch '{"spec": {"template": {"spec": {"containers": [{"name": "olm-operator","image": "quay.io/operator-framework/olm:latest"}]}}}}'
fi

registry="$IMAGE_REGISTRY"

if [[ $KUBEVIRT_PROVIDER != "external" ]]; then

    registry_port=$(docker ps | grep -Po '\d+(?=->5000)')
    registry_ip=$(docker ps | grep dnsmasq | awk '{ print $1 }' | xargs docker inspect | grep registry | sed -r 's/^.*:([0-9.]+)".*$/\1/g')
    registry=localhost:$registry_port

    IMAGE_REGISTRY=$registry OVERRIDE_MANIFEST_REGISTRY="registry:5000" make generate-bundle container-build container-push

    nodes=()
    if [[ $KUBEVIRT_PROVIDER =~ okd.* ]]; then
        for okd_node in "master-0" "worker-0"; do
            node=$(${KUBEVIRTCI_PATH}/kubectl.sh get nodes | grep -o '[^ ]*'${okd_node}'[^ ]*')
            nodes+=(${node})
        done
        pull_command="podman"
    else
        for i in $(seq 1 ${KUBEVIRT_NUM_NODES}); do
            nodes+=("node$(printf "%02d" ${i})")
        done
        pull_command="docker"
    fi

    for node in ${nodes[@]}; do
        ${KUBEVIRTCI_PATH}/ssh.sh ${node} "echo registry:5000/node-maintenance-operator | xargs \-\-max-args=1 sudo ${pull_command} pull"
        # Temporary until image is updated with provisioner that sets this field
        # This field is required by buildah tool
        ${KUBEVIRTCI_PATH}/ssh.sh ${node} "echo user.max_user_namespaces=1024 | xargs \-\-max-args=1 sudo sysctl -w"
    done

else
    make container-build container-push
fi

echo "[INFO] sync-cluster successful."
