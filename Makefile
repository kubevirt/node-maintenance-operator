IMAGE_REGISTRY ?= quay.io/kubevirt
IMAGE_TAG ?= v0.3.0
CSV_VERSION := $(shell echo ${IMAGE_TAG} | cut -c 2- )
OPERATOR_IMAGE ?= node-maintenance-operator
REGISTRY_IMAGE ?= node-maintenance-operator-registry

all: vet fmt container-build container-push
vet:
	go vet ./pkg/... ./cmd/...

fmt:
	go fmt ./pkg/... ./cmd/...

container-build: container-build-operator container-build-registry

container-build-operator:
	docker build -f build/Dockerfile -t $(IMAGE_REGISTRY)/$(OPERATOR_IMAGE):$(IMAGE_TAG) .

container-build-registry:
	docker build -f build/Dockerfile.registry -t $(IMAGE_REGISTRY)/$(REGISTRY_IMAGE):$(IMAGE_TAG) .

container-push: container-push-operator container-push-registry

container-push-operator:
	docker push $(IMAGE_REGISTRY)/$(OPERATOR_IMAGE):$(IMAGE_TAG)

container-push-registry:
	docker push $(IMAGE_REGISTRY)/$(REGISTRY_IMAGE):$(IMAGE_TAG)

manifests:	
	CSV_VERSION=$(CSV_VERSION) \
		./hack/release-manifests.sh

.PHONY: vet fmt container-build container-push manifests all
