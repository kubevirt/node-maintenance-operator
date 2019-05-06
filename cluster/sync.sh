#!/bin/bash -e

registry_port=$(./cluster/cli.sh ports registry | tr -d '\r')
registry=localhost:$registry_port

make cluster-clean

IMAGE_REGISTRY=$registry make container-build-operator container-push-operator

for i in $(seq 1 ${CLUSTER_NUM_NODES}); do
    ./cluster/cli.sh ssh "node$(printf "%02d" ${i})" 'sudo docker pull registry:5000/node-maintenance-operator'
    # Temporary until image is updated with provisioner that sets this field
    # This field is required by buildah tool
    ./cluster/cli.sh ssh "node$(printf "%02d" ${i})" 'sudo sysctl -w user.max_user_namespaces=1024'
done

# Cleanup previously generated manifests
rm -rf _out/
mkdir -p _out/

# Combine service_account, rbac, operator manifest into namespaced manifest
cp deploy/namespace.yaml _out/namespace-init.yaml
echo -e "\n---\n" >> _out/namespace-init.yaml
cp deploy/service_account.yaml _out/namespace-init.yaml
echo -e "\n---\n" >> _out/namespace-init.yaml
cat deploy/role.yaml >> _out/namespace-init.yaml
echo -e "\n---\n" >> _out/namespace-init.yaml
cat deploy/role_binding.yaml >> _out/namespace-init.yaml
echo -e "\n---\n" >> _out/namespace-init.yaml

cp deploy/operator.yaml _out/operator.yaml
sed -i 's,quay.io/kubevirt/node-maintenance-operator:<IMAGE_VERSION>,registry:5000/node-maintenance-operator,g' _out/operator.yaml
cat _out/operator.yaml >> _out/namespace-init.yaml
rm _out/operator.yaml

cp deploy/crds/nodemaintenance_crd.yaml _out/nodemaintenance_crd.yaml
