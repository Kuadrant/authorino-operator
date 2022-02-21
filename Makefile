# VERSION defines the project version for the bundle.
VERSION ?= 0.0.0

# Address of the container registry
REGISTRY = quay.io

# Organization in container resgistry
ORG ?= 3scale

# IMAGE_TAG_BASE defines the docker.io namespace and part of the image name for remote images.
IMAGE_TAG_BASE ?= $(REGISTRY)/$(ORG)/authorino-operator

ifeq (0.0.0,$(VERSION))
IMAGE_TAG ?= latest
else
IMAGE_TAG ?= v$(VERSION)
endif

# Image URL to use all building/pushing image targets
DEFAULT_OPERATOR_IMAGE = $(IMAGE_TAG_BASE):$(IMAGE_TAG)
OPERATOR_IMAGE ?= $(DEFAULT_OPERATOR_IMAGE)

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

all: build

##@ General

help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

manifests: controller-gen kustomize authorino-manifests ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role webhook paths="./..." output:crd:artifacts:config=config/crd/bases && $(KUSTOMIZE) build config/install > $(OPERATOR_MANIFESTS)
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
	KUBEBUILDER_ASSETS='$(strip $(shell $(SETUP_ENVTEST) --arch=amd64 use -p path 1.22.x))'  go test ./... -coverprofile cover.out

##@ Build

build: generate fmt vet ## Build manager binary.
	go build -o bin/manager main.go

run: manifests generate fmt vet ## Run a controller from your host.
	go run ./main.go

docker-build:  ## Build docker image with the manager.
	docker build -t ${OPERATOR_IMAGE} .

docker-push: ## Push docker image with the manager.
	docker push ${OPERATOR_IMAGE}

##@ Deployment

install: manifests kustomize install-authorino ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	kubectl apply -f $(OPERATOR_MANIFESTS)

uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config.
	kubectl delete -f $(OPERATOR_MANIFESTS)

deploy: manifests kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd config/manager && $(KUSTOMIZE) edit set image controller=${OPERATOR_IMAGE}
	$(KUSTOMIZE) build config/default | kubectl apply -f -
	# rollback kustomize edit
	cd config/manager && $(KUSTOMIZE) edit set image controller=${DEFAULT_OPERATOR_IMAGE}

undeploy: ## Undeploy controller from the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/default | kubectl delete -f -

install-authorino: ## install RBAC and CRD for authorino
	$(KUSTOMIZE) build config/authorino | kubectl apply -f -

CONTROLLER_GEN = $(shell pwd)/bin/controller-gen
controller-gen: ## Download controller-gen locally if necessary.
	$(call go-get-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen@v0.6.1)

KUSTOMIZE = $(shell pwd)/bin/kustomize
kustomize: ## Download kustomize locally if necessary.
	$(call go-get-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v3@v3.8.7)

# go-get-tool will 'go get' any package $2 and install it to $1.
PROJECT_DIR := $(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))
define go-get-tool
@[ -f $(1) ] || { \
set -e ;\
TMP_DIR=$$(mktemp -d) ;\
cd $$TMP_DIR ;\
go mod init tmp ;\
echo "Downloading $(2)" ;\
GOBIN=$(PROJECT_DIR)/bin go get $(2) ;\
rm -rf $$TMP_DIR ;\
}
endef

DEPLOYMENT_DIR = $(PROJECT_DIR)/config/deploy
DEPLOYMENT_FILE = $(DEPLOYMENT_DIR)/manifests.yaml
.PHONY: deploy-manifest
deploy-manifest:
	mkdir -p $(DEPLOYMENT_DIR)
	cd $(PROJECT_DIR)/config/manager && $(KUSTOMIZE) edit set image controller=$(OPERATOR_IMAGE) ;\
	cd $(PROJECT_DIR) && $(KUSTOMIZE) build config/deploy > $(DEPLOYMENT_FILE)
	# clean up
	cd $(PROJECT_DIR)/config/manager && $(KUSTOMIZE) edit set image controller=${DEFAULT_OPERATOR_IMAGE}

OPERATOR_SDK = $(shell pwd)/bin/operator-sdk
OPERATOR_SDK_VERSION = v1.15.0
operator-sdk: ## Download operator-sdk locally if necessary.
	./utils/install-operator-sdk.sh $(OPERATOR_SDK) $(OPERATOR_SDK_VERSION)

.PHONY: bundle
bundle: manifests kustomize operator-sdk ## Generate bundle manifests and metadata, then validate generated files.
	$(OPERATOR_SDK) generate kustomize manifests -q
	cd config/manager && $(KUSTOMIZE) edit set image controller=$(OPERATOR_IMAGE)
	$(KUSTOMIZE) build config/manifests | $(OPERATOR_SDK) generate bundle -q --overwrite --version $(VERSION) $(BUNDLE_METADATA_OPTS) --package authorino-operator
	$(OPERATOR_SDK) bundle validate ./bundle
	# Roll back edit
	cd config/manager && $(KUSTOMIZE) edit set image controller=${DEFAULT_OPERATOR_IMAGE}

.PHONY: bundle-build
bundle-build: ## Build the bundle image.
	docker build -f bundle.Dockerfile -t $(BUNDLE_IMG) .

.PHONY: bundle-push
bundle-push: ## Push the bundle image.
	$(MAKE) docker-push OPERATOR_IMAGE=$(BUNDLE_IMG)

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
