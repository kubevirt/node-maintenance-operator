#!/bin/bash

TOP_LEVEL=$(git rev-parse --show-toplevel)

if [[ $TOP_LEVEL == "" ]]; then
  echo "Error: this script must be called from a git repository"
  exit 1
fi

if [[ $1 == "setup" ]]; then
	if [[ ! -f $TOP_LEVEL/.git/hooks/pre-commit ]] && [[ ! -h $TOP_LEVEL/.git/hooks/pre-commit ]]; then
		ln -s  $TOP_LEVEL/hack/precommit-hook.sh $TOP_LEVEL/.git/hooks/pre-commit
	fi
	exit 0
fi

# Redirect output to stderr.
exec 1>&2

cd $TOP_LEVEL
${TOP_LEVEL}/hack/whitespace.sh --check-precommit
if [[ $? != 0 ]]; then
	echo "Error: whitespace check failed. run make whitespace to fix it (or make whitespace-commit to change just the files added for commit)"
	exit 1
fi
