#!/bin/bash -ex

if [ -n "${IMAGE_FORMAT}" ]; then
    # Override the index image name in the CatalogSource when this is invoked from OpenshiftCI
    # See https://github.com/openshift/ci-tools/blob/master/TEMPLATES.md#image_format
    # shellcheck disable=SC2016
    export FULL_INDEX_IMAGE="${IMAGE_FORMAT//'${component}'/node-maintenance-operator-index}"
    OLM_CHANNEL="9.9"
    export CLUSTER_COMMAND="oc"

    # disable default catalog resources
    ${CLUSTER_COMMAND} patch OperatorHub cluster --type json -p '[{"op": "add", "path": "/spec/disableAllDefaultSources", "value": true}]'
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

    # Use internal registry when using KubevirtCI
    if [[ $KUBEVIRT_PROVIDER != "external" ]]; then
        export KUBECONFIG=$(${KUBEVIRTCI_PATH}/kubeconfig.sh)
        export FULL_INDEX_IMAGE="registry:5000/${INDEX_IMAGE}:${IMAGE_TAG}"
    else
        export FULL_INDEX_IMAGE="${IMAGE_REGISTRY}/${INDEX_IMAGE}:${IMAGE_TAG}"
    fi

    if [[ $KUBEVIRT_PROVIDER = k8s* ]]; then
        OLM_NS=olm
        OPERATOR_NS=node-maintenance
    fi

    source "${KUBEVIRTCI_PATH}/hack/common.sh"

fi

echo "Deploying with index image ${FULL_INDEX_IMAGE}"

# Cleanup previously generated manifests
deploy_dir=_out

if [[ -d "${deploy_dir}" ]]; then
    make cluster-clean
fi

rm -rf v ${deploy_dir}/
mkdir -p ${deploy_dir}/

# copy manifests
cp deploy/namespace.yaml ${deploy_dir}/
cp deploy/catalogsource.yaml ${deploy_dir}/
cp deploy/operatorgroup.yaml ${deploy_dir}/
cp deploy/subscription.yaml ${deploy_dir}/

sed -i "s,REPLACE_INDEX_IMAGE,${FULL_INDEX_IMAGE},g" ${deploy_dir}/catalogsource.yaml
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
        ${CLUSTER_COMMAND} apply -f $deploy_dir
    else
        ${CLUSTER_COMMAND} apply -f $deploy_dir &>/dev/null
    fi
    CHECK_1=$?

    if [[ $iterations -eq $((max_iterations - 1)) ]] || [[ -n "${VERBOSE}" ]]; then
        ${CLUSTER_COMMAND} -n "${OPERATOR_NS}" wait deployment/node-maintenance-operator --for condition=Available --timeout 1s
    else
        ${CLUSTER_COMMAND} -n "${OPERATOR_NS}" wait deployment/node-maintenance-operator --for condition=Available --timeout 1s &>/dev/null
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
        # Fixme Webhook setup is slow... service needs to be created, endpoint needs to be created, and webhook server must be running
        # Didn't find an easy but good solution yet, so just wait a bit for now
        echo "[INFO] Giving the webhook some time to setup"
        sleep 30s
    fi
    set -e

done

if [[ $success -eq 0 ]]; then
    echo "[ERROR] Deployment failed, giving up."

    set +e
    echo "CatalogSource:\n\n"
    ${CLUSTER_COMMAND} -n ${OLM_NS} describe catalogsource node-maintenance-operator
    echo "Subscription:\n\n"
    ${CLUSTER_COMMAND} -n ${OPERATOR_NS} describe subscription node-maintenance-operator
    echo "OperatorGroup:\n\n"
    ${CLUSTER_COMMAND} -n ${OPERATOR_NS} describe operatorgroup node-maintenance-operator
    echo "All OperatorGroups:\n\n"
    ${CLUSTER_COMMAND} get operatorgroup -A -o=wide
    echo "Deployment:\n\n"
    ${CLUSTER_COMMAND} -n ${OPERATOR_NS} describe deployment node-maintenance-operator
    echo "Namespace:\n\n"
    ${CLUSTER_COMMAND} describe namespace ${OPERATOR_NS}
    exit 1
fi

echo "[INFO] Deployment successful."
