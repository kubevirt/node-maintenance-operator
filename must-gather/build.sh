#!/bin/bash

set -eax

echo "imag tag: ${IMAGE_TAG}"

pushd must-gather
make IMAGE_TAG=${IMAGE_TAG}
popd
