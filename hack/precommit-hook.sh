#!/bin/bash

TOP_LEVEL=$(git rev-parse --show-toplevel)

if [[ $1 == "setup" ]]; then
	ln -fs  $TOP_LEVEL/hack/precommit-hook.sh $TOP_LEVEL/.git/hooks/pre-commit
	exit 0
fi

if [[ $TOP_LEVEL == "" ]]; then
  echo "Error: this script must be called from a git repository"
  exit 1
fi

# Redirect output to stderr.
exec 1>&2

cd $TOP_LEVEL
${TOP_LEVEL}/hack/whitespace.sh --check-precommit
if [[ $? != 0 ]]; then
	echo "Error: whitespace check failed. run make whitespace to fix it (or make whitespace-commit to change just the files added for commit)"
	exit 1
fi
