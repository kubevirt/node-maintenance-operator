export OPERATOR_SDK_VERSION = v1.1.0
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

# Current Operator version
VERSION ?= 0.7.0
# Default bundle image tag
BUNDLE_IMG ?= node-maintenance-operator-bundle:$(VERSION)
# Options for 'bundle-build'
ifneq ($(origin CHANNELS), undefined)
BUNDLE_CHANNELS := --channels=$(CHANNELS)
endif
ifneq ($(origin DEFAULT_CHANNEL), undefined)
BUNDLE_DEFAULT_CHANNEL := --default-channel=$(DEFAULT_CHANNEL)
endif
BUNDLE_METADATA_OPTS ?= $(BUNDLE_CHANNELS) $(BUNDLE_DEFAULT_CHANNEL)

# Image URL to use all building/pushing image targets
IMG ?= node-maintenance-operator:latest
# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS ?= "crd:trivialVersions=true"

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

all: manager

# Run tests
.PHONY: test
test:
	./hack/coverage.sh
# ENVTEST_ASSETS_DIR = $(shell pwd)/testbin
# test: generate fmt vet manifests
# 	mkdir -p $(ENVTEST_ASSETS_DIR)
# 	test -f $(ENVTEST_ASSETS_DIR)/setup-envtest.sh || curl -sSLo $(ENVTEST_ASSETS_DIR)/setup-envtest.sh https://raw.githubusercontent.com/kubernetes-sigs/controller-runtime/v0.6.3/hack/setup-envtest.sh
# 	source $(ENVTEST_ASSETS_DIR)/setup-envtest.sh; fetch_envtest_tools $(ENVTEST_ASSETS_DIR); setup_envtest_env $(ENVTEST_ASSETS_DIR); go test ./... -coverprofile cover.out

# Build manager binary
manager: generate fmt vet
	go build -o bin/manager main.go

# Run against the configured Kubernetes cluster in ~/.kube/config
run: generate fmt vet manifests
	go run ./main.go

# Install CRDs into a cluster
install: manifests kustomize
	$(KUSTOMIZE) build config/crd | kubectl apply -f -

# Uninstall CRDs from a cluster
uninstall: manifests kustomize
	$(KUSTOMIZE) build config/crd | kubectl delete -f -

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
deploy: manifests kustomize
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default | kubectl apply -f -

# Generate manifests e.g. CRD, RBAC etc.
manifests: controller-gen
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role webhook paths="./..." output:crd:artifacts:config=config/crd/bases

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

.PHONY: shfmt
shfmt:
	go get mvdan.cc/sh/v3/cmd/shfmt
	shfmt -i 4 -w ./hack/
	shfmt -i 4 -w ./build/

.PHONY: check
check: shfmt fmt vet generate-all verify-manifests verify-unchanged test

# Generate code
generate: controller-gen
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

# Build the docker image
# docker-build: test
# 	docker build . -t ${IMG}

.PHONY: build
build:
	./hack/build.sh

.PHONY: container-build
container-build: container-build-operator container-build-bundle container-build-index container-build-must-gather

PHONY: container-build-operator
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

.PHONY: release-manifests
release-manifests: generate-bundle
	./hack/release-manifests.sh

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

# Push the docker image
docker-push:
	docker push ${IMG}

# find or download controller-gen
# download controller-gen if necessary
controller-gen:
ifeq (, $(shell which controller-gen))
	@{ \
	set -e ;\
	CONTROLLER_GEN_TMP_DIR=$$(mktemp -d) ;\
	cd $$CONTROLLER_GEN_TMP_DIR ;\
	go mod init tmp ;\
	go get sigs.k8s.io/controller-tools/cmd/controller-gen@v0.3.0 ;\
	rm -rf $$CONTROLLER_GEN_TMP_DIR ;\
	}
CONTROLLER_GEN=$(GOBIN)/controller-gen
else
CONTROLLER_GEN=$(shell which controller-gen)
endif

kustomize:
ifeq (, $(shell which kustomize))
	@{ \
	set -e ;\
	KUSTOMIZE_GEN_TMP_DIR=$$(mktemp -d) ;\
	cd $$KUSTOMIZE_GEN_TMP_DIR ;\
	go mod init tmp ;\
	go get sigs.k8s.io/kustomize/kustomize/v3@v3.5.4 ;\
	rm -rf $$KUSTOMIZE_GEN_TMP_DIR ;\
	}
KUSTOMIZE=$(GOBIN)/kustomize
else
KUSTOMIZE=$(shell which kustomize)
endif

# Generate bundle manifests and metadata, then validate generated files.
.PHONY: bundle
bundle: manifests
	operator-sdk generate kustomize manifests -q
	cd config/manager && $(KUSTOMIZE) edit set image controller=$(IMG)
	$(KUSTOMIZE) build config/manifests | operator-sdk generate bundle -q --overwrite --version $(VERSION) $(BUNDLE_METADATA_OPTS)
	operator-sdk bundle validate ./bundle

# Build the bundle image.
.PHONY: bundle-build
bundle-build:
	docker build -f bundle.Dockerfile -t $(BUNDLE_IMG) .
