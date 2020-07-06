FROM quay.io/openshift/origin-must-gather:4.6.0 AS builder

FROM registry.access.redhat.com/ubi8/ubi-minimal:latest

COPY --from=builder /usr/bin/oc /usr/bin/oc
COPY --from=builder /usr/bin/sed /usr/bin/sed
COPY --from=builder /usr/bin/awk /usr/bin/awk
COPY --from=builder /usr/bin/grep /usr/bin/grep

# Save original gather script
COPY --from=builder /usr/bin/gather /usr/bin/gather_original

# Copy all collection scripts to /usr/bin
COPY collection-scripts/* /usr/bin/

ENTRYPOINT /usr/bin/gather
