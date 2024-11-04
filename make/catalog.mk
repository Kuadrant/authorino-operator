##@ Operator Catalog

# The image tag given to the resulting catalog image (e.g. make catalog-build CATALOG_IMG=example.com/operator-catalog:v0.2.0).
CATALOG_IMG ?= $(IMAGE_TAG_BASE)-catalog:$(IMAGE_TAG)

OPM_DOCKERFILE_VERSION ?= 1.28.0

ifeq ($(origin CATALOG_ARCH),undefined)
OPM_DOCKERFILE_TAG = latest
else
OPM_DOCKERFILE_TAG = v$(OPM_DOCKERFILE_VERSION)-$(CATALOG_ARCH)
endif

CATALOG_FILE = $(PROJECT_DIR)/catalog/authorino-operator-catalog/operator.yaml
CATALOG_DOCKERFILE = $(PROJECT_DIR)/catalog/authorino-operator-catalog.Dockerfile

$(CATALOG_DOCKERFILE): $(OPM)
	-mkdir -p $(PROJECT_DIR)/catalog/authorino-operator-catalog
	cd $(PROJECT_DIR)/catalog && $(OPM) generate dockerfile authorino-operator-catalog -i "quay.io/operator-framework/opm:${OPM_DOCKERFILE_TAG}"
catalog-dockerfile: $(CATALOG_DOCKERFILE) ## Generate catalog dockerfile.

$(CATALOG_FILE): $(OPM) $(YQ)
	@echo "************************************************************"
	@echo Build authorino operator catalog
	@echo
	@echo BUNDLE_IMG					= $(BUNDLE_IMG)
	@echo CHANNELS						= $(CHANNELS)
	@echo "************************************************************"
	@echo
	@echo Please check this matches your expectations and override variables if needed.
	@echo
	$(PROJECT_DIR)/utils/generate-catalog.sh $(OPM) $(YQ) $(BUNDLE_IMG) $@ $(CHANNELS)

.PHONY: catalog
catalog: $(OPM) ## Generate catalog content and validate.
	# Initializing the Catalog
	-rm -rf $(PROJECT_DIR)/catalog/authorino-operator-catalog
	-rm -rf $(PROJECT_DIR)/catalog/authorino-operator-catalog.Dockerfile
	$(MAKE) $(CATALOG_DOCKERFILE)
	$(MAKE) $(CATALOG_FILE) BUNDLE_IMG=$(BUNDLE_IMG)
	cd $(PROJECT_DIR)/catalog && $(OPM) validate authorino-operator-catalog

# Build a catalog image by adding bundle images to an empty catalog using the operator package manager tool, 'opm'.
# Ref https://olm.operatorframework.io/docs/tasks/creating-a-catalog/#catalog-creation-with-raw-file-based-catalogs
.PHONY: catalog-build
catalog-build: ## Build a catalog image.
	# Build the Catalog
	docker build $(PROJECT_DIR)/catalog -f $(PROJECT_DIR)/catalog/authorino-operator-catalog.Dockerfile -t $(CATALOG_IMG)

# Push the catalog image.
.PHONY: catalog-push
catalog-push: ## Push a catalog image.
	$(MAKE) docker-push IMG=$(CATALOG_IMG)

deploy-catalog: $(KUSTOMIZE) $(YQ) ## Deploy operator to the K8s cluster specified in ~/.kube/config using OLM catalog image.
	V="$(CATALOG_IMG)" $(YQ) eval '.spec.image = strenv(V)' -i config/deploy/olm/catalogsource.yaml
	$(KUSTOMIZE) build config/deploy/olm | kubectl apply -f -

undeploy-catalog: $(KUSTOMIZE) ## Undeploy controller from the K8s cluster specified in ~/.kube/config using OLM catalog image.
	$(KUSTOMIZE) build config/deploy/olm | kubectl delete -f -
