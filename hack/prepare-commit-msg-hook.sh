#!/bin/bash

TOP_LEVEL=$(git rev-parse --show-toplevel)

if [[ $TOP_LEVEL == "" ]]; then
  echo "Error: this script must be called from a git repository"
  exit 1
fi

if [[ $1 == "setup" ]]; then
	if [[ ! -f $TOP_LEVEL/.git/hooks/prepare-commit-msg ]] && [[ ! -h $TOP_LEVEL/.git/hooks/prepare-commit-msg ]]; then
		ln -s  $TOP_LEVEL/hack/prepare-commit-msg-hook.sh $TOP_LEVEL/.git/hooks/prepare-commit-msg
	fi
	exit 0
fi

COMMIT_MSG_FILE="$1"

# check if commit message is empty
HAS_SIGNOFF=$(grep -c 'Signed-off-by:' "${COMMIT_MSG_FILE}")

if [[ $HAS_SIGNOFF == "0" ]]; then
	UNAME=$(git config user.name)
	UMAIL=$(git config user.email)

	if [[ $UNAME != "" ]] && [[ $UMAIL != "" ]]; then

		COMMIT_COMMENTS=$(sed -n 's/\(^#.*$\)/\1/p' "${COMMIT_MSG_FILE}")

		SIGNOFF_MSG="Signed-off-by: ${UNAME} <${UMAIL}>"

		# strip comments
		sed -i '/^#.*$/d' "${COMMIT_MSG_FILE}"

		# add signoff
		echo -e "\n${SIGNOFF_MSG}\n" >>${COMMIT_MSG_FILE}

		# add comments back
		echo "${COMMIT_COMMENTS}"  >>${COMMIT_MSG_FILE}
	fi
fi

