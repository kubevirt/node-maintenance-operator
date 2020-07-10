#!/bin/bash

set -ex

SELF=$( realpath $0 )
BASEPATH=$( dirname $SELF )
. "${BASEPATH}/gen-opm.sh"

# Skip adding an bundle by using an empty bundle image, just generate the Dockerfile (and an empty database)
# Reason: we don't want to use the bundle image for creating the db, we have the manifests available,
# So we don't need to pull images which is difficult in CI
# We create the database using the "initializer"
#BUNDLE="${IMAGE_REGISTRY}/${BUNDLE_IMAGE}:${IMAGE_TAG}"

"${OPM}" index add --bundles "${BUNDLE}" --generate --out-dockerfile build/index.Dockerfile --skip-tls --permissive

# Create the database
docker run -v "${BASEPATH}/..":/sources quay.io/operator-framework/upstream-registry-builder /bin/initializer -m /sources/manifests/node-maintenance-operator/v"${OPERATOR_VERSION_NEXT}" -o /sources/database/index.db
