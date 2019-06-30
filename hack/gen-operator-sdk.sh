#!/bin/bash

set -ex

SELF=$( realpath $0 )
BASEPATH=$( dirname $SELF )
OPERATOR_SDK_VERSION="${1:-v0.8.0}"

if ! [ -x "${BASEPATH}/../operator-sdk" ]; then
	which operator-sdk &> /dev/null || {
        echo "operator-sdk not found (see https://github.com/operator-framework/operator-sdk)"
        curl -JL https://github.com/operator-framework/operator-sdk/releases/download/${OPERATOR_SDK_VERSION}/operator-sdk-${OPERATOR_SDK_VERSION}-x86_64-linux-gnu -o operator-sdk
        chmod 0755 operator-sdk
	}
fi