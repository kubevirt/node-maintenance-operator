#!/bin/bash

set -ex

SELF=$( realpath $0 )
BASEPATH=$( dirname $SELF )


if [ -x "${BASEPATH}/../operator-sdk" ]
then
   GO111MODULE="auto" ${BASEPATH}/../operator-sdk generate k8s
else
   GO111MODULE="auto" operator-sdk generate k8s
fi
