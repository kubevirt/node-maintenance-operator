FROM golang:1.13.8 AS builder
WORKDIR /go/src/kubevirt.io/node-maintenance-operator/
ENV GOPATH=/go
COPY . .

RUN GOFLAGS=-mod=vendor CGO_ENABLED=0 GOOS=linux go build -o /node-maintenance-operator kubevirt.io/node-maintenance-operator/cmd/manager

FROM registry.access.redhat.com/ubi8/ubi-minimal
ENV ENTRYPOINT=/entrypoint \
    OPERATOR=/node-maintenance-operator \
    USER_UID=1001 \
    USER_NAME=node-maintenance-operator

LABEL org.kubevirt.hco.csv-generator.v1="/usr/bin/csv-generator"

COPY --from=builder /go/src/kubevirt.io/node-maintenance-operator/build/bin/user_setup /user_setup
RUN /user_setup

COPY deploy/crds /deploy/crds
COPY manifests/generated /manifests/generated
COPY build/csv-generator.sh /usr/bin/csv-generator

COPY --from=builder /node-maintenance-operator ${OPERATOR}
COPY --from=builder /go/src/kubevirt.io/node-maintenance-operator/build/bin/entrypoint ${ENTRYPOINT}
ENTRYPOINT ${ENTRYPOINT}
USER ${USER_UID}
