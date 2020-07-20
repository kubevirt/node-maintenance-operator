#!/usr/bin/env bash
set -e

PROJECT_ROOT="$(readlink -e $(dirname "$BASH_SOURCE[0]")/../)"
OUT_DIR=${PROJECT_ROOT}/_out

TAG="${1:-latest}"

if [ "${TAG}" == "latest" ]; then
    echo "Manifests release will not apply on \"latest\" tag"
    exit 0
fi

VERSION=${TAG#v}
OPERATOR="${IMAGE_REGISTRY}/${OPERATOR_IMAGE}:${VERSION}"

rm -rf ${OUT_DIR}
mkdir -p ${OUT_DIR}

cp deploy/operator.yaml ${OUT_DIR}/operator.yaml
sed -i "s/REPLACE_IMAGE/${OPERATOR}/g" ${OUT_DIR}/operator.yaml

cp manifests/node-maintenance-operator/${TAG}/manifests/* ${OUT_DIR}/
