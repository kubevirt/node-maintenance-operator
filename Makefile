all: fmt check

OPERATOR_SDK_VERSION = v0.17.0
IMAGE_REGISTRY ?= quay.io/kubevirt
IMAGE_TAG ?= latest
CURRENT_IMAGE_TAG=v0.6.0
OPERATOR_IMAGE ?= node-maintenance-operator
REGISTRY_IMAGE ?= node-maintenance-operator-registry
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
	./hack/gen-k8s.sh generate k8s

gen-k8s-check: $(apis_sources)
	./hack/verify-codegen.sh

container-build: container-build-operator container-build-registry

container-build-operator: csv-generator
	docker build -f build/Dockerfile -t $(IMAGE_REGISTRY)/$(OPERATOR_IMAGE):$(IMAGE_TAG) .

container-build-registry:
	docker build -f build/Dockerfile.registry -t $(IMAGE_REGISTRY)/$(REGISTRY_IMAGE):$(IMAGE_TAG) .

test-courier:
	echo "bundle dir: $(BUNDLE_DIR_VERSION)"
	export BUNDLE_DIR_VERSION; docker build -f build/Dockerfile.test-courier -t test-courier . --build-arg dir=$(BUNDLE_DIR_VERSION)

container-push: container-push-operator container-push-registry

container-push-operator:
	docker push $(IMAGE_REGISTRY)/$(OPERATOR_IMAGE):$(IMAGE_TAG)

container-push-registry:
	docker push $(IMAGE_REGISTRY)/$(REGISTRY_IMAGE):$(IMAGE_TAG)

csv-generator: gen-operator-sdk
	./build/make-csv-generator.sh

gen-operator-sdk:
	./hack/gen-operator-sdk.sh ${OPERATOR_SDK_VERSION}

verify-manifests: csv-generator
	./build/verify-manifests.sh ${CURRENT_IMAGE_TAG}

manifests: csv-generator
	./build/make-manifests.sh ${IMAGE_TAG}
	./hack/release-manifests.sh ${IMAGE_TAG}

cluster-up:
	KUBEVIRT_NUM_NODES=3 $(KUBEVIRTCI_PATH)/up.sh

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

olmtest:
	./hack/test-olm.sh

olmtestcleanup:
	./hack/test-olm-cleanup.sh kind rm

.PHONY: all check fmt test container-build container-push manifests verify-manifests cluster-up cluster-down cluster-sync cluster-functest cluster-clean pull-ci-changes test-courier setupgithook whitespace-commit olmtest olmtestcleanup
