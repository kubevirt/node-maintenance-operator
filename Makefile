all: fmt check

OPERATOR_SDK_VERSION = v0.18.2
export OPERATOR_SDK_VERSION
OPM_VERSION = v1.12.7
export OPM_VERSION

# The last released version (without v)
OPERATOR_VERSION_LAST=0.6.0
export OPERATOR_VERSION_LAST
# The version of the next release (without v)
OPERATOR_VERSION_NEXT=0.7.0
export OPERATOR_VERSION_NEXT
# The OLM channel this operator should be default of
OLM_CHANNEL=4.6
export OLM_CHANNEL

IMAGE_REGISTRY ?= quay.io/kubevirt
export IMAGE_REGISTRY
IMAGE_TAG ?= latest
export IMAGE_TAG
OPERATOR_IMAGE ?= node-maintenance-operator
export OPERATOR_IMAGE
BUNDLE_IMAGE ?= node-maintenance-operator-bundle
export BUNDLE_IMAGE
INDEX_IMAGE ?= node-maintenance-operator-index
export INDEX_IMAGE

TARGETCOVERAGE=60

KUBEVIRTCI_PATH=$$(pwd)/kubevirtci/cluster-up
KUBEVIRTCI_CONFIG_PATH=$$(pwd)/_ci-configs

TARGETS = \
	cluster-up \
	gen-k8s \
	gen-k8s-check \
	goimports \
	goimports-check \
	vet \
	setupgithook \
	whitespace \
	whitespace-commit \
	whitespace-check \
	manifests \
	test

GINKGO ?= build/_output/bin/ginkgo

# Make does not offer a recursive wildcard function, so here's one:
rwildcard=$(wildcard $1$2) $(foreach d,$(wildcard $1*),$(call rwildcard,$d/,$2))

# Gather needed source files and directories to create target dependencies
directories := $(filter-out ./ ./vendor/ ,$(sort $(dir $(wildcard ./*/))))
all_sources=$(call rwildcard,$(directories),*) $(filter-out $(TARGETS) ./go.mod ./go.sum, $(wildcard *))
cmd_sources=$(call rwildcard,cmd/,*.go)
pkg_sources=$(call rwildcard,pkg/,*.go)
apis_sources=$(call rwildcard,pkg/apis,*.go)

fmt: whitespace goimports

goimports:
	GO111MODULE=on go run golang.org/x/tools/cmd/goimports -w ./pkg ./cmd

goimports-check: $(cmd_sources) $(pkg_sources)
	GO111MODULE=on go run golang.org/x/tools/cmd/goimports -d ./pkg ./cmd

whitespace: $(all_sources)
	./hack/whitespace.sh --fix

whitespace-commit: $(all_sources)
	./hack/whitespace.sh --fix-commit

check: setupgithook vet goimports-check gen-operator-sdk gen-k8s-check verify-manifests test

whitespace-check: $(all_sources)
	./hack/whitespace.sh

vet: $(cmd_sources) $(pkg_sources)
	go vet -mod=vendor ./pkg/... ./cmd/...

test:
	./hack/coverage.sh $(GINKGO) $(TARGETCOVERAGE)

gen-k8s: $(apis_sources)
	./hack/gen-k8s.sh

gen-k8s-check: $(apis_sources)
	./hack/verify-codegen.sh

gen-crds: $(apis_sources)
	./hack/gen-crds.sh

container-build: container-build-operator container-build-bundle container-build-index

container-build-operator: csv-generator
	docker build -f build/Dockerfile -t $(IMAGE_REGISTRY)/$(OPERATOR_IMAGE):$(IMAGE_TAG) .

container-build-bundle:
	docker build -f build/bundle.Dockerfile -t $(IMAGE_REGISTRY)/$(BUNDLE_IMAGE):$(IMAGE_TAG) .

container-generate-index:
	./hack/gen-index.sh

container-build-index: container-generate-index
	docker build -f build/index.Dockerfile -t $(IMAGE_REGISTRY)/$(INDEX_IMAGE):$(IMAGE_TAG) .

container-push: container-push-operator container-push-bundle container-push-index

container-push-operator:
	docker push $(IMAGE_REGISTRY)/$(OPERATOR_IMAGE):$(IMAGE_TAG)

container-push-bundle:
	docker push $(IMAGE_REGISTRY)/$(BUNDLE_IMAGE):$(IMAGE_TAG)

container-push-index:
	docker push $(IMAGE_REGISTRY)/$(INDEX_IMAGE):$(IMAGE_TAG)

csv-generator: gen-operator-sdk
	./hack/gen-bundle.sh

gen-operator-sdk:
	./hack/gen-operator-sdk.sh

gen-opm:
	./hack/gen-opm.sh

verify-manifests:
	./hack/verify-manifests.sh

manifests: csv-generator
	./hack/release-manifests.sh ${IMAGE_TAG}

cluster-up:
	KUBEVIRT_NUM_NODES=2 $(KUBEVIRTCI_PATH)/up.sh

cluster-down:
	$(KUBEVIRTCI_PATH)/down.sh

pull-ci-changes:
	git subtree pull --prefix kubevirtci https://github.com/kubevirt/kubevirtci.git master --squash

cluster-sync:
	IMAGE_REGISTRY=$(IMAGE_REGISTRY) IMAGE_TAG=$(IMAGE_TAG) ./hack/sync.sh

cluster-functest:
	./hack/functest.sh

cluster-clean:
	./hack/clean.sh

setupgithook:
	./hack/precommit-hook.sh setup
	./hack/commit-msg-hook.sh setup

.PHONY: all check fmt test container-build container-push manifests verify-manifests cluster-up cluster-down cluster-sync cluster-functest cluster-clean pull-ci-changes test-courier setupgithook whitespace-commit
