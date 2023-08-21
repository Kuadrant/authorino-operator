# Use bash as shell
SHELL = /bin/bash

# VERSION defines the project version for the bundle.
VERSION ?= $(shell git rev-parse HEAD)

# Address of the container registry
DEFAULT_REGISTRY = quay.io
REGISTRY ?= $(DEFAULT_REGISTRY)

# Organization in the container resgistry
DEFAULT_ORG = kuadrant
ORG ?= $(DEFAULT_ORG)

# Repo in the container registry
DEFAULT_REPO = authorino-operator
REPO ?= $(DEFAULT_REPO)

DEFAULT_IMAGE_TAG = latest
IMAGE_TAG_BASE ?= $(REGISTRY)/$(ORG)/$(REPO)

using_semantic_version := $(shell [[ $(VERSION) =~ ^[0-9]+\.[0-9]+\.[0-9]+(-.+)?$$ ]] && echo "true")
ifdef using_semantic_version
BUNDLE_VERSION=$(VERSION)
IMAGE_TAG=v$(VERSION)
else
BUNDLE_VERSION=0.0.0
IMAGE_TAG=$(DEFAULT_IMAGE_TAG)
endif

# Image URL to use all building/pushing image targets
DEFAULT_OPERATOR_IMAGE = $(DEFAULT_REGISTRY)/$(DEFAULT_ORG)/$(DEFAULT_REPO):$(DEFAULT_IMAGE_TAG)
OPERATOR_IMAGE ?= $(IMAGE_TAG_BASE):$(IMAGE_TAG)

# BUNDLE_IMG defines the image:tag used for the bundle.
# You can use it as an arg. (E.g make bundle-build BUNDLE_IMG=<some-registry>/<project-name-bundle>:<tag>)
BUNDLE_IMG ?= $(IMAGE_TAG_BASE)-bundle:$(IMAGE_TAG)

# CHANNELS define the bundle channels used in the bundle.
# Add a new line here if you would like to change its default config. (E.g CHANNELS = "candidate,fast,stable")
ifneq ($(origin CHANNELS), undefined)
BUNDLE_CHANNELS := --channels=$(CHANNELS)
endif

# DEFAULT_CHANNEL defines the default channel used in the bundle.
# Add a new line here if you would like to change its default config. (E.g DEFAULT_CHANNEL = "stable")
ifneq ($(origin DEFAULT_CHANNEL), undefined)
BUNDLE_DEFAULT_CHANNEL := --default-channel=$(DEFAULT_CHANNEL)
endif
BUNDLE_METADATA_OPTS ?= $(BUNDLE_CHANNELS) $(BUNDLE_DEFAULT_CHANNEL)

# Operator manifests (RBAC & CRD)
OPERATOR_MANIFESTS ?= $(PROJECT_DIR)/config/install/manifests.yaml

# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS ?= "crd:trivialVersions=true,preserveUnknownFields=false"

# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.21

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# Setting SHELL to bash allows bash commands to be executed by recipes.
# This is a requirement for 'setup-envtest.sh' in the test target.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

AUTHORINO_VERSION ?= latest
ifeq (latest,$(AUTHORINO_VERSION))
AUTHORINO_BRANCH = main
else
AUTHORINO_BRANCH = v$(AUTHORINO_VERSION)
endif

AUTHORINO_IMAGE_FILE ?= authorino_image
DEFAULT_AUTHORINO_IMAGE ?= $(shell cat $(AUTHORINO_IMAGE_FILE) || echo $(DEFAULT_REGISTRY)/$(DEFAULT_ORG)/authorino:latest)

all: build

##@ General

help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)


##@ Tools

OPERATOR_SDK = $(shell pwd)/bin/operator-sdk
OPERATOR_SDK_VERSION = v1.22.0
operator-sdk: ## Download operator-sdk locally if necessary.
	./utils/install-operator-sdk.sh $(OPERATOR_SDK) $(OPERATOR_SDK_VERSION)

CONTROLLER_GEN = $(shell pwd)/bin/controller-gen
controller-gen: ## Download controller-gen locally if necessary.
	$(call go-get-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen@v0.6.1)

KUSTOMIZE = $(shell pwd)/bin/kustomize
kustomize: ## Download kustomize locally if necessary.
	$(call go-get-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v4@v4.5.5)

KIND = $(shell pwd)/bin/kind
kind:
	$(call go-get-tool,$(KIND),sigs.k8s.io/kind@v0.20.0)

YQ = $(shell pwd)/bin/yq
YQ_VERSION := v4.34.2
$(YQ):
	$(call go-get-tool,$(YQ),github.com/mikefarah/yq/v4@$(YQ_VERSION))

.PHONY: yq
yq: $(YQ) ## Download yq locally if necessary.

.PHONY: opm
OPM = ./bin/opm
opm: ## Download opm locally if necessary.
ifeq (,$(wildcard $(OPM)))
ifeq (,$(shell which opm 2>/dev/null))
	@{ \
	set -e ;\
	mkdir -p $(dir $(OPM)) ;\
	OS=$(shell go env GOOS) && ARCH=$(shell go env GOARCH) && \
	curl -sSLo $(OPM) https://github.com/operator-framework/operator-registry/releases/download/v1.15.1/$${OS}-$${ARCH}-opm ;\
	chmod +x $(OPM) ;\
	}
else
OPM = $(shell which opm)
endif
endif

##@ Development

manifests: controller-gen kustomize authorino-manifests ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=authorino-operator-manager webhook paths="./..." output:crd:artifacts:config=config/crd/bases && $(KUSTOMIZE) build config/install > $(OPERATOR_MANIFESTS)
	$(MAKE) deploy-manifest OPERATOR_IMAGE=$(OPERATOR_IMAGE)

.PHONY: authorino-manifests
authorino-manifests: export AUTHORINO_GITREF := $(AUTHORINO_BRANCH)
authorino-manifests: ## Update authorino manifests.
	envsubst \
        < config/authorino/kustomization.template.yaml \
        > config/authorino/kustomization.yaml

generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

fmt: ## Run go fmt against code.
	go fmt ./...

vet: ## Run go vet against code.
	go vet ./...


setup-envtest:
ifeq (, $(shell which setup-envtest))
	go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest
SETUP_ENVTEST=$(GOBIN)/setup-envtest
else
SETUP_ENVTEST=$(shell which setup-envtest)
endif

# Run the tests
test: manifests generate fmt vet setup-envtest
	echo $(SETUP_ENVTEST)
	KUBEBUILDER_ASSETS='$(strip $(shell $(SETUP_ENVTEST) --arch=amd64 use -p path 1.22.x))'  go test -ldflags="-X github.com/kuadrant/authorino-operator/controllers.DefaultAuthorinoImage=$(DEFAULT_AUTHORINO_IMAGE)" ./... -coverprofile cover.out

##@ Build

build: generate fmt vet ## Build manager binary.
	go build -ldflags "-X main.version=$(VERSION) -X github.com/kuadrant/authorino-operator/controllers.DefaultAuthorinoImage=$(DEFAULT_AUTHORINO_IMAGE)" -o bin/manager main.go

run: manifests generate fmt vet ## Run a controller from your host.
	go run -ldflags "-X main.version=$(VERSION) -X github.com/kuadrant/authorino-operator/controllers.DefaultAuthorinoImage=$(DEFAULT_AUTHORINO_IMAGE)" ./main.go

docker-build:  ## Build docker image with the manager.
	docker build --build-arg VERSION=$(VERSION) --build-arg DEFAULT_AUTHORINO_IMAGE=$(DEFAULT_AUTHORINO_IMAGE) -t $(OPERATOR_IMAGE) .

docker-push: ## Push docker image with the manager.
	docker push ${OPERATOR_IMAGE}

##@ Deployment

install: manifests kustomize install-authorino ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	@if [ $(NAMESPACE) != '' ];then \
		echo "Setting Custom Namespace: $(NAMESPACE)"; \
		cd $(PROJECT_DIR)/config/install && $(KUSTOMIZE) edit set namespace $(NAMESPACE); \
		kubectl create namespace $(NAMESPACE); \
		else \
		kubectl create namespace $(DEFAULT_REPO); \
	fi
	cd $(PROJECT_DIR) && $(KUSTOMIZE) build config/install > $(OPERATOR_MANIFESTS)
	kubectl apply -f $(OPERATOR_MANIFESTS)

	# clean up
	@if [ $(NAMESPACE) != '' ];then \
		echo "Removing Custom Namespace: $(NAMESPACE)"; \
		cd $(PROJECT_DIR)/config/install && $(KUSTOMIZE) edit set namespace $(DEFAULT_REPO); \
	fi


uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config.
	kubectl delete -f $(OPERATOR_MANIFESTS)

deploy: manifests kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd config/manager && $(KUSTOMIZE) edit set image controller=${OPERATOR_IMAGE}
	@if [ $(NAMESPACE) != '' ];then \
		echo "Setting Custom Namespace: $(NAMESPACE)"; \
		cd $(PROJECT_DIR)/config/default && $(KUSTOMIZE) edit set namespace $(NAMESPACE); \
	fi

	$(KUSTOMIZE) build config/default | kubectl apply -f -
	# rollback kustomize edit
	cd config/manager && $(KUSTOMIZE) edit set image controller=${DEFAULT_OPERATOR_IMAGE}
	@if [ $(NAMESPACE) != '' ];then \
		echo "Removing Custom Namespace: $(NAMESPACE)"; \
		cd $(PROJECT_DIR)/config/default && $(KUSTOMIZE) edit set namespace $(DEFAULT_REPO); \
	fi

undeploy: ## Undeploy controller from the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/default | kubectl delete -f -

install-authorino: ## install RBAC and CRD for authorino
	$(KUSTOMIZE) build config/authorino | kubectl apply -f -

# go-get-tool will 'go install' any package $2 and install it to $1.
PROJECT_DIR := $(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))
define go-get-tool
@[ -f $(1) ] || { \
set -e ;\
TMP_DIR=$$(mktemp -d) ;\
cd $$TMP_DIR ;\
go mod init tmp ;\
echo "Downloading $(2)" ;\
GOBIN=$(PROJECT_DIR)/bin go install $(2) ;\
rm -rf $$TMP_DIR ;\
}
endef

DEPLOYMENT_DIR = $(PROJECT_DIR)/config/deploy
DEPLOYMENT_FILE = $(DEPLOYMENT_DIR)/manifests.yaml
.PHONY: deploy-manifest
deploy-manifest: kustomize
	mkdir -p $(DEPLOYMENT_DIR)
	cd $(PROJECT_DIR)/config/manager && $(KUSTOMIZE) edit set image controller=$(OPERATOR_IMAGE)

	@if [ $(NAMESPACE) != '' ];then \
		echo "Setting Custom Namespace: $(NAMESPACE)"; \
		cd $(PROJECT_DIR)/config/deploy && $(KUSTOMIZE) edit set namespace $(NAMESPACE); \
	fi

	cd $(PROJECT_DIR) && $(KUSTOMIZE) build config/deploy > $(DEPLOYMENT_FILE)
	# clean up
	cd $(PROJECT_DIR)/config/manager && $(KUSTOMIZE) edit set image controller=${DEFAULT_OPERATOR_IMAGE}
	@if [ $(NAMESPACE) != '' ];then \
		echo "Removing Custom Namespace: $(NAMESPACE)"; \
		cd $(PROJECT_DIR)/config/deploy && $(KUSTOMIZE) edit set namespace $(DEFAULT_REPO); \
	fi

.PHONY: bundle
bundle: export IMAGE_TAG := $(IMAGE_TAG)
bundle: export BUNDLE_VERSION := $(BUNDLE_VERSION)
bundle: manifests kustomize operator-sdk ## Generate bundle manifests and metadata, then validate generated files.
	$(OPERATOR_SDK) generate kustomize manifests -q
	cd config/manager && $(KUSTOMIZE) edit set image controller=$(OPERATOR_IMAGE)
	envsubst \
        < config/manifests/bases/authorino-operator.clusterserviceversion.template.yaml \
        > config/manifests/bases/authorino-operator.clusterserviceversion.yaml
	$(KUSTOMIZE) build config/manifests | $(OPERATOR_SDK) generate bundle -q --overwrite --version $(BUNDLE_VERSION) $(BUNDLE_METADATA_OPTS) --package authorino-operator
	$(OPERATOR_SDK) bundle validate ./bundle
	# Roll back edit
	cd config/manager && $(KUSTOMIZE) edit set image controller=${DEFAULT_OPERATOR_IMAGE}

.PHONY: bundle-build
bundle-build: ## Build the bundle image.
	docker build -f bundle.Dockerfile -t $(BUNDLE_IMG) .

.PHONY: bundle-push
bundle-push: ## Push the bundle image.
	$(MAKE) docker-push OPERATOR_IMAGE=$(BUNDLE_IMG)

.PHONY: fix-csv-replaces
fix-csv-replaces: $(YQ)
	$(eval REPLACES_VERSION=$(shell curl -sSL -H "Accept: application/vnd.github+json" \
               https://api.github.com/repos/Kuadrant/authorino-operator/releases/latest | \
               jq -r '.name'))
	V="authorino-operator.$(REPLACES_VERSION)" $(YQ) eval '.spec.replaces = strenv(V)' -i bundle/manifests/authorino-operator.clusterserviceversion.yaml

.PHONY: prepare-release
prepare-release:
	$(MAKE) manifests bundle VERSION=$(VERSION) AUTHORINO_VERSION=$(AUTHORINO_VERSION)
	@if [ "$(AUTHORINO_VERSION)" = "latest" ]; then\
		[ ! -e "$(AUTHORINO_IMAGE_FILE)" ] || rm $(AUTHORINO_IMAGE_FILE); \
	else \
	    echo quay.io/kuadrant/authorino:v$(AUTHORINO_VERSION) > $(AUTHORINO_IMAGE_FILE); \
	fi
	$(MAKE) fix-csv-replaces

# A comma-separated list of bundle images (e.g. make catalog-build BUNDLE_IMGS=example.com/operator-bundle:v0.1.0,example.com/operator-bundle:v0.2.0).
# These images MUST exist in a registry and be pull-able.
BUNDLE_IMGS ?= $(BUNDLE_IMG)

# The image tag given to the resulting catalog image (e.g. make catalog-build CATALOG_IMG=example.com/operator-catalog:v0.2.0).
CATALOG_IMG ?= $(IMAGE_TAG_BASE)-catalog:$(IMAGE_TAG)

# Set CATALOG_BASE_IMG to an existing catalog image tag to add $BUNDLE_IMGS to that image.
ifneq ($(origin CATALOG_BASE_IMG), undefined)
FROM_INDEX_OPT := --from-index $(CATALOG_BASE_IMG)
endif

# Build a catalog image by adding bundle images to an empty catalog using the operator package manager tool, 'opm'.
# This recipe invokes 'opm' in 'semver' bundle add mode. For more information on add modes, see:
# https://github.com/operator-framework/community-operators/blob/7f1438c/docs/packaging-operator.md#updating-your-existing-operator
.PHONY: catalog-build
catalog-build: opm ## Build a catalog image.
	$(OPM) index add --container-tool docker --mode semver --tag $(CATALOG_IMG) --bundles $(BUNDLE_IMGS) $(FROM_INDEX_OPT)

.PHONY: catalog-generate
catalog-generate: opm ## Generate a catalog/index Dockerfile.
	$(OPM) index add --generate --container-tool docker --mode semver --tag $(CATALOG_IMG) --bundles $(BUNDLE_IMGS) $(FROM_INDEX_OPT)

# Push the catalog image.
.PHONY: catalog-push
catalog-push: ## Push a catalog image.
	$(MAKE) docker-push OPERATOR_IMAGE=$(CATALOG_IMG)

##@ Verify

## Targets to verify actions that generate/modify code have been executed and output committed

.PHONY: verify-manifests
verify-manifests: manifests ## Verify manifests update.
	git diff --exit-code ./config
	[ -z "$$(git ls-files --other --exclude-standard --directory --no-empty-directory ./config)" ]

.PHONY: verify-bundle
verify-bundle: bundle ## Verify bundle update.
	git diff --exit-code ./bundle
	[ -z "$$(git ls-files --other --exclude-standard --directory --no-empty-directory ./bundle)" ]

.PHONY: verify-fmt
verify-fmt: fmt ## Verify fmt update.
	git diff --exit-code ./api ./controllers

## local configurations

deploy-develmode: manifests kustomize ## Deploy controller in debug mode to the K8s cluster specified in ~/.kube/config.
	cd config/manager && $(KUSTOMIZE) edit set image controller=${OPERATOR_IMAGE}
	@if [ $(NAMESPACE) != '' ];then \
		echo "Setting Custom Namespace: $(NAMESPACE)"; \
		cd $(PROJECT_DIR)/config/default && $(KUSTOMIZE) edit set namespace $(NAMESPACE); \
	fi

	cd $(PROJECT_DIR) && $(KUSTOMIZE) build config/deploy > $(DEPLOYMENT_FILE)

	$(KUSTOMIZE) build config/default | kubectl apply -f -

# clean up
	cd config/manager && $(KUSTOMIZE) edit set image controller=${DEFAULT_OPERATOR_IMAGE}
	@if [ $(NAMESPACE) != '' ];then \
		echo "Removing Custom Namespace: $(NAMESPACE)"; \
		cd $(PROJECT_DIR)/config/default && $(KUSTOMIZE) edit set namespace $(DEFAULT_REPO); \
	fi

.PHONY: local-cleanup
local-cleanup:
	$(MAKE) kind-delete-cluster

.PHONY: local-env-setup
local-env-setup:
	$(MAKE) kind-delete-cluster
	$(MAKE) kind-create-cluster
	$(KUSTOMIZE) build config/crd | kubectl apply -f -

.PHONY: local-setup
local-setup: export OPERATOR_IMAGE := authorino-operator:dev
local-setup:
	$(MAKE) local-env-setup
	$(MAKE) docker-build
	echo "Deploying Authorino control plane"
	$(KIND) load docker-image ${OPERATOR_IMAGE} --name ${KIND_CLUSTER_NAME}
	$(MAKE) install
	$(MAKE) deploy

.PHONY: local-redeploy
local-redeploy: export OPERATOR_IMAGE := authorino-operator:dev
local-redeploy:
	$(MAKE) docker-build
	echo "Deploying Authorino control plane"
	$(KIND) load docker-image ${OPERATOR_IMAGE} --name ${KIND_CLUSTER_NAME}

	@if [ $(NAMESPACE) != '' ];then \
		kubectl rollout restart deployment -n $(NAMESPACE) authorino-operator; \
		echo "Wait for all deployments to be up"; \
		kubectl -n $(NAMESPACE) wait --timeout=300s --for=condition=Available deployments --all; \
		else \
		kubectl rollout restart deployment -n $(DEFAULT_REPO) authorino-operator; \
		echo  "Wait for all deployments to be up"; \
		kubectl -n $(DEFAULT_REPO) wait --timeout=300s --for=condition=Available deployments --all; \
	fi

## kind configuration

KIND_CLUSTER_NAME ?= authorino-local

.PHONY: kind-create-cluster
kind-create-cluster: kind ## Create the "authorino-local" kind cluster.
	$(KIND) create cluster --name $(KIND_CLUSTER_NAME) --config utils/kind-cluster.yaml

.PHONY: kind-delete-cluster
kind-delete-cluster: kind ## Delete the "authorino-local" kind cluster.
	$(KIND) delete cluster --name $(KIND_CLUSTER_NAME)