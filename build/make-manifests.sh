#!/bin/bash

set -ex

SELF=$( realpath $0 )
BASEPATH=$( dirname $SELF )

# intentionally "impossible"/obviously wrong version
TAG="${1:-v0.0.0}"
VERSION=${TAG#v}  # prune initial 'v', which should be present

BUNDLE_DIR="${2:-manifests/node-maintenance-operator}"
BUNDLE_DIR_VERSION="${BUNDLE_DIR}/${TAG}"
CHANNEL="beta"

if [ -x "${BASEPATH}/../operator-sdk" ]; then
       OPERATOR_SDK="${BASEPATH}/../operator-sdk"
else       
	which operator-sdk &> /dev/null || {
		echo "operator-sdk not found (see https://github.com/operator-framework/operator-sdk)"
		exit 1
	}
	OPERATOR_SDK="operator-sdk"
fi

HAVE_COURIER=0
if which operator-courier &> /dev/null; then
	HAVE_COURIER=1
fi

# store original operator.yaml file
cp deploy/operator.yaml operator.yaml
sed -i "s/<IMAGE_VERSION>/${TAG}/g" deploy/operator.yaml

mkdir -p ${BUNDLE_DIR_VERSION}

# note: this creates under deploy/olm-catalog ...
${OPERATOR_SDK} olm-catalog gen-csv --csv-version ${VERSION} 

# move back original operator.yaml file
mv operator.yaml deploy/operator.yaml

./build/update-olm.py \
	deploy/olm-catalog/node-maintenance-operator/${VERSION}/node-maintenance-operator.${TAG}.clusterserviceversion.yaml > \
	${BUNDLE_DIR_VERSION}/node-maintenance-operator.${TAG}.clusterserviceversion.yaml

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

if [ "${HAVE_COURIER}" == "1" ]; then
    operator-courier verify ${BUNDLE_DIR_VERSION} && echo "OLM verify passed" || echo "OLM verify failed"
fi

rm "${BUNDLE_DIR_VERSION}/node-maintenance-operator.package.yaml"