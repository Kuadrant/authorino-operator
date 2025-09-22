# Use bash as shell
# Setting SHELL to bash allows bash commands to be executed by recipes.
# This is a requirement for 'setup-envtest.sh' in the test target.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

MKFILE_PATH := $(abspath $(lastword $(MAKEFILE_LIST)))

PROJECT_DIR := $(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))

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

# Bundle CSV
BUNDLE_CSV = bundle/manifests/authorino-operator.clusterserviceversion.yaml

# Operand version. It can be a semantic version (X.Y.Z), a branch name, git SHA or 'latest'. If not specified, it will default to 'latest'.
ifeq ($(AUTHORINO_VERSION),)
AUTHORINO_VERSION = latest
endif
operand_using_semantic_version := $(shell [[ $(AUTHORINO_VERSION) =~ ^[0-9]+\.[0-9]+\.[0-9]+(-.+)?$$ ]] && echo "true")
ifdef operand_using_semantic_version
AUTHORINO_IMAGE_TAG = v$(AUTHORINO_VERSION)
AUTHORINO_GITREF = v$(AUTHORINO_VERSION)
else
AUTHORINO_IMAGE_TAG = $(AUTHORINO_VERSION)
ifeq ($(AUTHORINO_VERSION),latest)
AUTHORINO_GITREF = main
else
AUTHORINO_GITREF = $(AUTHORINO_VERSION)
endif
endif

# Container Engine to be used for building image and with kind
CONTAINER_ENGINE ?= docker

# Build file used to store replaces/authorinoImage options.
BUILD_CONFIG_FILE ?= build.yaml
DEFAULT_AUTHORINO_IMAGE = $(DEFAULT_REGISTRY)/$(DEFAULT_ORG)/authorino:$(AUTHORINO_IMAGE_TAG)
ACTUAL_DEFAULT_AUTHORINO_IMAGE ?= $(shell $(YQ) e -e '.config.authorinoImage' $(BUILD_CONFIG_FILE) || echo $(DEFAULT_AUTHORINO_IMAGE))

COVER_PKGS = ./pkg/...,./controllers/...,./api/...

all: build

##@ General

help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Tools

# go-install-tool will 'go install' any package $2 and install it to $1.
define go-install-tool
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

OPERATOR_SDK = $(shell pwd)/bin/operator-sdk
OPERATOR_SDK_VERSION = v1.32.0
operator-sdk: ## Download operator-sdk locally if necessary.
	./utils/install-operator-sdk.sh $(OPERATOR_SDK) $(OPERATOR_SDK_VERSION)

CONTROLLER_GEN = $(shell pwd)/bin/controller-gen
controller-gen: ## Download controller-gen locally if necessary.
	$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen@v0.15.0)

KUSTOMIZE = $(PROJECT_DIR)/bin/kustomize
$(KUSTOMIZE):
	$(call go-install-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v4@v4.5.5)

.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary.

YQ = $(shell pwd)/bin/yq
YQ_VERSION := v4.34.2
$(YQ):
	$(call go-install-tool,$(YQ),github.com/mikefarah/yq/v4@$(YQ_VERSION))

.PHONY: yq
yq: $(YQ) ## Download yq locally if necessary.

ARCH ?= $(shell go env GOARCH)
OPM = $(PROJECT_DIR)/bin/opm
OPM_VERSION ?= 1.48.0
$(OPM):
	@{ \
	set -e ;\
	mkdir -p $(dir $(OPM)) ;\
	OS=$(shell go env GOOS) && \
	curl -sSLo $(OPM) https://github.com/operator-framework/operator-registry/releases/download/v$(OPM_VERSION)/$${OS}-$(ARCH)-opm ;\
	chmod +x $(OPM) ;\
	}

.PHONY: opm
opm: $(OPM) ## Download opm locally if necessary.

HELM = ./bin/helm
HELM_VERSION = v3.15.0
$(HELM):
	@{ \
	set -e ;\
	mkdir -p $(dir $(HELM)) ;\
	OS=$(shell go env GOOS) && ARCH=$(shell go env GOARCH) && \
	curl -sL -o helm.tar.gz https://get.helm.sh/helm-$(HELM_VERSION)-$${OS}-$${ARCH}.tar.gz ;\
	tar -zxvf helm.tar.gz ;\
	mv $${OS}-$${ARCH}/helm $(HELM) ;\
	chmod +x $(HELM) ;\
	rm -rf $${OS}-$${ARCH} helm.tar.gz ;\
	}

.PHONY: helm
helm: $(HELM) ## Download helm locally if necessary.

KIND = $(PROJECT_DIR)/bin/kind
KIND_VERSION = v0.23.0
$(KIND):
	$(call go-install-tool,$(KIND),sigs.k8s.io/kind@$(KIND_VERSION))

.PHONY: kind
kind: $(KIND) ## Download kind locally if necessary.

setup-envtest: ## Setup envtest.
ifeq (, $(shell which setup-envtest))
	go install sigs.k8s.io/controller-runtime/tools/setup-envtest@release-0.16
SETUP_ENVTEST=$(GOBIN)/setup-envtest
else
SETUP_ENVTEST=$(shell which setup-envtest)
endif

# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.29.0

# Cert manager is required for the webhooks.
CERT_MANAGER_VERSION ?= 1.12.1

##@ Development

manifests: controller-gen kustomize authorino-manifests ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) crd rbac:roleName=authorino-operator-manager webhook paths="./..." output:crd:artifacts:config=config/crd/bases && $(KUSTOMIZE) build config/install > $(OPERATOR_MANIFESTS)
	$(MAKE) deploy-manifest OPERATOR_IMAGE=$(OPERATOR_IMAGE)

.PHONY: authorino-manifests
authorino-manifests: export AUTHORINO_GITREF := $(AUTHORINO_GITREF)
authorino-manifests: export AUTHORINO_IMAGE_TAG := $(AUTHORINO_IMAGE_TAG)
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

test: manifests generate fmt vet setup-envtest ## Run the tests.
	echo $(SETUP_ENVTEST)
	KUBEBUILDER_ASSETS='$(strip $(shell $(SETUP_ENVTEST)  use -p path $(ENVTEST_K8S_VERSION)))' \
		go test -ldflags="-X github.com/kuadrant/authorino-operator/pkg/reconcilers.DefaultAuthorinoImage=$(ACTUAL_DEFAULT_AUTHORINO_IMAGE)" \
		-coverprofile cover.out \
	  	--coverpkg $(COVER_PKGS) \
		./...

##@ Build

build: GIT_SHA=$(shell git rev-parse HEAD || echo "unknown")
build: DIRTY=$(shell $(PROJECT_DIR)/utils/check-git-dirty.sh || echo "unknown")
build: generate fmt vet $(YQ) ## Build manager binary.
	go build -ldflags "-X main.version=$(VERSION) -X main.gitSHA=${GIT_SHA} -X main.dirty=${DIRTY} -X github.com/kuadrant/authorino-operator/pkg/reconcilers.DefaultAuthorinoImage=$(ACTUAL_DEFAULT_AUTHORINO_IMAGE)" -o bin/manager main.go

run: GIT_SHA=$(shell git rev-parse HEAD || echo "unknown")
run: DIRTY=$(shell $(PROJECT_DIR)/utils/check-git-dirty.sh || echo "unknown")
run: manifests generate fmt vet ## Run a controller from your host.
	go run -ldflags "-X main.version=$(VERSION) -X main.gitSHA=${GIT_SHA} -X main.dirty=${DIRTY} -X github.com/kuadrant/authorino-operator/pkg/reconcilers.DefaultAuthorinoImage=$(ACTUAL_DEFAULT_AUTHORINO_IMAGE)" ./main.go --log-level debug --log-mode development

docker-build: GIT_SHA=$(shell git rev-parse HEAD || echo "unknown")
docker-build: DIRTY=$(shell $(PROJECT_DIR)/utils/check-git-dirty.sh || echo "unknown")
docker-build:  ## Build docker image with the manager.
	docker build --build-arg OPERATOR_VERSION=$(VERSION) --build-arg GIT_SHA=$(GIT_SHA) --build-arg DIRTY=$(DIRTY) --build-arg ACTUAL_DEFAULT_AUTHORINO_IMAGE=$(ACTUAL_DEFAULT_AUTHORINO_IMAGE) -t $(OPERATOR_IMAGE) .

docker-push: ## Push docker image with the manager.
	docker push ${OPERATOR_IMAGE}

##@ Deployment

install: install-authorino install-operator ## Install CRDs into the K8s cluster specified in ~/.kube/config.

uninstall: uninstall-operator uninstall-authorino ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config.

deploy: manifests kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd config/manager && $(KUSTOMIZE) edit set image controller=${OPERATOR_IMAGE}
	$(KUSTOMIZE) build config/default | kubectl apply -f -
	# rollback kustomize edit
	cd config/manager && $(KUSTOMIZE) edit set image controller=${DEFAULT_OPERATOR_IMAGE}

undeploy: ## Undeploy controller from the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/default | kubectl delete -f - --ignore-not-found

install-operator: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	kubectl apply -f $(OPERATOR_MANIFESTS)

uninstall-operator: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config.
	kubectl delete -f $(OPERATOR_MANIFESTS) --ignore-not-found

install-authorino: $(KUSTOMIZE) create-namespace ## install RBAC and CRD for authorino
	$(KUSTOMIZE) build config/authorino | kubectl apply -f -

uninstall-authorino: $(KUSTOMIZE) ## uninstall RBAC and CRD for authorino
	$(KUSTOMIZE) build config/authorino | kubectl delete -f - --ignore-not-found

install-cert-manager: ## install the cert manager need for the web hooks
	kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v${CERT_MANAGER_VERSION}/cert-manager.yaml
	kubectl -n cert-manager wait --timeout=300s --for=condition=Available deployments --all

uninstall-cert-manager: ## uninstall the cert manager need for the web hooks
	kubectl delete -f https://github.com/cert-manager/cert-manager/releases/download/v${CERT_MANAGER_VERSION}/cert-manager.yaml --ignore-not-found

create-namespace:
	kubectl create namespace authorino-operator --dry-run=client -o yaml | kubectl apply -f - ## handle namespace already existing.

delete-namespace:
	kubectl delete namespace authorino-operator --ignore-not-found

DEPLOYMENT_DIR = $(PROJECT_DIR)/config/deploy
DEPLOYMENT_FILE = $(DEPLOYMENT_DIR)/manifests.yaml
.PHONY: deploy-manifest
deploy-manifest: $(KUSTOMIZE)
	mkdir -p $(DEPLOYMENT_DIR)
	cd $(PROJECT_DIR)/config/manager && $(KUSTOMIZE) edit set image controller=$(OPERATOR_IMAGE) ;\
	cd $(PROJECT_DIR) && $(KUSTOMIZE) build config/deploy > $(DEPLOYMENT_FILE)
	# clean up
	cd $(PROJECT_DIR)/config/manager && $(KUSTOMIZE) edit set image controller=${DEFAULT_OPERATOR_IMAGE}

##@ OLM manifest bundle

.PHONY: bundle
bundle: export IMAGE_TAG := $(IMAGE_TAG)
bundle: export BUNDLE_VERSION := $(BUNDLE_VERSION)
bundle: manifests kustomize operator-sdk $(YQ) ## Generate bundle manifests and metadata, then validate generated files.
	$(OPERATOR_SDK) generate kustomize manifests -q
	V="$(ACTUAL_DEFAULT_AUTHORINO_IMAGE)" $(YQ) eval '(select(.kind == "Deployment").spec.template.spec.containers[].env[] | select(.name == "RELATED_IMAGE_AUTHORINO").value) = strenv(V)' -i config/manager/manager.yaml
	cd config/manager && $(KUSTOMIZE) edit set image controller=$(OPERATOR_IMAGE)
	envsubst \
        < config/manifests/bases/authorino-operator.clusterserviceversion.template.yaml \
        > config/manifests/bases/authorino-operator.clusterserviceversion.yaml
	$(KUSTOMIZE) build config/manifests | $(OPERATOR_SDK) generate bundle -q --overwrite --version $(BUNDLE_VERSION) $(BUNDLE_METADATA_OPTS) --package authorino-operator
	($(YQ) e -e '.config.replaces' $(BUILD_CONFIG_FILE) && \
		V="$(shell $(YQ) e -e '.config.replaces' $(BUILD_CONFIG_FILE))" $(YQ) eval '.spec.replaces = strenv(V)' -i $(BUNDLE_CSV)) || \
		($(YQ) eval '.' -i $(BUNDLE_CSV) && echo "no replaces added")
	$(OPERATOR_SDK) bundle validate ./bundle
	# Roll back edit
	cd config/manager && $(KUSTOMIZE) edit set image controller=${DEFAULT_OPERATOR_IMAGE}
	$(MAKE) bundle-custom-modifications

.PHONY: bundle-custom-modifications
OPENSHIFT_VERSIONS_ANNOTATION_KEY="com.redhat.openshift.versions"
# Supports Openshift v4.12+ (https://redhat-connect.gitbook.io/certified-operator-guide/ocp-deployment/operator-metadata/bundle-directory/managing-openshift-versions)
OPENSHIFT_SUPPORTED_VERSIONS="v4.12"
bundle-custom-modifications:
	# Set Openshift version in bundle annotations
	$(YQ) -i '.annotations[$(OPENSHIFT_VERSIONS_ANNOTATION_KEY)] = $(OPENSHIFT_SUPPORTED_VERSIONS)' bundle/metadata/annotations.yaml
	$(YQ) -i '(.annotations[$(OPENSHIFT_VERSIONS_ANNOTATION_KEY)] | key) headComment = "Custom annotations"' bundle/metadata/annotations.yaml
	# Set Openshift version in bundle Dockerfile
	@echo "" >> bundle.Dockerfile
	@echo "# Custom labels" >> bundle.Dockerfile
	@echo "LABEL $(OPENSHIFT_VERSIONS_ANNOTATION_KEY)=$(OPENSHIFT_SUPPORTED_VERSIONS)" >> bundle.Dockerfile

.PHONY: bundle-build
bundle-build: ## Build the bundle image.
	docker build -f bundle.Dockerfile -t $(BUNDLE_IMG) .

.PHONY: bundle-push
bundle-push: ## Push the bundle image.
	$(MAKE) docker-push OPERATOR_IMAGE=$(BUNDLE_IMG)

##@ Release

.PHONY: create-build-file
create-build-file: $(YQ) ## Creates the build info file.
	$(YQ) -n '.config' > $(BUILD_CONFIG_FILE)

.PHONY: set-authorino-default-image
set-authorino-default-image: $(YQ) ## Sets the default Authorino image in the build file.
	@if [ "$(AUTHORINO_VERSION)" != "latest" ]; then\
		V="$(DEFAULT_REGISTRY)/$(DEFAULT_ORG)/authorino:$(AUTHORINO_IMAGE_TAG)" $(YQ) eval '.config.authorinoImage = strenv(V)' -i $(BUILD_CONFIG_FILE); \
	fi

.PHONY: set-replaces-directive
set-replaces-directive: $(YQ) ## Sets the value for the OLM replaces directive in the build file.
	$(eval REPLACES_VERSION=$(shell curl -sSL -H "Accept: application/vnd.github+json" \
               https://api.github.com/repos/Kuadrant/authorino-operator/releases/latest | \
               jq -r '.name'))
	V="authorino-operator.$(REPLACES_VERSION)" $(YQ) e -i '.config.replaces = strenv(V)' $(BUILD_CONFIG_FILE)

.PHONY: prepare-release
prepare-release: ## Prepares a release: create build info file, generate manifests, OLM bundle and Helm chart.
	$(MAKE) create-build-file
	$(MAKE) set-authorino-default-image
	$(MAKE) set-replaces-directive
	$(MAKE) manifests bundle VERSION=$(VERSION) AUTHORINO_VERSION=$(AUTHORINO_VERSION)
	$(MAKE) helm-build VERSION=$(VERSION) AUTHORINO_VERSION=$(AUTHORINO_VERSION)

##@ Verify

## Targets to verify actions that generate/modify code have been executed and output committed

OPERATOR_IMAGE_REPO = $(shell echo $(OPERATOR_IMAGE) | cut -d: -f1)
DEFAULT_AUTHORINO_IMAGE_REPO = $(shell echo $(DEFAULT_AUTHORINO_IMAGE) | cut -d: -f1)

.PHONY: verify-manifests
verify-manifests: manifests $(YQ) ## Verify manifests update.
	git diff -I'^    createdAt:' -I'$(OPERATOR_IMAGE_REPO)' -I'$(DEFAULT_AUTHORINO_IMAGE_REPO)' --exit-code -- ./config ':(exclude)config/authorino/kustomization.yaml'
	[ -z "$$(git ls-files --other --exclude-standard --directory --no-empty-directory ./config)" ]
	$(YQ) ea -e 'select([.][].kind == "Deployment") | select([.][].metadata.name == "authorino-operator").spec.template.spec.containers[0].image | . == "$(OPERATOR_IMAGE)"' config/deploy/manifests.yaml
# $(YQ) ea -e 'select([.][].kind == "Deployment") | select([.][].metadata.name == "authorino-webhooks").spec.template.spec.containers[0].image | . == "$(DEFAULT_AUTHORINO_IMAGE)"' config/deploy/manifests.yaml
	$(YQ) ea -e 'select([.][].kind == "Deployment") | select([.][].metadata.name == "authorino-operator").spec.template.spec.containers[0].env | select(.[].name == "RELATED_IMAGE_AUTHORINO") | .[].value == "$(DEFAULT_AUTHORINO_IMAGE)"' config/deploy/manifests.yaml
	$(YQ) e -e '.metadata.annotations.containerImage == "$(OPERATOR_IMAGE)"' config/manifests/bases/authorino-operator.clusterserviceversion.yaml

.PHONY: verify-bundle
verify-bundle: bundle $(YQ) ## Verify bundle update.
	git diff -I'^    createdAt:' -I'$(OPERATOR_IMAGE_REPO)' -I'$(DEFAULT_AUTHORINO_IMAGE_REPO)' --exit-code -- ./bundle ':(exclude)config/authorino/kustomization.yaml'
	[ -z "$$(git ls-files --other --exclude-standard --directory --no-empty-directory ./bundle)" ]
	$(YQ) e -e '.metadata.annotations.containerImage == "$(OPERATOR_IMAGE)"' $(BUNDLE_CSV)
	$(YQ) e -e '.spec.install.spec.deployments[0].spec.template.spec.containers[0].image == "$(OPERATOR_IMAGE)"' $(BUNDLE_CSV)
	$(YQ) e -e '.spec.install.spec.deployments[0].spec.template.spec.containers[0].env | select(.[].name == "RELATED_IMAGE_AUTHORINO") | .[].value == "$(DEFAULT_AUTHORINO_IMAGE)"' $(BUNDLE_CSV)
#	$(YQ) e -e '.spec.install.spec.deployments[1].spec.template.spec.containers[0].image == "$(DEFAULT_AUTHORINO_IMAGE)"' $(BUNDLE_CSV)

.PHONY: verify-fmt
verify-fmt: fmt ## Verify fmt update.
	git diff --exit-code ./api ./controllers

# Include last to avoid changing MAKEFILE_LIST used above
include ./make/*.mk
