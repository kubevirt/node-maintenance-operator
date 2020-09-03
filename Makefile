export OPERATOR_SDK_VERSION = v0.18.2
export OPM_VERSION = v1.12.7

# The last released version (without v)
export OPERATOR_VERSION_LAST ?= 0.7.0
# The version of the next release (without v)
export OPERATOR_VERSION_NEXT ?= 0.8.0
# The OLM channel this operator should be default of
export OLM_CHANNEL ?= 4.7
export OLM_NS ?= openshift-marketplace
export OPERATOR_NS ?= openshift-node-maintenance-operator

export IMAGE_REGISTRY ?= quay.io/kubevirt
export IMAGE_TAG ?= latest
export OPERATOR_IMAGE ?= node-maintenance-operator
export BUNDLE_IMAGE ?= node-maintenance-operator-bundle
export INDEX_IMAGE ?= node-maintenance-operator-index
export MUST_GATHER_IMAGE ?= lifecycle-must-gather

export TARGETCOVERAGE=60

KUBEVIRTCI_PATH=$$(pwd)/kubevirtci/cluster-up
KUBEVIRTCI_CONFIG_PATH=$$(pwd)/_ci-configs
export KUBEVIRT_NUM_NODES ?= 3

GIT_VERSION=$$(git describe --always --tags)
VERSION=$${CI_UPSTREAM_VERSION:-$(GIT_VERSION)}
GIT_COMMIT=$$(git rev-list -1 HEAD)
COMMIT=$${CI_UPSTREAM_COMMIT:-$(GIT_COMMIT)}
BUILD_DATE=$$(date --utc -Iseconds)

export GINKGO ?= build/_output/bin/ginkgo

# Make does not offer a recursive wildcard function, so here's one:
rwildcard=$(wildcard $1$2) $(foreach d,$(wildcard $1*),$(call rwildcard,$d/,$2))

# Gather needed source files and directories to create target dependencies
directories := $(filter-out ./ ./vendor/ ./kubevirtci/ ,$(sort $(dir $(wildcard ./*/))))
# exclude directories which are also targets
all_sources=$(call rwildcard,$(directories),*) $(filter-out build test manifests ./go.mod ./go.sum, $(wildcard *))
cmd_sources=$(call rwildcard,cmd/,*.go)
pkg_sources=$(call rwildcard,pkg/,*.go)
apis_sources=$(call rwildcard,pkg/apis,*.go)

.PHONY: all
all: check

.PHONY: fmt
fmt: whitespace goimports

.PHONY: goimports
goimports:
	go run golang.org/x/tools/cmd/goimports -w ./pkg ./cmd ./test

.PHONY: whitespace
whitespace: $(all_sources)
	./hack/whitespace.sh

.PHONY: vet
vet: $(cmd_sources) $(pkg_sources)
	go vet -mod=vendor ./pkg/... ./cmd/... ./test/...

.PHONY: verify-unchanged
verify-unchanged:
	./hack/verify-unchanged.sh

.PHONY: test
test:
	./hack/coverage.sh

.PHONY: shfmt
shfmt:
	go get mvdan.cc/sh/v3/cmd/shfmt
	shfmt -i 4 -w ./hack/
	shfmt -i 4 -w ./build/

.PHONY: check
check: shfmt fmt vet generate-all verify-manifests verify-unchanged test

.PHONY: build
build:
	mkdir -p _out; \
	LDFLAGS="-s -w "; \
	LDFLAGS+="-X kubevirt.io/node-maintenance-operator/version.Version=$(VERSION) "; \
	LDFLAGS+="-X kubevirt.io/node-maintenance-operator/version.GitCommit=$(COMMIT) "; \
	LDFLAGS+="-X kubevirt.io/node-maintenance-operator/version.BuildDate=$(BUILD_DATE) "; \
	GOFLAGS=-mod=vendor CGO_ENABLED=0 GOOS=linux go build -ldflags="$$LDFLAGS" -o _out/node-maintenance-operator kubevirt.io/node-maintenance-operator/cmd/manager

.PHONY: container-build
container-build: container-build-operator container-build-bundle container-build-index container-build-must-gather

.PHONY: container-build-operator
container-build-operator: generate-bundle
	docker build -f build/Dockerfile -t $(IMAGE_REGISTRY)/$(OPERATOR_IMAGE):$(IMAGE_TAG) .

.PHONY: container-build-bundle
container-build-bundle:
	docker build -f build/bundle.Dockerfile -t $(IMAGE_REGISTRY)/$(BUNDLE_IMAGE):$(IMAGE_TAG) .

.PHONY: container-build-index
container-build-index:
	docker build --build-arg OPERATOR_VERSION_NEXT=$(OPERATOR_VERSION_NEXT) -f build/index.Dockerfile -t $(IMAGE_REGISTRY)/$(INDEX_IMAGE):$(IMAGE_TAG) .

.PHONY: container-build-must-gather
container-build-must-gather:
	docker build -f must-gather/Dockerfile -t $(IMAGE_REGISTRY)/$(MUST_GATHER_IMAGE):$(IMAGE_TAG) must-gather

.PHONY: container-push
container-push: container-push-operator container-push-bundle container-push-index container-push-must-gather

.PHONY: container-push-operator
container-push-operator:
	docker push $(IMAGE_REGISTRY)/$(OPERATOR_IMAGE):$(IMAGE_TAG)

.PHONY: container-push-bundle
container-push-bundle:
	docker push $(IMAGE_REGISTRY)/$(BUNDLE_IMAGE):$(IMAGE_TAG)

.PHONY: container-push-index
container-push-index:
	docker push $(IMAGE_REGISTRY)/$(INDEX_IMAGE):$(IMAGE_TAG)

.PHONY: container-push-must-gather
container-push-must-gather:
	docker push $(IMAGE_REGISTRY)/$(MUST_GATHER_IMAGE):$(IMAGE_TAG)

.PHONY: get-operator-sdk
get-operator-sdk:
	./hack/get-operator-sdk.sh

.PHONY: get-opm
get-opm:
	./hack/get-opm.sh

.PHONY: generate-k8s
generate-k8s: $(apis_sources)
	./hack/generate-k8s.sh

.PHONY: generate-crds
generate-crds: $(apis_sources)
	./hack/generate-crds.sh

.PHONY: generate-bundle
generate-bundle:
	./hack/generate-bundle.sh

.PHONY: generate-template-bundle
generate-template-bundle:
	OPERATOR_VERSION_NEXT=9.9.9 OLM_CHANNEL=9.9 IMAGE_REGISTRY=IMAGE_REGISTRY OPERATOR_IMAGE=OPERATOR_IMAGE IMAGE_TAG=IMAGE_TAG make generate-bundle

.PHONY: generate-all
generate-all: generate-k8s generate-crds generate-template-bundle generate-bundle

.PHONY: manifests
manifests: generate-bundle
	./hack/release-manifests.sh ${IMAGE_TAG}

.PHONY: verify-manifests
verify-manifests:
	./hack/verify-manifests.sh

.PHONY: cluster-up
cluster-up:
	$(KUBEVIRTCI_PATH)/up.sh

.PHONY: cluster-down
cluster-down:
	$(KUBEVIRTCI_PATH)/down.sh

.PHONY: pull-ci-changes
pull-ci-changes:
	git subtree pull --prefix kubevirtci https://github.com/kubevirt/kubevirtci.git master --squash

.PHONY: cluster-sync-prepare
cluster-sync-prepare:
	./hack/sync-prepare.sh

.PHONY: cluster-sync-deploy
cluster-sync-deploy:
	./hack/sync-deploy.sh

.PHONY: cluster-sync
cluster-sync: cluster-sync-prepare cluster-sync-deploy

.PHONY: cluster-functest
cluster-functest:
	./hack/functest.sh

.PHONY: cluster-clean
cluster-clean:
	./hack/clean.sh

.PHONY: setupgithook
setupgithook:
	./hack/precommit-hook.sh setup
	./hack/commit-msg-hook.sh setup
