#!/bin/bash

set -e

SELF=$( realpath $0 )
BASEPATH=$( dirname $SELF )

if [ -x "${BASEPATH}/../operator-sdk" ]; then
       OPERATOR_SDK="${BASEPATH}/../operator-sdk"
else
    which operator-sdk &> /dev/null || {
        echo "operator-sdk not found (see https://github.com/operator-framework/operator-sdk)"
        exit 1
    }
    OPERATOR_SDK="operator-sdk"
fi

MANIFESTS_GENERATED_DIR="manifests/generated"
MANIFESTS_GENERATED_CSV=${MANIFESTS_GENERATED_DIR}/node-maintenance-operator.vVERSION.clusterserviceversion.yaml
PLACEHOLDER_CSV_VERSION="9999.9999.9999"

# Create CSV with placeholder version. The version
# has to be semver compatible in order for the
# operator sdk to create it for us. That's why we
# are using the absurd 9999.9999.9999 version here.

${OPERATOR_SDK} generate csv --csv-version ${PLACEHOLDER_CSV_VERSION}

# Move CSV to generated folder
mv deploy/olm-catalog/node-maintenance-operator/${PLACEHOLDER_CSV_VERSION}/node-maintenance-operator.v${PLACEHOLDER_CSV_VERSION}.clusterserviceversion.yaml $MANIFESTS_GENERATED_CSV

# cleanup placeholder version's deployment dir
rm -rf mv deploy/olm-catalog/node-maintenance-operator/${PLACEHOLDER_CSV_VERSION}

# replace placeholder version with a human readable variable name
# that will be used later on by csv-generator
sed -i "s/${PLACEHOLDER_CSV_VERSION}/PLACEHOLDER_CSV_VERSION/g" $MANIFESTS_GENERATED_CSV

# inject the CRD and Description related data into the CSV
cp $MANIFESTS_GENERATED_CSV ${MANIFESTS_GENERATED_CSV}.tmp

python3 build/update-olm.py ${MANIFESTS_GENERATED_CSV}.tmp > ${MANIFESTS_GENERATED_CSV}
rm ${MANIFESTS_GENERATED_CSV}.tmp
