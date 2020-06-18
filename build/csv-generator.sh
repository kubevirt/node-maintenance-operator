#!/bin/bash

set -e

MANIFESTS_GENERATED_DIR="manifests/generated"
CRDS_DIR="deploy/crds"
if ! [ -d $MANIFESTS_GENERATED_DIR ]; then
    MANIFESTS_GENERATED_DIR="/manifests/generated"
    CRDS_DIR="/deploy/crds"
fi
MANIFESTS_GENERATED_CSV=${MANIFESTS_GENERATED_DIR}/node-maintenance-operator.vVERSION.clusterserviceversion.yaml
TMP_FILE=$(mktemp)

help_text() {
    echo "USAGE: csv-generator --csv-version=<version> --namespace=<namespace> --operator-image=<operator image> [optional args]"
    echo ""
    echo "ARGS:"
    echo "  --csv-version:    (REQUIRED) The version of the CSV file"
    echo "  --namespace:      (REQUIRED) The namespace set on the CSV file"
    echo "  --operator-image: (REQUIRED) The operator container image to use in the CSV file"
    echo "  --watch-namespace:   (OPTIONAL)"
    echo "  --dump-crds:         (OPTIONAL) Dumps CRD manifests with the CSV to stdout"
}

# REQUIRED ARGS
CSV_VERSION=""
NAMESPACE=""
OPERATOR_IMAGE=""

# OPTIONAL ARGS
WATCH_NAMESPACE=""

while (( "$#" )); do
    ARG=`echo $1 | awk -F= '{print $1}'`
    VAL=`echo $1 | awk -F= '{print $2}'`
    shift

    case "$ARG" in
    --csv-version)
        CSV_VERSION=$VAL
        ;;
    --namespace)
        NAMESPACE=$VAL
        ;;
    --operator-image)
        OPERATOR_IMAGE=$VAL
        ;;
    --watch-namespace)
        WATCH_NAMESPACE=$VAL
        ;;
    --dump-crds)
        DUMP_CRDS="true"
        ;;
    --)
        break
        ;;
    *) # unsupported flag
        echo "Error: Unsupported flag $ARG" >&2
        exit 1
        ;;
    esac
done

if [ -z "$CSV_VERSION" ] || [ -z "$NAMESPACE" ] || [ -z "$OPERATOR_IMAGE" ]; then
    echo "Error: Missing required arguments"
    help_text
    exit 1
fi

cp ${MANIFESTS_GENERATED_CSV} ${TMP_FILE}

# replace placeholder version with a human readable variable name
# that will be used later on by csv-generator
sed -i "s/PLACEHOLDER_CSV_VERSION/${CSV_VERSION}/g" ${TMP_FILE}
sed -i "s/namespace: node-maintenance-operator/namespace: ${NAMESPACE}/g" ${TMP_FILE}
sed -i "s|quay.io/kubevirt/node-maintenance-operator:<IMAGE_VERSION>|${OPERATOR_IMAGE}|g" ${TMP_FILE}

sed -ie 's/\([[:space:]]*\)fieldPath: metadata\.annotations\[.olm\.targetNamespaces.].*$/\1fieldPath: metadata.namespace/' ${TMP_FILE}

# dump CSV and CRD manifests to stdout
echo "---"
cat ${TMP_FILE}
rm ${TMP_FILE}
if [ "$DUMP_CRDS" = "true" ]; then
    for CRD in $( ls ${CRDS_DIR}/nodemaintenance_*crd.yaml ); do
        echo "---"
        cat ${CRD}
    done
fi
