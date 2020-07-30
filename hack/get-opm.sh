#!/bin/bash

set -ex

SELF=$(realpath $0)
BASEPATH=$(dirname $SELF)
TARGET_OPM_VERSION="${1:-$OPM_VERSION}"

CURRENT_OPM=${BASEPATH}/../opm

if [[ ! -x $CURRENT_OPM ]]; then
    set +e
    CURRENT_OPM=$(which opm)
    set -e
fi

DETECTED_OPM_VERSION=v$($CURRENT_OPM version | sed -r 's/^.*OpmVersion:"([0-9.]+)".*$/\1/g')

function check_need_upgrade() {
    local detectedversion="$2"
    local desiredversion="$1"
    local needupgrade=0
    local parseddetectedversion
    local parseddesiredversion

    if [[ $detectedversion == "" || $desiredversion == "" ]]; then
        needupgrade=1
    else

        IFS=$' ' read -r -a parseddetectedversion <<<$(echo "$detectedversion" | sed -n 's/v\([[:digit:]]*\).\([[:digit:]]*\).\([[:digit:]]*\).*$/\1 \2 \3/p')
        IFS=$' ' read -r -a parseddesiredversion <<<$(echo "$desiredversion" | sed -n 's/v\([[:digit:]]*\).\([[:digit:]]*\).\([[:digit:]]*\).*$/\1 \2 \3/p')

        for i in $(seq 1 3); do
            if [[ ${parseddetectedversion[$i]} -lt ${parseddesiredversion[$i]} ]]; then
                needupgrade=1
                break
            fi
        done
    fi
    echo "$needupgrade"

}

echo "detected opm version  $DETECTED_OPM_VERSION"

need_upgrade=$(check_need_upgrade "$TARGET_OPM_VERSION" "$DETECTED_OPM_VERSION")

if [[ $need_upgrade == "1" ]]; then

    echo "opm current version $DETECTED_OPM_VERSION but need $TARGET_OPM_VERSION see https://github.com/operator-framework/operator-registry/releases)"
    curl -JL https://github.com/operator-framework/operator-registry/releases/download/${TARGET_OPM_VERSION}/linux-amd64-opm -o ${BASEPATH}/../opm
    chmod 0755 ${BASEPATH}/../opm

fi

# set OPM var
if [ -x "${BASEPATH}/../opm" ]; then
    OPM="${BASEPATH}/../opm"
else
    which opm &>/dev/null || {
        echo "opm not found (see https://github.com/operator-framework/operator-registry/releases)"
        exit 1
    }
    OPM="opm"
fi

echo "opm version: "
${OPM} version
