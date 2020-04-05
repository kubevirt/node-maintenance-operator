#!/bin/bash

set -ex

SELF=$( realpath $0 )
BASEPATH=$( dirname $SELF )
OPERATOR_SDK_VERSION="${1:-v0.16.0}"

if ! [ -x "${BASEPATH}/../operator-sdk" ]; then
	which operator-sdk &> /dev/null || {
        echo "operator-sdk not found (see https://github.com/operator-framework/operator-sdk)"
        curl -JL https://github.com/operator-framework/operator-sdk/releases/download/${OPERATOR_SDK_VERSION}/operator-sdk-${OPERATOR_SDK_VERSION}-x86_64-linux-gnu -o operator-sdk
        chmod 0755 operator-sdk
	}
fi

OP_VERSION=$(operator-sdk version  | awk '{ print $3 }' | sed 's/,$//')

if [[ "$OP_VERSION" != $OPERATOR_SDK_VERSION ]]; then

     echo "operator-sdk current version $OP_VERSION but need $OPERATOR_SDK_VERSION see https://github.com/operator-framework/operator-sdk)"
     curl -JL https://github.com/operator-framework/operator-sdk/releases/download/${OPERATOR_SDK_VERSION}/operator-sdk-${OPERATOR_SDK_VERSION}-x86_64-linux-gnu -o operator-sdk
     chmod 0755 operator-sdk

fi
