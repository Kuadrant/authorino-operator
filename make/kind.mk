
##@ Kind

## Targets to help install and use kind for development https://kind.sigs.k8s.io

KIND_CLUSTER_NAME ?= authorino-local

.PHONY: kind-create-cluster
kind-create-cluster: kind ## Create the "authorino-local" kind cluster.
	KIND_EXPERIMENTAL_PROVIDER=$(CONTAINER_ENGINE) $(KIND) create cluster --name $(KIND_CLUSTER_NAME) --config utils/kind-cluster.yaml

.PHONY: kind-delete-cluster
kind-delete-cluster: kind ## Delete the "authorino-local" kind cluster.
	- KIND_EXPERIMENTAL_PROVIDER=$(CONTAINER_ENGINE) $(KIND) delete cluster --name $(KIND_CLUSTER_NAME)
