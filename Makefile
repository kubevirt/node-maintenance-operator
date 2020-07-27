export OPERATOR_SDK_VERSION = v0.18.2
export OPM_VERSION = v1.12.7

# The last released version (without v)
export OPERATOR_VERSION_LAST ?= 0.6.0
# The version of the next release (without v)
export OPERATOR_VERSION_NEXT ?= 0.7.0
# The OLM channel this operator should be default of
export OLM_CHANNEL ?= 4.6

export IMAGE_REGISTRY ?= quay.io/kubevirt
export IMAGE_TAG ?= latest
export OPERATOR_IMAGE ?= node-maintenance-operator
export BUNDLE_IMAGE ?= node-maintenance-operator-bundle
export INDEX_IMAGE ?= node-maintenance-operator-index

export TARGETCOVERAGE=60

KUBEVIRTCI_PATH=$$(pwd)/kubevirtci/cluster-up
KUBEVIRTCI_CONFIG_PATH=$$(pwd)/_ci-configs

export GINKGO ?= build/_output/bin/ginkgo

# Make does not offer a recursive wildcard function, so here's one:
rwildcard=$(wildcard $1$2) $(foreach d,$(wildcard $1*),$(call rwildcard,$d/,$2))

# Gather needed source files and directories to create target dependencies
directories := $(filter-out ./ ./vendor/ ./kubevirtci/ ,$(sort $(dir $(wildcard ./*/))))
# exclude directories which are also targets
all_sources=$(call rwildcard,$(directories),*) $(filter-out test manifests ./go.mod ./go.sum, $(wildcard *))
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

.PHONY: check
check: shfmt fmt vet generate-all verify-manifests verify-unchanged test

.PHONY: container-build
container-build: container-build-operator container-build-bundle container-build-index

.PHONY: container-build-operator
container-build-operator: generate-bundle
	docker build -f build/Dockerfile -t $(IMAGE_REGISTRY)/$(OPERATOR_IMAGE):$(IMAGE_TAG) .

.PHONY: container-build-bundle
container-build-bundle:
	docker build -f build/bundle.Dockerfile -t $(IMAGE_REGISTRY)/$(BUNDLE_IMAGE):$(IMAGE_TAG) .

.PHONY: container-generate-index
container-generate-index:
	./hack/generate-index.sh

.PHONY: container-build-index
container-build-index: container-generate-index
	docker build -f build/index.Dockerfile -t $(IMAGE_REGISTRY)/$(INDEX_IMAGE):$(IMAGE_TAG) .

.PHONY: container-push
container-push: container-push-operator container-push-bundle container-push-index

.PHONY: container-push-operator
container-push-operator:
	docker push $(IMAGE_REGISTRY)/$(OPERATOR_IMAGE):$(IMAGE_TAG)

.PHONY: container-push-bundle
container-push-bundle:
	docker push $(IMAGE_REGISTRY)/$(BUNDLE_IMAGE):$(IMAGE_TAG)

.PHONY: container-push-index
container-push-index:
	docker push $(IMAGE_REGISTRY)/$(INDEX_IMAGE):$(IMAGE_TAG)

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

.PHONY: generate-all
generate-all: generate-k8s generate-crds generate-bundle

.PHONY: manifests
manifests: generate-bundle
	./hack/release-manifests.sh ${IMAGE_TAG}

.PHONY: verify-manifests
verify-manifests:
	./hack/verify-manifests.sh

.PHONY: cluster-up
cluster-up:
	KUBEVIRT_NUM_NODES=2 $(KUBEVIRTCI_PATH)/up.sh

.PHONY: cluster-down
cluster-down:
	$(KUBEVIRTCI_PATH)/down.sh

.PHONY: pull-ci-changes
pull-ci-changes:
	git subtree pull --prefix kubevirtci https://github.com/kubevirt/kubevirtci.git master --squash

.PHONY: cluster-sync
cluster-sync:
	IMAGE_REGISTRY=$(IMAGE_REGISTRY) IMAGE_TAG=$(IMAGE_TAG) ./hack/sync.sh

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
