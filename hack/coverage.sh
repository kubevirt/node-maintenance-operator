#!/bin/bash -ex

GINKGO="$1"
COVERAGE_FILE=cover.out
GINKGO_COVERAGE_ARGS="-cover -coverprofile=${COVERAGE_FILE} -outputdir=. --skipPackage ./vendor"
GINKGO_ARGS="-v -r --progress ${GINKGO_EXTRA_ARGS} ${GINKGO_COVERAGE_ARGS}"

# source files excluded from ginkgo coverage report. these files are not used during the unit test and include code that is only relevant to the installed product.
declare -a EXCLUDE_FILES_FROM_COVERAGE=("nodemaintenance_controller_init.go")

# delete coverage files (if present)
find . -name ${COVERAGE_FILE} | xargs rm -f

# run ginkgo with coverage result line
${GINKGO} ${GINKGO_ARGS} ./pkg/ ./cmd/ | sed  '/coverage:.*$/d'
GSTAT=${PIPESTATUS[0]}

if [[ $GSTAT != 0 ]]; then
	echo "* ginkgo run failed *"
	exit 1
fi


# ginkgo and html coverage don't quite live in harmony. fix that.
# ginkgo aggregates the coverage file, but the resulting file is not accepted by go tool cover.
# the reason is that there are repeated mode: headers, we need to leave just the first one of them
sed -i  '/mode: atomic/d' $COVERAGE_FILE
sed -i '1i mode: atomic' $COVERAGE_FILE

function exclude_file {
	local file=$2
	local term=$1
	set -x

	grep -v $term ${file} >${file}.tmp
	mv -f ${file}.tmp $file
}

# so that the function can be called from xargs
export -f exclude_file

# exclude files listed as excluded from coverage report
for f in "${EXCLUDE_FILES_FROM_COVERAGE}"; do
	find . -name $COVERAGE_FILE | xargs bash -c "exclude_file $f $@"
done

# function coverage report (textual)
go tool cover -func=$COVERAGE_FILE

# html coverage report (this makes sense if run in interactive mode - go tool displays the results in the current browser)
go tool cover -html=$COVERAGE_FILE

