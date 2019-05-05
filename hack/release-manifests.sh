#!/usr/bin/env bash
set -e

REPO_ORG=kubevirt
REPO_NAME=node-maintenance-operator
PROJECT_ROOT="$(readlink -e $(dirname "$BASH_SOURCE[0]")/../)"
OUT_DIR=${PROJECT_ROOT}/_out

VERSION=$(git ls-remote https://github.com/${REPO_ORG}/${REPO_NAME}.git | tail -1 | tr -d '^{}' | awk '{ print $2}' | cut -f 3 -d / | cut -f 2 -d v)
echo $CSV_VERSION
CSV_VERSION="${CSV_VERSION:-$VERSION}"

MANIFESTS_DIR=manifests/node-maintenance-operator/v${CSV_VERSION}
CSV_FILE=${MANIFESTS_DIR}/node-maintenance-operator.v${CSV_VERSION}.clusterserviceversion.yaml
echo $CSV_FILE

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



