#!/usr/bin/env bash
#
# This file is part of the KubeVirt project
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#
# Copyright 2017 Red Hat, Inc.
#

# This logic moved into the Makefile.
# We're leaving this file around for people who still reference this
# specific script in their development workflow.

set -e

TAG="${1:-latest}"

registry_port=$(./cluster-up/cli.sh ports registry | tr -d '\r')
registry=localhost:$registry_port


source hack/common.sh
source cluster-up/cluster/$KUBEVIRT_PROVIDER/provider.sh
source hack/config.sh

kubectl() { cluster-up/kubectl.sh "$@"; }

if [ -d "_out"]; then
    make cluster-clean
fi

echo "Building ..."

# Build everyting and publish it
IMAGE_REGISTRY=$registry IMAGE_TAG=${TAG} make container-build-operator container-push-operator

#${KUBEVIRT_PATH}hack/dockerized "DOCKER_PREFIX=${DOCKER_PREFIX} DOCKER_TAG=${IMAGE_TAG} KUBEVIRT_PROVIDER=${KUBEVIRT_PROVIDER} ./hack/bazel-build.sh"
#${KUBEVIRT_PATH}hack/dockerized "DOCKER_PREFIX=${DOCKER_PREFIX} DOCKER_TAG=${IMAGE_TAG} DOCKER_TAG_ALT=${IMAGE_TAG_ALT} KUBEVIRT_PROVIDER=${KUBEVIRT_PROVIDER} IMAGE_PREFIX=${IMAGE_PREFIX} IMAGE_PREFIX_ALT=${IMAGE_PREFIX_ALT} ./hack/bazel-push-images.sh"
#${KUBEVIRT_PATH}hack/dockerized "DOCKER_PREFIX=${DOCKER_PREFIX} DOCKER_TAG=${IMAGE_TAG} KUBEVIRT_PROVIDER=${KUBEVIRT_PROVIDER} IMAGE_PULL_POLICY=${IMAGE_PULL_POLICY} VERBOSITY=${VERBOSITY} IMAGE_PREFIX=${IMAGE_PREFIX}  IMAGE_PREFIX_ALT=${IMAGE_PREFIX_ALT} ./hack/build-manifests.sh"

# Make sure that all nodes use the newest images

#for i in $(seq 1 ${CLUSTER_NUM_NODES}); do
#    ./cluster/cli.sh ssh "node$(printf "%02d" ${i})" "sudo docker pull registry:5000/node-maintenance-operator:${TAG}"
    # Temporary until image is updated with provisioner that sets this field
    # This field is required by buildah tool
#    ./cluster/cli.sh ssh "node$(printf "%02d" ${i})" 'sudo sysctl -w user.max_user_namespaces=1024'
#done


container=""
container_alias=""
for arg in ${docker_images}; do
    name=${image_prefix}$(basename $arg)
    container="${container} ${manifest_docker_prefix}/${name}:${docker_tag}"
    container_alias="${container_alias} ${manifest_docker_prefix}/${name}:${docker_tag} node-maintenance-operator/${name}:${docker_tag}"
done

# OKD/OCP providers has different node names and does not have docker
if [[ $KUBEVIRT_PROVIDER =~ ocp.* ]]; then
    nodes=()
    nodes+=($(kubectl get nodes --no-headers | awk '{print $1}' | grep master))
    nodes+=($(kubectl get nodes --no-headers | awk '{print $1}' | grep worker))
    pull_command="podman"
elif [[ $KUBEVIRT_PROVIDER =~ okd.* ]]; then
    nodes=("master-0" "worker-0")
    pull_command="podman"
elif [[ $KUBEVIRT_PROVIDER == "external" ]] || [[ $KUBEVIRT_PROVIDER =~ kind.* ]]; then
    nodes=() # in case of external provider / kind we have no control over the nodes
else
    nodes=()
    for i in $(seq 1 ${KUBEVIRT_NUM_NODES}); do
        nodes+=("node$(printf "%02d" ${i})")
    done
    pull_command="docker"
fi

for node in ${nodes[@]}; do
    until ${KUBEVIRT_PATH}cluster-up/ssh.sh ${node} "echo \"${container}\" | xargs \-\-max-args=1 sudo ${pull_command} pull"; do
        sleep 1
    done

    until ${KUBEVIRT_PATH}cluster-up/ssh.sh ${node} "echo \"${container_alias}\" | xargs \-\-max-args=2 sudo ${pull_command} tag"; do
        sleep 1
    done
done


# Cleanup previously generated manifests
rm -rf _out/
mkdir -p _out/

# Create node-maintenance-operator namespace
cluster-up/kubectl.sh create -f deploy/namespace.yaml

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


echo "Done"
