#!/bin/bash

set -ex

SELF=$( realpath $0 )
BASEPATH=$( dirname $SELF )
TARGET_SDK_VERSION="${1:-v0.16.0}"

DETECTED_SDK_VERSION=$(${BASEPATH}/../operator-sdk version  | awk '{ print $3 }' | sed 's/,$//')

if [[ "$DETECTED_SDK_VERSION" != $TARGET_SDK_VERSION ]]; then

     echo "operator-sdk current version $DETECTED_SDK_VERSION but need $TARGET_SDK_VERSION see https://github.com/operator-framework/operator-sdk)"
     curl -JL https://github.com/operator-framework/operator-sdk/releases/download/${TARGET_SDK_VERSION}/operator-sdk-${TARGET_SDK_VERSION}-x86_64-linux-gnu -o ${BASEPATH}/../operator-sdk
     chmod 0755 ${BASEPATH}/../operator-sdk

fi

