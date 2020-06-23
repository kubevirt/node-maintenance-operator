#!/bin/bash

set -ex

OPT=${1:-kind}

if [[ $OPT != "kind" ]] && [[ $OPT != "minikube" ]]; then
	echo "error: argument must be either kind or minikube"
	exit 1
fi

LOCAL_OLM=olm-repo

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
	exec &> >(tee -a test-olm.out)
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
	if [[ ! -f $LOCAL_OLM/repo_init ]]; then
		git clone https://github.com/operator-framework/operator-lifecycle-manager.git $LOCAL_OLM
		pushd $LOCAL_OLM
		#git checkout origin/release-4.5 -b release-4.5
		echo "" >repo_init
		popd

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

	pushd $LOCAL_OLM
	make run-local
	kubectl get nodes
	popd
}

docker_tag() {
	local from="$1"
	local to="$2"

	#IMG=$(docker images | grep -F $from' ' | awk '{ print $3 }')
	IMG=$(docker images --no-trunc --quiet "$from")
	docker tag $IMG $to
	docker push $to
}

make_olm_op_bundle_new() {

	# make the operator
	make container-build

	docker_tag $REGISTRY/$NMO $LOCAL_REGISTRY/$NMO

	# make local copy of the bundle

	rm -rf tmp-manifest || true
	mkdir -p tmp-manifest/node-maintenance-operator/manifests
	mkdir -p tmp-manifest/node-maintenance-operator/metadata

	cp -rf manifests/node-maintenance-operator/v0.6.0/*  tmp-manifest/node-maintenance-operator/manifests

	#cp manifests/node-maintenance-operator/node-maintenance-operator.package.yaml tmp-manifest/node-maintenance-operator/

	./hack/test-olm-make-annotations.sh >tmp-manifest/node-maintenance-operator/metadata/annotations.yaml  	

	case  "$OPT" in
		minikube)
		sed -i -e 's#quay.io/kubevirt/node-maintenance-operator:v0.6.0#localhost.localdomain:5000/node-maintenance-operator:latest#' tmp-manifest/node-maintenance-operator/manifests/node-maintenance-operator.v0.6.0.clusterserviceversion.yaml

		sed -i -e 's#quay.io/kubevirt/node-maintenance-operator#localhost.localdomain:5000/node-maintenance-operator#' tmp-manifest/node-maintenance-operator/manifests/node-maintenance-operator.v0.6.0.clusterserviceversion.yaml
		;;

		kind)
		sed -i -e 's#quay.io/kubevirt/node-maintenance-operator:v0.6.0#localhost:5000/node-maintenance-operator:latest#' tmp-manifest/node-maintenance-operator/manifests/node-maintenance-operator.v0.6.0.clusterserviceversion.yaml

		sed -i -e 's#quay.io/kubevirt/node-maintenance-operator#localhost:5000/node-maintenance-operator#' tmp-manifest/node-maintenance-operator/manifests/node-maintenance-operator.v0.6.0.clusterserviceversion.yaml
		;;
	esac

	./hack/test-olm-make-docker.sh

	docker build -f docker.tmp -t $NMOBUNDLE:latest .

	docker_tag $NMOBUNDLE:latest $LOCAL_REGISTRY/$NMOBUNDLE:latest
}

run_bundle() {

NMO_NAMESPACE="node-maintenance-operator"
REG_IMG=$LOCAL_REGISTRY/$NMOREG:latest

opm index add --debug --bundles $LOCAL_REGISTRY/$NMOBUNDLE:latest --tag $REG_IMG -c docker

docker_tag $REG_IMG $REG_IMG

# is it needed anywhere?
kubectl create ns ${NMO_NAMESPACE}

if [[ $OP == "kind" ]]; then
	kind load docker-image ${LOCAL_REGISTRY}/node-maintenance-operator:latest
	kind load docker-image ${LOCAL_REGISTRY}/node-maintenance-operator-registry:latest
	kind load docker-image ${LOCAL_REGISTRY}/node-maintenance-operator-bundle:latest
fi

CATALOG_NAMESPACE="${CATALOG_NAMESPACE}" REG_IMG="${REG_IMG}" ./hack/test-olm-make-catalog-source.sh

CATALOG_NAMESPACE="${CATALOG_NAMESPACE}" REG_IMG="${REG_IMG}" ./hack/test-olm-make-catalog-source.sh | kubectl create -f -


CATALOG_NAMESPACE="${CATALOG_NAMESPACE}" ./hack/test-olm-make-subscription.sh

CATALOG_NAMESPACE="${CATALOG_NAMESPACE}" ./hack/test-olm-make-subscription.sh | kubectl create -f -

}

test_nmo_webhook() {

 retry_count=0
 while [[ $retry_count -lt 30 ]]; do
   nmo_pod=$(kubectl get pods -n olm | grep node-maintenance-operator | head -1 | awk '{ print $1 }')
   if [[ "$nmo_pod" != "" ]]; then
	  break
   fi
   sleep 10s
   ((retry_count+=1))
 done

 kubectl wait --for=condition=available --timeout=600s deployment/node-maintenance-operator -n  olm

 echo "nmo operator pod available"

 echo "*** create nmo with invalid node ***"

set +e

MSG=$(NMO_NAME="nodemaintenance-xyz" NODE_NAME="nodeMiau" ./hack/test-olm-make-nmo-on-node.sh  | kubectl create -f 2>&1 -)

if [[ $? == 0 ]]; then
	echo "test failed, succeeded to create an nmo object on non existing node. "
	exit
fi

CNT=$(echo "$MSG" | grep -c 'admission webhook "nodemaintenance-validator.kubevirt.io" denied the request')

if [[  "$CNT" != "1" ]]; then
	echo "Unexpected error message. actual message: <msg>${MSG}</msg>"
	exit 1
fi

realNodeName=$(kubectl get nodes | sed '1d' | awk '{ print $1 }' | tail -n 1)
if [[ "$realNodeName" == "" ]]; then
	echo "can't get the real node name"
	exit 1
fi

# create valid nmo
NMO_NAME="nodemaintenance-x123" NODE_NAME="${realNodeName}" ./hack/test-olm-make-nmo-on-node.sh | kubectl create -f 2>&1 -

if [[ $? != 0 ]]; then
	echo "test failed, can't create nmo on existing node. "
	exit
fi

MSG=$(NMO_NAME="nodemaintenance-x1234" NODE_NAME="${realNodeName}" ./hack/test-olm-make-nmo-on-node.sh  | kubectl create -f 2>&1 -)

if [[ $? == 0 ]]; then
	echo "test failed, can create second nmo on existing node. "
	exit
fi

CNT=$(echo "$MSG" | grep -c 'NMO object already working with this node')

if [[  "$CNT" != "1" ]]; then
	echo "Unexpected error message. actual message: <msg>${MSG}</msg>"
	exit 1
fi
}

log_to_file
if [[ $OPT == "minikube" ]]; then
	start_registry
fi
setup_src
setup_utils
start_cluster
make_olm_op_bundle_new
run_bundle
test_nmo_webhook

echo "*** webhook is working with olm. All systems running. liftoff. ***"

