#!/bin/bash

set -ex

SELF=$( realpath $0 )
BASEPATH=$( dirname $SELF )
. "${BASEPATH}/get-operator-sdk.sh"

"${OPERATOR_SDK}" generate crds --crd-version v1beta1
