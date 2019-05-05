#!/bin/bash -e

function new_test() {
    name=$1

    printf "%0.s=" {1..80}
    echo
    echo ${name}
}

new_test 'Test e2e Node Mainenance'
#TODO - set IMAGE_VERSION according to tag if needed. 
sed -i "s/<IMAGE_VERSION>/latest/g" deploy/operator.yaml
# Combine service_account, rbac, operator manifest into namespaced manifest
cp deploy/namespace.yaml deploy/namespace-init.yaml
echo -e "\n---\n" >> deploy/namespace-init.yaml
cp deploy/service_account.yaml deploy/namespace-init.yaml
echo -e "\n---\n" >> deploy/namespace-init.yaml
cat deploy/role.yaml >> deploy/namespace-init.yaml
echo -e "\n---\n" >> deploy/namespace-init.yaml
cat deploy/role_binding.yaml >> deploy/namespace-init.yaml
echo -e "\n---\n" >> deploy/namespace-init.yaml
cat deploy/operator.yaml >> deploy/namespace-init.yaml
# Run tests
go test ./test/e2e/... -root=$(pwd) -kubeconfig=cluster/.kubeconfig -globalMan deploy/crds/nodemaintenance_crd.yaml -namespacedMan deploy/namespace-init.yaml


