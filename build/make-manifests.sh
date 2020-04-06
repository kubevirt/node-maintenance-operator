#!/bin/bash

set -ex

SELF=$( realpath $0 )
BASEPATH=$( dirname $SELF )

TAG="${1:-latest}"

if [ "${TAG}" == "latest" ];  then
  echo "Manifests generation will not apply on \"latest\" tag"
  exit 0
fi

VERSION=${TAG#v}  # prune initial 'v', which should be present

BUNDLE_DIR="${2:-manifests/node-maintenance-operator}"
BUNDLE_DIR_VERSION="${BUNDLE_DIR}/${TAG}"
CHANNEL="beta"

mkdir -p ${BUNDLE_DIR_VERSION}

./build/csv-generator.sh --csv-version=${VERSION} --namespace=placeholder --operator-image="quay.io/kubevirt/node-maintenance-operator:${TAG}" > ${BUNDLE_DIR_VERSION}/node-maintenance-operator.${TAG}.clusterserviceversion.yaml

# caution: operator-courier (as in 5a4852c) wants *one* entity per yaml file (e.g. it does NOT use safe_load_all)
for CRD in $( ls deploy/crds/nodemaintenance_*crd.yaml ); do
	cp ${CRD} ${BUNDLE_DIR_VERSION}
done

cat << EOF > ${BUNDLE_DIR}/node-maintenance-operator.package.yaml
packageName: node-maintenance-operator
channels:
- name: ${CHANNEL}
  currentCSV: node-maintenance-operator.${TAG}
EOF

echo "built these manifests:"
ls ${BUNDLE_DIR_VERSION}

# needed to make operator-courier happy
cp "${BUNDLE_DIR}/node-maintenance-operator.package.yaml" "${BUNDLE_DIR_VERSION}/node-maintenance-operator.package.yaml"

set +e

echo "OLM verify bundle"
$HOME/.local/bin/operator-courier verify ${BUNDLE_DIR_VERSION}
if [[ $? != 0 ]]; then
	echo "OLM verify failed"
	exit 1
else
	echo "OLM verify passed"
fi

echo "OLM verify bundle for operator hub"
$HOME/.local/bin/operator-courier verify --ui_validate_io ${BUNDLE_DIR_VERSION}
if [[ $? != 0 ]]; then
	echo "OLM verify for operator hub failed"
	exit 1
else
	echo "OLM verify for operator hub passed"
fi



set -e

rm "${BUNDLE_DIR_VERSION}/node-maintenance-operator.package.yaml"





