#!/bin/bash

set -ex

SELF=$( realpath $0 )
BASEPATH=$( dirname $SELF )
OPERATOR_SDK_VERSION="${1:-v0.8.0}"

if [ -x "${BASEPATH}/../operator-sdk" ]
then
   ${BASEPATH}/../operator-sdk generate k8s
else
   operator-sdk generate k8s
fi