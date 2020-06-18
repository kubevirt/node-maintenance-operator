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

source $KUBEVIRTCI_PATH/hack/common.sh

registry="$IMAGE_REGISTRY"
TAG="${1:-$IMAGE_TAG}"

if [[ -d "_out" ]]; then
    make cluster-clean
fi

if [[ $KUBEVIRT_PROVIDER != "external" ]]; then

    registry_port=$(docker ps | grep -Po '\d+(?=->5000)')
    registry=localhost:$registry_port

    IMAGE_REGISTRY=$registry make container-build-operator container-push-operator

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
    make container-build-operator container-push-operator
fi

# Cleanup previously generated manifests
rm -rf _out/
mkdir -p _out/

# Create node-maintenance-operator namespace
${KUBEVIRTCI_PATH}/kubectl.sh create -f deploy/namespace.yaml

# Combine service_account, rbac, operator manifest into namespaced manifest
cp deploy/service_account.yaml _out/namespace-init.yaml
echo -e "\n---\n" >> _out/namespace-init.yaml
cat deploy/role.yaml >> _out/namespace-init.yaml
echo -e "\n---\n" >> _out/namespace-init.yaml
cat deploy/role_binding.yaml >> _out/namespace-init.yaml
echo -e "\n---\n" >> _out/namespace-init.yaml


cp deploy/operator.yaml _out/operator.yaml
if [[ $KUBEVIRT_PROVIDER != "external" ]]; then
    sed -i "s,quay.io/kubevirt/node-maintenance-operator:<IMAGE_VERSION>,registry:5000/node-maintenance-operator:${TAG},g" _out/operator.yaml
else
    sed -i "s,quay.io/kubevirt/node-maintenance-operator:<IMAGE_VERSION>,${registry}/node-maintenance-operator:${TAG},g" _out/operator.yaml
fi
cat _out/operator.yaml >> _out/namespace-init.yaml
rm _out/operator.yaml

cp deploy/crds/nodemaintenance_crd.yaml _out/nodemaintenance_crd.yaml
