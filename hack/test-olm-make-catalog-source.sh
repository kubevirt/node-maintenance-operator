#!/bin/bash

cat <<EOF
apiVersion: operators.coreos.com/v1alpha1
kind: CatalogSource
metadata:
  name: nmocatalogsource
  namespace: ${CATALOG_NAMESPACE}
spec:
  sourceType: grpc
  image: ${REG_IMG}
  displayName: KubeVirt HyperConverged
  publisher: Red Hat
EOF

