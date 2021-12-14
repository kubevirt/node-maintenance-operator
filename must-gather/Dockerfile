FROM quay.io/openshift/origin-must-gather:4.10.0 AS builder

FROM registry.access.redhat.com/ubi8/ubi-minimal
RUN microdnf install tar rsync

# Copy must-gather required binaries
COPY --from=builder /usr/bin/oc /usr/bin/oc

# Copy our scripts
COPY collection-scripts/* /usr/bin/

ENTRYPOINT /usr/bin/gather
