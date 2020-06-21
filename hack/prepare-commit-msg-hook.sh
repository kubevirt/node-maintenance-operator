#!/bin/bash

TOP_LEVEL=$(git rev-parse --show-toplevel)

if [[ $1 == "setup" ]]; then
	ln -fs  $TOP_LEVEL/hack/prepare-commit-msg-hook.sh $TOP_LEVEL/.git/hooks/prepare-commit-msg
	exit 0
fi

if [[ $TOP_LEVEL == "" ]]; then
  echo "Error: this script must be called from a git repository"
  exit 1
fi

COMMIT_MSG_FILE="$1"
SIGNOFF_MSG="Signed-off-by: "$(git config user.name)" <"$(git config user.email)">" >>${COMMIT_MSG_FILE}
sed -ie "1i ${SIGNOFF_MSG}" "${COMMIT_MSG_FILE}"


