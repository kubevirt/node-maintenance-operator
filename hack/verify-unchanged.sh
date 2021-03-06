#!/bin/bash

if [[ -n "$(git status --porcelain .)" ]]; then
    echo "Uncommitted generated files. Run 'make check' and commit results."
    echo "$(git status --porcelain .)"
    exit 1
fi
