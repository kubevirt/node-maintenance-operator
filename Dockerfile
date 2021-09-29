# Build the manager binary
FROM quay.io/centos/centos:stream8 AS builder
RUN dnf install golang -y

# Ensure go 1.16
RUN go install golang.org/dl/go1.16@latest
RUN ~/go/bin/go1.16 download
RUN /bin/cp -f ~/go/bin/go1.16 /usr/bin/go
RUN go version

WORKDIR /workspace

COPY go.mod go.mod
COPY go.sum go.sum
COPY main.go main.go
COPY api/ api/
COPY controllers/ controllers/
COPY vendor vendor

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o manager main.go

# Use ubi8 as minimal base image to package the manager binary
FROM registry.access.redhat.com/ubi8/ubi-minimal
WORKDIR /
COPY --from=builder /workspace/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]
