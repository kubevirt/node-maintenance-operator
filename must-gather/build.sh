#!/bin/bash

set -ex

pushd must-gather
make docker-build
popd
