##@ Helm Charts

# Chart name
CHART_NAME ?= authorino-operator
# Chart directory
CHART_DIRECTORY ?= charts/$(CHART_NAME)

.PHONY: helm-build
helm-build: $(YQ) kustomize manifests ## Build the helm chart from kustomize manifests
	# Set desired authorino image
	V="$(ACTUAL_DEFAULT_AUTHORINO_IMAGE)" $(YQ) eval '(select(.kind == "Deployment").spec.template.spec.containers[].env[] | select(.name == "RELATED_IMAGE_AUTHORINO").value) = strenv(V)' -i config/manager/manager.yaml
	# Replace the controller image
	cd config/helm && $(KUSTOMIZE) edit set namespace "{{ .Release.Namespace }}"
	cd config/authorino/webhook && $(KUSTOMIZE) edit set namespace "{{ .Release.Namespace }}"
	cd config/manager && $(KUSTOMIZE) edit set image controller=$(OPERATOR_IMAGE)
	# Build the helm chart templates from kustomize manifests
	$(KUSTOMIZE) build config/helm > $(CHART_DIRECTORY)/templates/manifests.yaml
	V="$(BUNDLE_VERSION)" $(YQ) -i e '.version = strenv(V)' $(CHART_DIRECTORY)/Chart.yaml
	V="$(BUNDLE_VERSION)" $(YQ) -i e '.appVersion = strenv(V)' $(CHART_DIRECTORY)/Chart.yaml
	# Roll back edit
	cd config/manager && $(KUSTOMIZE) edit set image controller=${DEFAULT_OPERATOR_IMAGE}
	cd config/helm && $(KUSTOMIZE) edit set namespace $(CHART_NAME)
	cd config/authorino/webhook && $(KUSTOMIZE) edit set namespace $(CHART_NAME)

.PHONY: helm-install
helm-install: $(HELM) ## Install the helm chart
	# Install the helm chart in the cluster
	$(HELM) install $(CHART_NAME) $(CHART_DIRECTORY)

.PHONY: helm-uninstall
helm-uninstall: $(HELM) ## Uninstall the helm chart
	# Uninstall the helm chart from the cluster
	$(HELM) uninstall $(CHART_NAME)

.PHONY: helm-upgrade
helm-upgrade: $(HELM) ## Upgrade the helm chart
	# Upgrade the helm chart in the cluster
	$(HELM) upgrade $(CHART_NAME) $(CHART_DIRECTORY)

.PHONY: helm-package
helm-package: $(HELM) ## Package the helm chart
	# Package the helm chart
	$(HELM) package $(CHART_DIRECTORY)

# GPG_KEY_UID: substring of the desired key's uid, the name or email
GPG_KEY_UID ?= 'Kuadrant Development Team'
# The keyring should've been imported before running this target
.PHONY: helm-package-sign
helm-package-sign: $(HELM) ## Package the helm chart and GPG sign it
	# Package the helm chart and sign it
	$(HELM) package --sign --key $(GPG_KEY_UID) $(CHART_DIRECTORY)

# GitHub Token with permissions to upload to the release assets
HELM_WORKFLOWS_TOKEN ?= <YOUR-TOKEN>
# GitHub Release Asset Browser Download URL, it can be find in the output of the uploaded asset
BROWSER_DOWNLOAD_URL ?= <BROWSER-DOWNLOAD-URL>
# Github repo name for the helm charts repository
HELM_REPO_NAME ?= helm-charts

CHART_VERSION ?= $(BUNDLE_VERSION)

.PHONY: helm-sync-package-created
helm-sync-package-created: ## Sync the helm chart package to the helm-charts repo
	curl -L \
	  -X POST \
	  -H "Accept: application/vnd.github+json" \
	  -H "Authorization: Bearer $(HELM_WORKFLOWS_TOKEN)" \
	  -H "X-GitHub-Api-Version: 2022-11-28" \
	  https://api.github.com/repos/$(ORG)/$(HELM_REPO_NAME)/dispatches \
	  -d '{"event_type":"chart-created","client_payload":{"chart":"$(CHART_NAME)","version":"$(CHART_VERSION)", "browser_download_url": "$(BROWSER_DOWNLOAD_URL)"}}'

.PHONY: helm-sync-package-deleted
helm-sync-package-deleted: ## Sync the deleted helm chart package to the helm-charts repo
	curl -L \
	  -X POST \
	  -H "Accept: application/vnd.github+json" \
	  -H "Authorization: Bearer $(HELM_WORKFLOWS_TOKEN)" \
	  -H "X-GitHub-Api-Version: 2022-11-28" \
	  https://api.github.com/repos/$(ORG)/$(HELM_REPO_NAME)/dispatches \
	  -d '{"event_type":"chart-deleted","client_payload":{"chart":"$(CHART_NAME)","version":"$(CHART_VERSION)"}}'
