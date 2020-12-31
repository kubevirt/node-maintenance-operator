FROM golang:1.13.8 AS builder
WORKDIR /go/src/kubevirt.io/node-maintenance-operator/
ENV GOPATH=/go
COPY . .

RUN make check-all
