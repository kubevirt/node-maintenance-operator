#!/bin/bash

cat <<EOF
apiVersion: nodemaintenance.kubevirt.io/v1beta1
kind: NodeMaintenance
metadata:
  name: ${NMO_NAME}
spec:
  nodeName: ${NODE_NAME}
EOF

