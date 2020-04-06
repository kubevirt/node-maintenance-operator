#!/bin/bash

set -ex

SELF=$( realpath $0 )
BASEPATH=$( dirname $SELF )
TARGET_SDK_VERSION="${1:-v0.16.0}"

if ! [ -x "${BASEPATH}/../operator-sdk" ]; then
	which operator-sdk &> /dev/null || {
        echo "operator-sdk not found (see https://github.com/operator-framework/operator-sdk)"
        curl -JL https://github.com/operator-framework/operator-sdk/releases/download/${TARGET_SDK_VERSION}/operator-sdk-${TARGET_SDK_VERSION}-x86_64-linux-gnu -o operator-sdk
        chmod 0755 operator-sdk
	}
fi

DETECTED_SDK_VERSION=$(operator-sdk version  | awk '{ print $3 }' | sed 's/,$//')

if [[ "$DETECTED_SDK_VERSION" != $TARGET_SDK_VERSION ]]; then

     echo "operator-sdk current version $DETECTED_SDK_VERSION but need $TARGET_SDK_VERSION see https://github.com/operator-framework/operator-sdk)"
     curl -JL https://github.com/operator-framework/operator-sdk/releases/download/${TARGET_SDK_VERSION}/operator-sdk-${TARGET_SDK_VERSION}-x86_64-linux-gnu -o operator-sdk
     chmod 0755 operator-sdk

fi


# install the operator courier for the current user
# the executable is downloaded into $HOME/.local/bin/operator-courier
echo "install operator courier"
pip3 install --user operator-courier

