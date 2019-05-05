#!/usr/bin/env bash
set -e

PROJECT_ROOT="$(readlink -e $(dirname "$BASH_SOURCE[0]")/../)"
OUT_DIR=${PROJECT_ROOT}/_out

if [ "$CSV_VERSION" == "latest" ]; then
  echo "Running latest release, no need to verify manifests."
  exit 0
fi

CSV_VERSION=$(echo ${CSV_VERSION} | cut -c 2- )
MANIFESTS_DIR=manifests/node-maintenance-operator/v${CSV_VERSION}
CSV_FILE=${MANIFESTS_DIR}/node-maintenance-operator.v${CSV_VERSION}.clusterserviceversion.yaml

if [ ! -d "$MANIFESTS_DIR" ] || [ ! -f "$CSV_FILE" ] ; then
  echo "Manifests under directory ${MANIFESTS_DIR} for version v${CSV_VERSION} do not exist."
  echo "To create manifests for v${CSV_VERSION} run: "
  echo "./build/make-olm.sh ${CSV_VERSION}"
  exit 1
fi

rm -rf ${OUT_DIR}
mkdir -p ${OUT_DIR}

cp deploy/operator.yaml ${OUT_DIR}/operator.yaml
sed -i "s/<IMAGE_VERSION>/v${CSV_VERSION}/g" ${OUT_DIR}/operator.yaml



