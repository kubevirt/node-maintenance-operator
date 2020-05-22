#!/bin/bash -e

GINKGO="$1"
TARGETCOVERAGE="$2"

COVERAGE_FILE=cover.out
GINKGO_COVERAGE_ARGS="-cover -coverprofile=${COVERAGE_FILE} -outputdir=.  --skipPackage ./vendor"
GINKGO_ARGS="-v -r --progress ${GINKGO_EXTRA_ARGS} ${GINKGO_COVERAGE_ARGS}"

# source files excluded from ginkgo coverage report. these files are not used during the unit test and include code that is only relevant to the installed product.
declare -a EXCLUDE_FILES_FROM_COVERAGE=("nodemaintenance_controller_init.go")

# delete coverage files (if present)
find . -name ${COVERAGE_FILE} | xargs rm -f

REPEAT_COUNT=0
MAX_REPEAT_COUNT=10
while [[ $REPEAT_COUNT -lt $MAX_REPEAT_COUNT ]]; do

		# run ginkgo with coverage result line
		GINKGO_OUT=$(${GINKGO} ${GINKGO_ARGS} ./pkg/ ./cmd/ 2>&1 | sed  '/coverage:.*$/d')
		GSTAT=${PIPESTATUS[0]}
		echo "$GINKGO_OUT"

		if [[ $GSTAT -eq 0 ]]; then
			break
		fi

		# sometimes the error message is 'Unable to read coverage file' for no apparent reason
		# repeat the test if that is so.
		#
		# See also: https://github.com/onsi/ginkgo/issues/417
		#  'coverage:' lines hold no clue and unsetting -outputdir produced no effect
		set +e
		echo "$GINKGO_OUT" | grep "Unable to read coverage file"
		STAT=$?
		set -e

		if [[ $STAT -ne 0 ]]; then
			break
		fi
		echo "repeating ginkgo run, unable to read coverage"
		((REPEAT_COUNT+=1))
done

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

	grep -v $term ${file} >${file}.tmp
	mv -f ${file}.tmp $file
}

# so that the function can be called from xargs
export -f exclude_file

# exclude files listed as excluded from coverage report
for f in "${EXCLUDE_FILES_FROM_COVERAGE}"; do
	exclude_file "$f" "${COVERAGE_FILE}"
done

#echo "now exit"
#exit 1


# function coverage report (textual)
FUNC_REP=$(go tool cover -func=$COVERAGE_FILE)

echo "$FUNC_REP"

COVERAGENUM=$(echo "$FUNC_REP" | sed -n 's/^total:[[:blank:]]*(statements)[[:blank:]]*\([0-9\.]*\)\%$/\1/p')

ROUNDEDCOVERAGE=$(echo "$COVERAGENUM" | awk '{print int($1+0.5)}')

if [[ "$ROUNDEDCOVERAGE" -lt "$TARGETCOVERAGE" ]]; then
	echo "Error: actual coverage $ROUNDEDCOVERAGE of unit tests is less then the target coverage $TARGETCOVERAGE"
	exit 1
fi

# html coverage report (this makes sense if run in interactive mode - go tool displays the results in the current browser)
go tool cover -html=$COVERAGE_FILE

