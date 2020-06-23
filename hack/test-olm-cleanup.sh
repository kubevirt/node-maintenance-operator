#!/bin/bash

set -x

OPT=${1:-kind}
DELOPT=$2

if [[ $OPT != "kind" ]] && [[ $OPT != "minikube" ]]; then
	echo "error: argument must be either kind or minikube"
	exit 1
fi

LOCAL_OLM=olm-repo
LOCAL_OPREG=operator-registry

setup_utils() {
	export PATH=$PWD/bin:$PWD/$LOCAL_OPREG/bin:$PATH
}

stop_cluster() {

	if [[ $OPT == "minikube" ]]; then
		REG=$(docker ps | grep registry:2[[:space:]] | awk '{ print $1 }')
		docker stop $REG
		minikube stop
		minikube delete
	fi
	if [[ $OPT == "kind" ]]; then
		./hack/kind-with-registry-stop.sh
	fi
}

deletefiles() {
	rm -rf ${LOCAL_OLM}
	rm -rf ${LOCAL_OPREG}
	rm -rf bin
}

setup_utils
stop_cluster

if [[ $DELOPT == "rm" ]]; then
	deletefiles
fi
