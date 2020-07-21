#!/bin/bash

set -ex

SELF=$(realpath $0)
BASEPATH=$(dirname $SELF)
. "${BASEPATH}/get-operator-sdk.sh"

MANIFEST_DIR=manifests/node-maintenance-operator/v${OPERATOR_VERSION_NEXT}

${OPERATOR_SDK} bundle validate "${MANIFEST_DIR}"
