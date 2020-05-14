#!/bin/bash

set -ex

SELF=$( realpath $0 )
BASEPATH=$( dirname $SELF )

if [ -x "${BASEPATH}/../operator-sdk" ]
then
   ${BASEPATH}/../operator-sdk generate k8s
else
   operator-sdk generate k8s
fi
