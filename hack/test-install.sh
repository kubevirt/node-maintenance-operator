#!/bin/bash

set -ex

OPT=${1:-kind}
USE_OLM_INSTALL=1
OLM_RELEASE_VERSION="0.15.1"
LOCAL_OLM=olm-repo

if [[ $OPT != "kind" ]] && [[ $OPT != "minikube" ]]; then
	echo "error: argument must be either kind or minikube"
	exit 1
fi

case "$OPT" in
	minikube)
		LOCAL_REGISTRY=localhost.localdomain:5000
		HOST_IP=$(hostname -I | awk '{ print $1 }')
		;;

	kind)
		LOCAL_REGISTRY=localhost:5000
		;;
esac

NMOBUNDLE=node-maintenance-operator-bundle
NMOREG=node-maintenance-operator-registry
NMO=node-maintenance-operator
CATALOG_NAMESPACE="olm"
REGISTRY=quay.io/kubevirt

log_to_file() {
	exec &> >(tee -a test-install.out)
}

start_registry() {
	hreg_name='local-registry'
	reg_port='5000'
	running="$(docker inspect -f '{{.State.Running}}' "${reg_name}" 2>/dev/null || true)"
	if [ "${running}" != 'true' ]; then
	  docker run \
		-d --restart=always -p "${reg_port}:5000" --name "${reg_name}" \
		registry:2
	fi
}

setup_src() {
	if [[ $USE_OLM_INSTALL == "" ]]; then
		if [[ ! -f $LOCAL_OLM/repo_init ]]; then
			git clone https://github.com/operator-framework/operator-lifecycle-manager.git $LOCAL_OLM
			pushd $LOCAL_OLM
			git checkout origin/release-4.5 -b release-4.5
			echo "" >repo_init
			popd
		fi
	fi
}

setup_utils() {
	mkdir bin || true

	if [[ ! -x bin/kubectl ]]; then
		curl -L https://storage.googleapis.com/kubernetes-release/release/v1.18.2/bin/linux/amd64/kubectl -o bin/kubectl
		chmod +x bin/kubectl
	fi
	case "$OPT" in
		kind)
			if [[ ! -x bin/kind ]]; then
				curl -sLo bin/kind "$(curl -sL https://api.github.com/repos/kubernetes-sigs/kind/releases/latest | jq -r '[.assets[] | select(.name == "kind-linux-amd64")] | first | .browser_download_url')"

				chmod +x bin/kind
			fi
			;;
		minikube)
			if [[ ! -x bin/minikube ]]; then
				curl -Lo ./bin/minikube https://storage.googleapis.com/minikube/releases/latest/minikube-linux-amd64 && chmod +x ./bin/minikube
			fi
			;;
	esac

	if [[ ! -x bin/opm ]]; then
		curl -L https://github.com/operator-framework/operator-registry/releases/download/v1.12.5/linux-amd64-opm -o bin/opm
		chmod +x bin/opm
	fi

	if [[ $USE_OLM_INSTALL != "" ]]; then
            if [[ ! -x bin/install-olm.sh ]]; then
			curl -L https://github.com/operator-framework/operator-lifecycle-manager/releases/download/${OLM_RELEASE_VERSION}/install.sh -o bin/install-olm.sh
			chmod +x bin/install-olm.sh
		fi
	else
		pushd $LOCAL_OLM
		make run-local
		kubectl get nodes
		popd
	fi

	export PATH=$PWD/bin:$PATH
}

start_cluster() {
	if [[ $OPT == "kind" ]]; then
		./hack/kind-with-registry.sh
		kind export kubeconfig
	fi

	if [[ $OPT == "minikube" ]]; then
		minikube start  --insecure-registry="${LOCAL_REGISTRY}"
		minikube ssh 'sudo bash -c "echo '$HOST_IP' localhost.localdomain >>/etc/hosts"'
	fi

	if [[ $USE_OLM_INSTALL != "" ]]; then
        install-olm.sh ${OLM_RELEASE_VERSION}
    fi
}

docker_tag() {
	local from="$1"
	local to="$2"

	#IMG=$(docker images | grep -F $from' ' | awk '{ print $3 }')
	IMG=$(docker images --no-trunc --quiet "$from")
	docker tag $IMG $to
	docker push $to
}

make_deployment_def() {
    local install_img="$1"
    local OFILE="$2"

	set -ex
	cp deploy/namespace.yaml ${OFILE}
	echo -e "\n---\n" >>${OFILE}
	cat deploy/service_account.yaml >>${OFILE}
	echo -e "\n---\n" >>${OFILE}
	cat deploy/role.yaml >>${OFILE}
	echo -e "\n---\n" >>${OFILE}
	cat deploy/role_binding.yaml >>${OFILE}
	echo -e "\n---\n" >>${OFILE}
	cat deploy/operator.yaml | sed  's#quay.io/kubevirt/node-maintenance-operator:<IMAGE_VERSION>#'${install_img}'#' >>${OFILE}
	echo -e "\n---\n" >>${OFILE}
	cat deploy/crds/nodemaintenance_crd.yaml  >>${OFILE}
}

install_from_deployment() {
    local OFILE=tmp.yml

    docker_tag $REGISTRY/$NMO $LOCAL_REGISTRY/$NMO

    make_deployment_def	 $LOCAL_REGISTRY/$NMO  "${OFILE}"

    kubectl create -f ${OFILE} --allow-missing-template-keys=true

    kubectl wait --for=condition=available --timeout=600s deployment/node-maintenance-operator -n node-maintenance-operator

    kubectl describe pod -n node-maintenance-operator  | grep Image

}

uninstall_from_deployment() {
    local OFILE=tmp.yml

    make_deployment_def	 $LOCAL_REGISTRY/$NMO  "${OFILE}"

    kubectl delete -f ${OFILE}
}

install_with_olm() {

local nspace="$1"

if [[ $nspace != "node-maintenance-operator" ]]; then
    cat <<EOF | kubectl create -f -
apiVersion: v1
kind: Namespace
metadata:
  annotations:
  labels:
    kubevirt.io: ""
  name:  ${nspace}
EOF
fi

cat <<EOF | kubectl create -f -
apiVersion: operators.coreos.com/v1alpha2
kind: OperatorGroup
metadata:
  name: node-maintenance-operator
  namespace: ${nspace}
EOF

cat <<EOF | kubectl create -f -
apiVersion: operators.coreos.com/v1alpha1
kind: CatalogSource
metadata:
  name: node-maintenance-operator
  namespace: olm
spec:
  sourceType: grpc
  image: quay.io/kubevirt/node-maintenance-operator-registry:v0.6.0
  displayName: node-maintenance-operator
  publisher: Red hat
EOF

cat <<EOF | kubectl create -f -
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: node-maintenance-operator-subscription
  namespace: ${nspace}
spec:
  channel: beta
  name: node-maintenance-operator
  source: node-maintenance-operator
  sourceNamespace: olm
  startingCSV: node-maintenance-operator.v0.6.0
EOF

 retry_count=0
 while [[ $retry_count -lt 30 ]]; do
   nmo_pod=$(kubectl get pods -n ${nspace} | grep node-maintenance-operator | head -1 | awk '{ print $1 }')
   if [[ "$nmo_pod" != "" ]]; then
	  break
   fi
   sleep 10s
   ((retry_count+=1))
 done

    kubectl wait --for=condition=available --timeout=600s deployment/node-maintenance-operator -n ${nspace}

    kubectl describe pod -n ${nspace} | grep Image

}
log_to_file

create_cr_object() {

    local namespace_one="$1"
    local namespace_two="$2"

    cat  <<EOF | kubectl create -f -
apiVersion: nodemaintenance.kubevirt.io/v1beta1
kind: NodeMaintenance
metadata:
  name: nodemaintenance-xyz
spec:
  nodeName: kind-worker
  reason: "Test node maintenance"
EOF
 sleep 2

 kubectl logs -n ${namespace_one} $(kubectl get pods -n ${namespace_one} | sed '1d' | head -1 | awk '{ print $1 }')

 kubectl logs -n ${namespace_two} $(kubectl get pods -n ${namespace_two} | sed '1d' | tail -1 | awk '{ print $1 }')

}

if [[ $OPT == "minikube" ]]; then
	start_registry
fi
setup_src
setup_utils
start_cluster
install_from_deployment
install_with_olm "node-maintenance-operator2"
create_cr_object "node-maintenance-operator" "node-maintenance-operator2"

echo "***  All systems running. liftoff. ***"
