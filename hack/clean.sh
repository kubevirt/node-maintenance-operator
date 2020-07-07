#!/bin/bash

echo 'Cleaning up ...'

if [ -z $(ls _out/*) ]; then
  echo "nothing to do";
  exit
fi

./kubevirtci/cluster-up/kubectl.sh delete --ignore-not-found -f _out/
