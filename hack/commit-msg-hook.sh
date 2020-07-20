#!/bin/bash

TOP_LEVEL=$(git rev-parse --show-toplevel)

if [[ $TOP_LEVEL == "" ]]; then
    echo "Error: this script must be called from a git repository"
    exit 1
fi

if [[ $1 == "setup" ]]; then
    if [[ ! -f $TOP_LEVEL/.git/hooks/commit-msg ]] && [[ ! -L $TOP_LEVEL/.git/hooks/commit-msg ]]; then
        ln -s $TOP_LEVEL/hack/commit-msg-hook.sh $TOP_LEVEL/.git/hooks/commit-msg
    fi
    exit 0
fi

COMMIT_MSG_FILE="$1"

# check if commit message is present
HAS_SIGNOFF=$(grep -c 'Signed-off-by:' "${COMMIT_MSG_FILE}")

if [[ $HAS_SIGNOFF == "0" ]]; then
    exec 1>&2
    cat <<EOF
Commit Signoff is missing from the commit message.
Please create the commit with the following commit

	git commit  --signoff
EOF
    exit 1
fi
