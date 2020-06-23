#!/bin/bash

set -ex

OPT=${1:-kind}

if [[ $OPT != "kind" ]] && [[ $OPT != "minikube" ]]; then
	echo "error: argument must be either kind or minikube"
	exit 1
fi

LOCAL_OLM=olm-repo
LOCAL_OPREG=operator-registry

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

	if [[ ! -f $LOCAL_OPREG/repo_init ]]; then
		git clone https://github.com/operator-framework/operator-registry $LOCAL_OPREG
		pushd $LOCAL_OPREG
		make
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

	export PATH=$PWD/bin:$PWD/$LOCAL_OPREG/bin:$PATH
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

	cat >tmp-manifest/node-maintenance-operator/metadata/annotations.yaml <<EOF
annotations:
  operators.operatorframework.io.bundle.mediatype.v1: "registry+v1"
  operators.operatorframework.io.bundle.manifests.v1: "manifests/"
  operators.operatorframework.io.bundle.metadata.v1: "metadata/"
  operators.operatorframework.io.bundle.package.v1: "node-maintenance-operator"
  operators.operatorframework.io.bundle.channels.v1: "beta"
  operators.operatorframework.io.bundle.channel.default.v1: "beta"
EOF

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

	cat >docker.tmp <<EOF
FROM scratch

# We are pushing an operator-registry bundle
# that has both metadata and manifests.
LABEL operators.operatorframework.io.bundle.mediatype.v1=registry+v1
LABEL operators.operatorframework.io.bundle.manifests.v1=manifests/
LABEL operators.operatorframework.io.bundle.metadata.v1=metadata/
LABEL operators.operatorframework.io.bundle.package.v1=node-maintenance-operator
LABEL operators.operatorframework.io.bundle.channels.v1=beta
LABEL operators.operatorframework.io.bundle.channel.default.v1=beta

COPY ./tmp-manifest/node-maintenance-operator/manifests  /manifests/
ADD ./tmp-manifest/node-maintenance-operator/metadata/annotations.yaml /metadata/annotations.yaml

EOF

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

cat <<EOF | kubectl create -f -
apiVersion: operators.coreos.com/v1alpha1
kind: CatalogSource
metadata:
  name: nmocatalogsource
  namespace: ${CATALOG_NAMESPACE}
spec:
  sourceType: grpc
  image: ${REG_IMG}
  displayName: KubeVirt HyperConverged
  publisher: Red Hat
EOF

cat <<EOF | kubectl create -f -
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: node-maintenance-operator
  namespace: ${CATALOG_NAMESPACE}
spec:
  channel: "beta"
  installPlanApproval: Automatic
  name: node-maintenance-operator
  source: nmocatalogsource
  sourceNamespace: ${CATALOG_NAMESPACE}
EOF
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
MSG=$(cat <<EOF  | kubectl create -f 2>&1 -
apiVersion: nodemaintenance.kubevirt.io/v1beta1
kind: NodeMaintenance
metadata:
  name: nodemaintenance-xyz
spec:
  nodeName: nodeMiau
EOF
)

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

MSG=$(cat <<EOF  | kubectl create -f 2>&1 -
apiVersion: nodemaintenance.kubevirt.io/v1beta1
kind: NodeMaintenance
metadata:
  name: nodemaintenance-x123
spec:
  nodeName: ${realNodeName}
EOF
)

if [[ $? != 0 ]]; then
	echo "test failed, can't create nmo on existing node. "
	exit
fi


MSG=$(cat <<EOF  | kubectl create -f 2>&1 -
apiVersion: nodemaintenance.kubevirt.io/v1beta1
kind: NodeMaintenance
metadata:
  name: nodemaintenance-x1234
spec:
  nodeName: ${realNodeName}
EOF
)

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

