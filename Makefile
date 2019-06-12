OPERATOR_SDK_VERSION ?= v0.8.0
IMAGE_REGISTRY ?= quay.io/kubevirt
IMAGE_TAG ?= latest
OPERATOR_IMAGE ?= node-maintenance-operator
REGISTRY_IMAGE ?= node-maintenance-operator-registry

all: vet fmt
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

operator-sdk:
	curl -JL https://github.com/operator-framework/operator-sdk/releases/download/$(OPERATOR_SDK_VERSION)/operator-sdk-$(OPERATOR_SDK_VERSION)-x86_64-linux-gnu -o operator-sdk
	chmod 0755 operator-sdk

manifests: operator-sdk
	./build/make-manifests.sh ${IMAGE_TAG}
	./hack/release-manifests.sh ${IMAGE_TAG}

cluster-up:
	CLUSTER_NUM_NODES=3 ./cluster/up.sh

cluster-down:
	./cluster/down.sh

cluster-sync:
	./cluster/sync.sh	

cluster-functest:
	./cluster/functest.sh

cluster-clean:
	./cluster/clean.sh		

.PHONY: vet fmt container-build container-push manifests cluster-up cluster-down cluster-sync cluster-functest cluster-clean all
