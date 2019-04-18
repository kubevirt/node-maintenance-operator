#!/bin/bash

CSV_VERSION="${1:-0.0.0}"
BUNDLE_DIR="manifests/node-maintenance-operator/"
BUNDLE_DIR_VERSION="${BUNDLE_DIR}${CSV_VERSION}/"


# TODO: tested with operator-sdk 0.7.0: should we require it?
which operator-sdk &> /dev/null || {
	echo "operator-sdk not found (see https://github.com/operator-framework/operator-sdk)"
	exit 1
}

which operator-courier &> /dev/null || {
	echo "operator-courier not found (see https://github.com/operator-framework/operator-courier)"
	exit 2
}

mkdir -p ${BUNDLE_DIR_VERSION}

operator-sdk olm-catalog gen-csv --csv-version ${CSV_VERSION}

./build/update-olm.py \
	deploy/olm-catalog/node-maintenance-operator/${CSV_VERSION}/node-maintenance-operator.v${CSV_VERSION}.clusterserviceversion.yaml > \
	${BUNDLE_DIR_VERSION}/node-maintenance-operator.v${CSV_VERSION}.clusterserviceversion.yaml

# caution: operator-courier (as in 5a4852c) wants *one* entity per yaml file (e.g. it does NOT use safe_load_all)
for CRD in $( ls deploy/crds/nodemaintenance_*crd.yaml ); do
	cp ${CRD} ${BUNDLE_DIR_VERSION}
done

cat << EOF > ${BUNDLE_DIR}/node-maintenance-package.yaml
packageName: node-maintenance
channels:
  - name: beta
    currentCSV: node-maintenance-operator.v${CSV_VERSION}
EOF

echo "built these manifests:"
ls ${BUNDLE_DIR_VERSION}

cp "${BUNDLE_DIR}node-maintenance-package.yaml" "${BUNDLE_DIR_VERSION}node-maintenance-package.yaml"

operator-courier verify --ui_validate_io ${BUNDLE_DIR_VERSION} && echo "OLM verify passed" || echo "OLM verify failed"

rm "${BUNDLE_DIR_VERSION}node-maintenance-package.yaml"