FROM golang:1.15 AS builder
WORKDIR /go/src/kubevirt.io/node-maintenance-operator/
ENV GOPATH=/go
COPY . .

RUN make check-all
