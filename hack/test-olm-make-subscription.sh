#!/bin/bash

cat <<EOF
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: node-maintenance-operator
  namespace: ${CATALOG_NAMESPACE}
spec:
  channel: "beta"
  installPlanApproval: Automatic
  name: node-maintenance-operator
  source: nmocatalogsource
  sourceNamespace: ${CATALOG_NAMESPACE}
EOF

