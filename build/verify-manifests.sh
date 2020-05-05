#!/bin/bash -ex

TAG_CURRENT="${1}"
BUNDLE_DIR="${2:-manifests/node-maintenance-operator}"
VERIFY_DIR="manifests/verify-dir"

function compare_files {
	local file_a="$1"
	local file_b="$2"

	set +e
	diff "$file_a" "$file_b"
	if [[ $? != 0 ]]; then
		echo "$file_b"
	fi
	set -e
}

# build manifests in verification directory for purpose of comparison with the current version
mkdir -p "${VERIFY_DIR}" || true
./build/make-manifests.sh  "${TAG_CURRENT}" "${VERIFY_DIR}"

#compare the crd files
file="nodemaintenance_crd.yaml"
compare_files "${BUNDLE_DIR}/${TAG_CURRENT}/${file}" "${VERIFY_DIR}/${TAG_CURRENT}/${file}"

#compare csv file
file="node-maintenance-operator.${TAG_CURRENT}.clusterserviceversion.yaml"
compare_files "${BUNDLE_DIR}/${TAG_CURRENT}/${file}" "${VERIFY_DIR}/${TAG_CURRENT}/${file}"

rm -rf "${VERIFY_DIR}"
