FROM golang:1.10 AS builder
WORKDIR /go/src/kubevirt.io/node-maintenance-operator/
ENV GOPATH=/go
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /node-maintenance-operator kubevirt.io/node-maintenance-operator/cmd/manager

FROM registry.access.redhat.com/ubi8/ubi-minimal
ENV ENTRYPOINT=/entrypoint \
    OPERATOR=/node-maintenance-operator \
    USER_UID=1001 \
    USER_NAME=node-maintenance-operator
COPY --from=builder /go/src/kubevirt.io/node-maintenance-operator/build/bin/user_setup /user_setup
RUN /user_setup
COPY --from=builder /node-maintenance-operator ${OPERATOR}
COPY --from=builder /go/src/kubevirt.io/node-maintenance-operator/build/bin/entrypoint ${ENTRYPOINT}
ENTRYPOINT ${ENTRYPOINT}
USER ${USER_UID}