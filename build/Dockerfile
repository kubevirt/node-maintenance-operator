FROM registry.access.redhat.com/ubi8/go-toolset:1.15.7 AS builder
WORKDIR /opt/app-root/src
COPY . .

RUN make build

FROM registry.access.redhat.com/ubi8/ubi-minimal
ENV OPERATOR=/usr/local/bin/node-maintenance-operator \
    USER_UID=1001 \
    USER_NAME=node-maintenance-operator

# install operator binary
COPY --from=builder /opt/app-root/src/_out/node-maintenance-operator ${OPERATOR}

# install scripts
COPY --from=builder /opt/app-root/src/build/bin /usr/local/bin
RUN  /usr/local/bin/user_setup

# needed for HCO
LABEL org.kubevirt.hco.csv-generator.v1="/usr/local/bin/csv-generator"
COPY --from=builder /opt/app-root/src/build/hco/csv-generator /usr/local/bin/
COPY --from=builder /opt/app-root/src/manifests/node-maintenance-operator/v9.9.9/manifests /manifests

ENTRYPOINT ["/usr/local/bin/entrypoint"]

USER ${USER_UID}
