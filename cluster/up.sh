#!/bin/bash -xe

KUBERNETES_1_11_IMAGE="k8s-1.11.0@sha256:3412f158ecad53543c9b0aa8468db84dd043f01832a66f0db90327b7dc36a8e8"
KUBERNETES_1_13_3_IMAGE="k8s-1.13.3@sha256:bc0f02d6b970650eb16d12f97e5aa1376b3a13b0ffed6227db98675be2ca1184"
OPENSHIFT_IMAGE_3_11="os-3.11.0-crio@sha256:3f11a6f437fcdf2d70de4fcc31e0383656f994d0d05f9a83face114ea7254bc0"
OPENSHIFT_IMAGE_4_3="ocp-4.3@sha256:8700c73ceec8d8e63913ea9e7073d33974a31e418117514f1e2d06bd3e9f37ff"
OPENSHIFT_IMAGE_4_1="okd-4.1@sha256:e7e3a03bb144eb8c0be4dcd700592934856fb623d51a2b53871d69267ca51c86"
OPENSHIFT_IMAGE_4_2="okd-4.2@sha256:a830064ca7bf5c5c2f15df180f816534e669a9a038fef4919116d61eb33e84c5"

CLUSTER_PROVIDER=${CLUSTER_PROVIDER:-k8s-1.11.0}
CLUSTER_MEMORY_SIZE=${CLUSTER_MEMORY_SIZE:-5120M}
CLUSTER_NUM_NODES=${CLUSTER_NUM_NODES:-1}

if ! [[ $CLUSTER_NUM_NODES =~ ^-?[0-9]+$ ]] || [[ $CLUSTER_NUM_NODES -lt 1 ]] ; then
    CLUSTER_NUM_NODES=1
fi

case "${CLUSTER_PROVIDER}" in
    'k8s-1.11.0')
        image=$KUBERNETES_1_11_IMAGE
        ;;
    'k8s-1.13.3')
        image=$KUBERNETES_1_13_3_IMAGE
        ;;
    'os-3.11.0')
        image=$OPENSHIFT_IMAGE_3_11
        ;;
    'ocp-4.3')
        image=$OPENSHIFT_IMAGE_4_3
        ;;
esac


function onfail() {
    st=$?
    echo "command failed with status: $st"
    echo "environment:"
    printenv
    exit $st
}

echo "Install cluster from image: ${image}"
if [[ $image == $KUBERNETES_1_11_IMAGE ]] || [[ $image == $KUBERNETES_1_13_3_IMAGE ]]; then
    # Run Kubernetes cluster image
    ./cluster/cli.sh run --random-ports --nodes ${CLUSTER_NUM_NODES} --memory ${CLUSTER_MEMORY_SIZE} --background kubevirtci/${image}

    # Copy kubectl tool and configuration file
    ./cluster/cli.sh scp /usr/bin/kubectl - > ./cluster/.kubectl
    chmod u+x ./cluster/.kubectl
    ./cluster/cli.sh scp /etc/kubernetes/admin.conf - > ./cluster/.kubeconfig

    # Configure insecure access to Kubernetes cluster
    cluster_port=$(./cluster/cli.sh ports k8s | tr -d '\r')
    ./cluster/kubectl.sh config set-cluster kubernetes --server=https://127.0.0.1:$cluster_port
    ./cluster/kubectl.sh config set-cluster kubernetes --insecure-skip-tls-verify=true
elif [[ $image == $OPENSHIFT_IMAGE_3_11 ]] ; then
    # If on a developer setup, expose ocp on 8443, so that the openshift web console can be used (the port is important because of auth redirects)
    if [ -z "${JOB_NAME}" ]; then
        CLUSTER_PROVIDER_EXTRA_ARGS="${CLUSTER_PROVIDER_EXTRA_ARGS} --ocp-port 8443"
    fi

    # Run OpenShift cluster image
    ./cluster/cli.sh run --random-ports --reverse --nodes ${CLUSTER_NUM_NODES} --memory ${CLUSTER_MEMORY_SIZE} --background kubevirtci/${image} ${CLUSTER_PROVIDER_EXTRA_ARGS}
    ./cluster/cli.sh scp /etc/origin/master/admin.kubeconfig - > ./cluster/.kubeconfig
    ./cluster/cli.sh ssh node01 -- sudo cp /etc/origin/master/admin.kubeconfig ~vagrant/
    ./cluster/cli.sh ssh node01 -- sudo chown vagrant:vagrant ~vagrant/admin.kubeconfig

    # Copy oc tool and configuration file
    ./cluster/cli.sh scp /usr/bin/oc - > ./cluster/.kubectl
    chmod u+x ./cluster/.kubectl
    ./cluster/cli.sh scp /etc/origin/master/admin.kubeconfig - > ./cluster/.kubeconfig

    # Update Kube config to support unsecured connection
    cluster_port=$(./cluster/cli.sh ports ocp | tr -d '\r')
    ./cluster/kubectl.sh config set-cluster node01:8443 --server=https://127.0.0.1:$cluster_port
    ./cluster/kubectl.sh config set-cluster node01:8443 --insecure-skip-tls-verify=true

elif [[ $image == $OPENSHIFT_IMAGE_4_3 ]] ; then


    # If on a developer setup, expose ocp on 8443, so that the openshift web console can be used (the port is important because of auth redirects)
    #if [ -z "${JOB_NAME}" ]; then
    #    CLUSTER_PROVIDER_EXTRA_ARGS="${CLUSTER_PROVIDER_EXTRA_ARGS} --ocp-port 8443"
    #fi

    #export CLUSTER_MEMORY_SIZE="7680M"
    #./cluster/cli.sh run --random-ports --reverse --nodes ${CLUSTER_NUM_NODES} --memory ${CLUSTER_MEMORY_SIZE} --background kubevirtci/${image} ${CLUSTER_PROVIDER_EXTRA_ARGS} || onfail


    # Run OpenShift cluster image
    ./cluster/cli.sh run okd \
            --random-ports \
            --background \
            --prefix=okd-4.3 \
            --registry-volume okd-4.3-registry \
             kubevirtci/${image} \
             ${CLUSTER_PROVIDER_EXTRA_ARGS} || onfail

    ./cluster/cli.sh scp /etc/origin/master/admin.kubeconfig - > ./cluster/.kubeconfig
    ./cluster/cli.sh ssh node01 -- sudo cp /etc/origin/master/admin.kubeconfig ~vagrant/
    ./cluster/cli.sh ssh node01 -- sudo chown vagrant:vagrant ~vagrant/admin.kubeconfig

    # Copy oc tool and configuration file
    ./cluster/cli.sh scp /usr/bin/oc - > ./cluster/.kubectl
    chmod u+x ./cluster/.kubectl
    ./cluster/cli.sh scp /etc/origin/master/admin.kubeconfig - > ./cluster/.kubeconfig

    # Update Kube config to support unsecured connection
    cluster_port=$(./cluster/cli.sh ports ocp | tr -d '\r')
    ./cluster/kubectl.sh config set-cluster node01:8443 --server=https://127.0.0.1:$cluster_port
    ./cluster/kubectl.sh config set-cluster node01:8443 --insecure-skip-tls-verify=true
fi

echo 'Wait until all nodes are ready'
until [[ $(./cluster/kubectl.sh get nodes --no-headers | wc -l) -eq $(./cluster/kubectl.sh get nodes --no-headers | grep ' Ready' | wc -l) ]]; do
    sleep 1
 done
