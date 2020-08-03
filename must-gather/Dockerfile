FROM quay.io/openshift/origin-must-gather:4.6.0 AS builder

FROM registry.access.redhat.com/ubi8/ubi-minimal
RUN microdnf install tar rsync

# Copy must-gather required binaries
COPY --from=builder /usr/bin/oc /usr/bin/oc

# Save original gather script
COPY --from=builder /usr/bin/gather* /usr/bin/
RUN mv /usr/bin/gather /usr/bin/gather_original

# Copy our scripts
COPY collection-scripts/* /usr/bin/

ENTRYPOINT /usr/bin/gather
