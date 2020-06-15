#!/usr/bin/env bash

set -e

function fix_one_file() {
	local fname="$1"
 	sed --follow-symlinks 's/[[:space:]]*$//' ${fname} | sed 'N;/^\n$/D;P;D;' >${fname}.tmp
	cp -f ${fname}.tmp ${fname}
	rm -f ${fname}.tmp
}

export -f fix_one_file

function fix() {
    git ls-files -- ':!vendor/' | grep -vE '^kubevirtci/' | xargs -I {} bash -c 'fix_one_file "{}"'
}

function check() {
    invalid_files=$(git ls-files -- ':!vendor/' | grep -vE '^kubevirtci/' | xargs egrep -Hn " +$" || true)
    if [[ $invalid_files ]]; then
        echo 'Found trailing whitespaces. Please remove trailing whitespaces using `make fmt`:'
        echo "$invalid_files"
        return 1
    fi
}

if [ "$1" == "--fix" ]; then
    fix
else
    check
fi
