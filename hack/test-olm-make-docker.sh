#!/bin/bash

	cat >docker.tmp <<EOF
FROM scratch

# We are pushing an operator-registry bundle
# that has both metadata and manifests.
LABEL operators.operatorframework.io.bundle.mediatype.v1=registry+v1
LABEL operators.operatorframework.io.bundle.manifests.v1=manifests/
LABEL operators.operatorframework.io.bundle.metadata.v1=metadata/
LABEL operators.operatorframework.io.bundle.package.v1=node-maintenance-operator
LABEL operators.operatorframework.io.bundle.channels.v1=beta
LABEL operators.operatorframework.io.bundle.channel.default.v1=beta

COPY ./tmp-manifest/node-maintenance-operator/manifests  /manifests/
ADD ./tmp-manifest/node-maintenance-operator/metadata/annotations.yaml /metadata/annotations.yaml

EOF

