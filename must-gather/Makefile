IMAGE_REGISTRY ?=quay.io/kubevirt
IMAGE_TAG ?= latest
MUST_GATHER_IMAGE ?= must-gather-lifecycle

all:
	docker build -t ${IMAGE_REGISTRY}/${MUST_GATHER_IMAGE}:${IMAGE_TAG} .

.PHONY: docker-build
