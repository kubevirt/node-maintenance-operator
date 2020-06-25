IMAGE_REGISTRY ?=quay.io/kubevirt
IMAGE_TAG ?= latest
MUST_GATHER_IMAGE ?= must-gather-lifecycle

docker-build:
	docker build -t ${IMAGE_REGISTRY}/${MUST_GATHER_IMAGE}:${IMAGE_TAG} .

docker-push:
	docker push ${IMAGE_REGISTRY}/${MUST_GATHER_IMAGE}:${IMAGE_TAG}

.PHONY: docker-build docker-push
