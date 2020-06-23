#!/bin/bash

	cat <<EOF
annotations:
  operators.operatorframework.io.bundle.mediatype.v1: "registry+v1"
  operators.operatorframework.io.bundle.manifests.v1: "manifests/"
  operators.operatorframework.io.bundle.metadata.v1: "metadata/"
  operators.operatorframework.io.bundle.package.v1: "node-maintenance-operator"
  operators.operatorframework.io.bundle.channels.v1: "beta"
  operators.operatorframework.io.bundle.channel.default.v1: "beta"
EOF

