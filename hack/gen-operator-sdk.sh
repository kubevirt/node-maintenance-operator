#!/bin/bash

set -ex

SELF=$( realpath $0 )
BASEPATH=$( dirname $SELF )
TARGET_SDK_VERSION="${1:-v0.16.0}"

CURRENT_OPERATOR_SDK=${BASEPATH}/../operator-sdk

if [[ ! -x $CURRENT_OPERATOR_SDK ]]; then
    set +e
	CURRENT_OPERATOR_SDK=$(which operator-sdk)
	set -e
fi

if [[ "$CURRENT_OPERATOR_SDK" != "" ]]; then
	DETECTED_SDK_VERSION=$($CURRENT_OPERATOR_SDK version  | awk '{ print $3 }' | sed 's/,$//')
else
	DETECTED_SDK_VERSION="0.0.0"
fi
function check_need_upgrade {
	local detectedversion="$2"
	local desiredversion="$1"
	local needupgrade=0
	local parseddetectedversion
	local parseddesiredversion

	if [[ $detectedversion == "" || $desiredversion == "" ]]; then
		needupgrade=1
	else

		IFS=$' ' read  -r -a parseddetectedversion <<< $(echo "$detectedversion" | sed -n 's/v\([[:digit:]]*\).\([[:digit:]]*\).\([[:digit:]]*\).*$/\1 \2 \3/p')
		IFS=$' ' read  -r -a parseddesiredversion <<< $(echo "$desiredversion" | sed -n 's/v\([[:digit:]]*\).\([[:digit:]]*\).\([[:digit:]]*\).*$/\1 \2 \3/p')

		for i in $(seq 1 3); do
			if [[ ${parseddetectedversion[$i]} -lt ${parseddesiredversion[$i]}  ]]; then
				needupgrade=1
				break
			fi
		done
	fi
	echo "$needupgrade"

}

echo "detected sdk version  $DETECTED_SDK_VERSION"

need_upgrade=$(check_need_upgrade "$TARGET_SDK_VERSION" "$DETECTED_SDK_VERSION")

if [[ $need_upgrade == "1" ]]; then

     echo "operator-sdk current version $DETECTED_SDK_VERSION but need $TARGET_SDK_VERSION see https://github.com/operator-framework/operator-sdk)"
     curl -JL https://github.com/operator-framework/operator-sdk/releases/download/${TARGET_SDK_VERSION}/operator-sdk-${TARGET_SDK_VERSION}-x86_64-linux-gnu -o ${BASEPATH}/../operator-sdk
     chmod 0755 ${BASEPATH}/../operator-sdk

fi

