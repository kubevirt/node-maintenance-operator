#!/bin/bash -ex

COVERAGE_FILE=$1

declare -a EXCLUDE_FILES_FROM_COVERAGE=("nodemaintenance_controller_init.go")

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

# exclude files listed from coverage report
for f in "${EXCLUDE_FILES_FROM_COVERAGE}"; do
	find . -name $COVERAGE_FILE | xargs bash -c "exclude_file $f $@"
done

# function coverage report (textual)
go tool cover -func=$COVERAGE_FILE

# html coverage report
go tool cover -html=$COVERAGE_FILE

