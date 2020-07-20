#!/bin/bash

# This script creates manifest for the upcoming version by
# - taking a template CSV and package manifest from /manifests/template and putting into /deploy/olm-catalog (the default working dir of operator-sdk)
# - updating CSV and package manifests, generating bundle metadata
# - copying everything into the release dir in /manifests
#
# Note: dealing with bundle images and creating index images from them is difficult in CI
# We use the "initializer" instead for creating the regisrty database
# That's why we still need a package manifest: the "initializer" does not read the annotation file from the metadata
# See ./generate-index.sh

set -ex

SELF=$( realpath $0 )
BASEPATH=$( dirname $SELF )
. "${BASEPATH}/get-operator-sdk.sh"

CSV_TEMPLATE=manifests/node-maintenance-operator/template/node-maintenance-operator.clusterserviceversion.yaml
PACKAGE_TEMPLATE=manifests/node-maintenance-operator/template/node-maintenance-operator.package.yaml
WORKING_DIR=deploy/olm-catalog/node-maintenance-operator
WORKING_CSV="${WORKING_DIR}/manifests/node-maintenance-operator.clusterserviceversion.yaml"
WORKING_PACKAGE="${WORKING_DIR}/node-maintenance-operator.package.yaml"
NEW_DIR=manifests/node-maintenance-operator/v${OPERATOR_VERSION_NEXT}

# Copy templates to deploy dir, will be used as base CSV / package manifest
cp "${CSV_TEMPLATE}" "${WORKING_CSV}"
cp "${PACKAGE_TEMPLATE}" "${WORKING_DIR}/"

# Generate new manifests
${OPERATOR_SDK} generate csv --csv-version "${OPERATOR_VERSION_NEXT}" --update-crds

# Replace operator image
# OVERRIDE_MANIFEST_REGISTRY is set if we need another registry during runtime in the manifest than during build time when pushing images
# That is needed e.g. for KubeVirtCI: at build time we push to a local registry at localhost:<some_port>, during runtime we need to pull
# from registry:5000
REGISTRY="${OVERRIDE_MANIFEST_REGISTRY:-$IMAGE_REGISTRY}"
OPERATOR="${REGISTRY}/${OPERATOR_IMAGE}:${IMAGE_TAG}"
sed -i "s|REPLACE_IMAGE|${OPERATOR}|g" "${WORKING_CSV}"

# Remove replace directive
# TODO check if we need replace, and if so how to create the index image while old version is delivered by HCO
sed -i '/  replaces: node-maintenance-operator.v*/d' "${WORKING_CSV}"

# Generate bundle metadata
${OPERATOR_SDK} bundle create --generate-only --channels "${OLM_CHANNEL}" --default-channel "${OLM_CHANNEL}" --directory "${WORKING_DIR}/manifests" --overwrite

# Update package
sed -i "s|CHANNEL|${OLM_CHANNEL}|g" "${WORKING_PACKAGE}"
sed -i "s|OPERATOR_VERSION_NEXT|${OPERATOR_VERSION_NEXT}|g" "${WORKING_PACKAGE}"

# Copy new manifests to permanent location
mkdir -p "${NEW_DIR}"
cp -a "${WORKING_DIR}"/* "${NEW_DIR}"/

# Move bundle.Dockerfile
mv ./bundle.Dockerfile ./build/
