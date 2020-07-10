#!/bin/bash

set -ex

SELF=$( realpath $0 )
BASEPATH=$( dirname $SELF )
. "${BASEPATH}/gen-operator-sdk.sh"

"${OPERATOR_SDK}" generate k8s
