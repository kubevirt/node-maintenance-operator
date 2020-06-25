FROM quay.io/openshift/origin-must-gather:4.3.0 as builder

FROM centos:7

# For gathering data from nodes
RUN yum update -y && yum install iproute tcpdump pciutils util-linux nftables rsync -y && yum clean all

COPY --from=builder /usr/bin/oc /usr/bin/oc
COPY --from=builder /usr/bin/sed /usr/bin/sed
COPY --from=builder /usr/bin/awk /usr/bin/awk
COPY --from=builder /usr/bin/grep /usr/bin/grep

# Save original gather script
COPY --from=builder /usr/bin/gather /usr/bin/gather_original

# Copy all collection scripts to /usr/bin
COPY collection-scripts/* /usr/bin/

# Copy node-gather resources to /etc
COPY node-gather/node-gather-crd.yaml /etc/
COPY node-gather/node-gather-ds.yaml /etc/

ENTRYPOINT /usr/bin/gather
