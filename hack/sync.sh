#!/bin/bash -ex

source ./cluster-up/hack/common.sh

TAG="${1:-latest}"

registry_port=$(docker ps | grep -Po '\d+(?=->5000)')
registry=localhost:$registry_port

if [[ -d "_out" ]]; then
    make cluster-clean
fi


# Cleanup previously generated manifests
#rm -rf _out/
#
# Copy release manifests as a base for generated ones, this should make it possible to upgrade
#cp -r deploy _out/
#
# Sed from docker.io to local registry
#sed -i "s/image: quay\.io\/kubevirt\/hyperconverged-cluster-operator:latest/image: registry:5000\/kubevirt\/hyperconverged-cluster-operator:latest/g" _out/operator.yaml
#
#CMD="./cluster-up/kubectl.sh" ./hack/clean.sh

IMAGE_REGISTRY=$registry make container-build-operator container-push-operator

nodes=()
if [[ $KUBEVIRT_PROVIDER =~ okd.* ]]; then
    for okd_node in "master-0" "worker-0"; do
        node=$(./cluster-up/kubectl.sh get nodes | grep -o '[^ ]*'${okd_node}'[^ ]*')
        nodes+=(${node})
    done
    pull_command="podman"
else
    for i in $(seq 1 ${KUBEVIRT_NUM_NODES}); do
        nodes+=("node$(printf "%02d" ${i})")
    done
    pull_command="docker"
fi

docker ps -a

for node in ${nodes[@]}; do
    ./cluster-up/ssh.sh ${node} "echo registry:5000/node-maintenance-operator | xargs \-\-max-args=1 sudo ${pull_command} pull"
    # Temporary until image is updated with provisioner that sets this field
    # This field is required by buildah tool
    ./cluster-up/ssh.sh ${node} "echo user.max_user_namespaces=1024 | xargs \-\-max-args=1 sudo sysctl -w"
done

# Deploy the HCO
#CMD="./cluster-up/kubectl.sh" HCO_IMAGE="registry:5000/kubevirt/hyperconverged-cluster-operator:latest" ./hack/deploy.sh


# Cleanup previously generated manifests
rm -rf _out/
mkdir -p _out/

# Create node-maintenance-operator namespace
./cluster-up/kubectl.sh create -f deploy/namespace.yaml

# Combine service_account, rbac, operator manifest into namespaced manifest
cp deploy/service_account.yaml _out/namespace-init.yaml
echo -e "\n---\n" >> _out/namespace-init.yaml
cat deploy/role.yaml >> _out/namespace-init.yaml
echo -e "\n---\n" >> _out/namespace-init.yaml
cat deploy/role_binding.yaml >> _out/namespace-init.yaml
echo -e "\n---\n" >> _out/namespace-init.yaml


cp deploy/operator.yaml _out/operator.yaml
sed -i "s,quay.io/kubevirt/node-maintenance-operator:<IMAGE_VERSION>,registry:5000/node-maintenance-operator:${TAG},g" _out/operator.yaml
cat _out/operator.yaml >> _out/namespace-init.yaml
rm _out/operator.yaml

cp deploy/crds/nodemaintenance_crd.yaml _out/nodemaintenance_crd.yaml
