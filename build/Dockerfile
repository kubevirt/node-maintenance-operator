FROM golang:1.13.8 AS builder
WORKDIR /go/src/kubevirt.io/node-maintenance-operator/
ENV GOPATH=/go
COPY . .

RUN mkdir -p _out && GOFLAGS=-mod=vendor CGO_ENABLED=0 GOOS=linux go build -o _out/node-maintenance-operator kubevirt.io/node-maintenance-operator/cmd/manager

FROM registry.access.redhat.com/ubi8/ubi-minimal
ENV OPERATOR=/usr/local/bin/node-maintenance-operator \
    USER_UID=1001 \
    USER_NAME=node-maintenance-operator

# install operator binary
COPY --from=builder /go/src/kubevirt.io/node-maintenance-operator/_out/node-maintenance-operator ${OPERATOR}

# install scripts
COPY --from=builder /go/src/kubevirt.io/node-maintenance-operator/build/bin /usr/local/bin
RUN  /usr/local/bin/user_setup

ENTRYPOINT ["/usr/local/bin/entrypoint"]

USER ${USER_UID}
