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
TAG="${1:-$IMAGE_TAG}"

if [[ -d "_out" ]]; then
    make cluster-clean
fi

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

# Cleanup previously generated manifests
deploy_dir=_out
rm -rf v ${deploy_dir}/
mkdir -p ${deploy_dir}/

# copy manifests
cp deploy/namespace.yaml ${deploy_dir}/
cp deploy/catalogsource.yaml ${deploy_dir}/
cp deploy/operatorgroup.yaml ${deploy_dir}/
cp deploy/subscription.yaml ${deploy_dir}/

if [[ $KUBEVIRT_PROVIDER != "external" ]]; then
    sed -i "s,REPLACE_INDEX_IMAGE,registry:5000/node-maintenance-operator-index:${TAG},g" ${deploy_dir}/catalogsource.yaml
else
    sed -i "s,REPLACE_INDEX_IMAGE,${registry}/node-maintenance-operator-index:${TAG},g" ${deploy_dir}/catalogsource.yaml
fi

sed -i "s,MARKETPLACE_NAMESPACE,${OLM_NS},g" ${deploy_dir}/*.yaml
sed -i "s,SUBSCRIPTION_NAMESPACE,${OPERATOR_NS},g" ${deploy_dir}/*.yaml
sed -i "s,CHANNEL,\"${OLM_CHANNEL}\",g" ${deploy_dir}/*.yaml

# Deploy
success=0
iterations=0
sleep_time=10
max_iterations=30 # results in 5 minute timeout

until [[ $success -eq 1 ]] || [[ $iterations -eq $max_iterations ]]; do

    echo "[INFO] Deploying NMO via OLM"
    set +e

    # be verbose on last iteration only
    if [[ $iterations -eq $((max_iterations - 1)) ]] || [[ -n "${VERBOSE}" ]]; then
        ./kubevirtci/cluster-up/kubectl.sh apply -f $deploy_dir
    else
        ./kubevirtci/cluster-up/kubectl.sh apply -f $deploy_dir &>/dev/null
    fi
    CHECK_1=$?

    if [[ $iterations -eq $((max_iterations - 1)) ]] || [[ -n "${VERBOSE}" ]]; then
        ./kubevirtci/cluster-up/kubectl.sh -n "${OPERATOR_NS}" wait deployment/node-maintenance-operator --for condition=Available --timeout 1s
    else
        ./kubevirtci/cluster-up/kubectl.sh -n "${OPERATOR_NS}" wait deployment/node-maintenance-operator --for condition=Available --timeout 1s &>/dev/null
    fi
    CHECK_2=$?

    if [[ ${CHECK_1} != 0 ]] || [[ ${CHECK_2} != 0 ]]; then

        iterations=$((iterations + 1))
        iterations_left=$((max_iterations - iterations))
        if [[ $iterations_left != 0 ]]; then
            echo "[WARN] Deployment did not fully succeed yet, retrying in $sleep_time sec, $iterations_left retries left"
            sleep $sleep_time
        else
            echo "[WARN] At least one deployment failed, giving up"
        fi

    else
        # All resources deployed successfully
        success=1
    fi
    set -e

done

if [[ $success -eq 0 ]]; then
    echo "[ERROR] Deployment failed, giving up."
    exit 1
fi

echo "[INFO] Deployment successful."
