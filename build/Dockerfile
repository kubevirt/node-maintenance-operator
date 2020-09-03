FROM golang:1.13.8 AS builder
WORKDIR /go/src/kubevirt.io/node-maintenance-operator/
ENV GOPATH=/go
COPY . .

RUN make build

FROM registry.access.redhat.com/ubi8/ubi-minimal
ENV OPERATOR=/usr/local/bin/node-maintenance-operator \
    USER_UID=1001 \
    USER_NAME=node-maintenance-operator

# install operator binary
COPY --from=builder /go/src/kubevirt.io/node-maintenance-operator/_out/node-maintenance-operator ${OPERATOR}

# install scripts
COPY --from=builder /go/src/kubevirt.io/node-maintenance-operator/build/bin /usr/local/bin
RUN  /usr/local/bin/user_setup

# needed for HCO
LABEL org.kubevirt.hco.csv-generator.v1="/usr/local/bin/csv-generator"
COPY --from=builder /go/src/kubevirt.io/node-maintenance-operator/build/hco/csv-generator /usr/local/bin/
COPY --from=builder /go/src/kubevirt.io/node-maintenance-operator/manifests/node-maintenance-operator/v9.9.9/manifests /manifests

ENTRYPOINT ["/usr/local/bin/entrypoint"]

USER ${USER_UID}
