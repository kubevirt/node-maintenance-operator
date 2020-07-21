#!/usr/bin/env bash

set -e

function fix_one_file() {
    local fname="$1"
    sed --follow-symlinks 's/[[:space:]]*$//' ${fname} | sed 'N;/^\n$/D;P;D;' >${fname}.tmp
    cp -f ${fname}.tmp ${fname}
    rm -f ${fname}.tmp
}

function check_files() {
    while read fname; do
        IDEAL_FILE=$(sed --follow-symlinks 's/[[:space:]]*$//' ${fname} | sed 'N;/^\n$/D;P;D;')
        REAL_FILE=$(cat ${fname})

        if [[ "$IDEAL_FILE" != "$REAL_FILE" ]]; then
            echo "need whitespace fix for file $fname"
            diff -u <(echo "$REAL_FILE") <(echo "$IDEAL_FILE")
        fi
    done
}

export -f fix_one_file

function fix() {
    git ls-files -- ':!vendor/' | grep -vE '^kubevirtci/' | xargs -I {} bash -c 'fix_one_file "{}"'
}

fix
