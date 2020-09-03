#!/usr/bin/env bash
set -e

PROJECT_ROOT="$(readlink -e $(dirname "$BASH_SOURCE[0]")/../)"
OUT_DIR=${PROJECT_ROOT}/_out

rm -rf ${OUT_DIR}
mkdir -p ${OUT_DIR}

if [ "${IMAGE_TAG}" == "latest" ]; then
    echo "Manifests release will not apply on \"latest\" tag"
    exit 1
fi

# copy everything we want to release to $OUT_DIR
# it was build by the generate-bundle make target dependency already
cp manifests/node-maintenance-operator/${IMAGE_TAG}/manifests/* ${OUT_DIR}/
