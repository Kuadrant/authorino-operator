# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Authorino Operator is a Kubernetes Operator built with the Operator SDK that
manages [Authorino](https://github.com/Kuadrant/authorino) authorization service instances. It deploys and configures
Authorino instances in Kubernetes clusters based on the `Authorino` Custom Resource Definition (CRD).

**Technology Stack:** Go • Kubebuilder/controller-runtime • Kubernetes client-go • Ginkgo/Gomega testing • zap logger

## Quick Reference

| I want to...                  | Go to                                                                                   |
|-------------------------------|-----------------------------------------------------------------------------------------|
| Test a change locally         | [Quick Start](#quick-start-for-local-development)                                       |
| Modify API types              | [Critical Rules](#critical-rules-must-read) → [Making API Changes](#making-api-changes) |
| Add a new reconciled resource | [Adding New Reconciled Resources](#adding-new-reconciled-resources)                     |
| Understand the architecture   | [Architecture & Key Concepts](#architecture--key-concepts)                              |
| Debug reconciliation issues   | [Troubleshooting](#troubleshooting)                                                     |
| Create a release              | [Release Process](#release-process) → `make prepare-release`                            |
| Find a specific file          | [Key File Locations](#key-file-locations)                                               |
| See common commands           | [Common Development Commands](#common-development-commands)                             |

## Critical Rules (MUST READ)

> **These rules prevent common CI failures.** Read this section before making any changes.

**After modifying API types** (`api/v1beta1/authorino_types.go`):

```bash
make generate manifests       # Regenerate code and CRDs
make bundle helm-build        # Update bundle and helm chart (keeps these in sync)
# Then commit ALL generated files (CRDs, RBAC, deepcopy, bundle, helm, etc.)
```

**Before committing any changes:**

```bash
make verify-manifests verify-bundle verify-fmt  # CI will fail if these don't pass
```

**TLS Configuration:**

- The sample configs use `enabled: false` for easier local development (see `config/samples/`)
- If enabled, operator validates that TLS secrets exist before deployment
- For production, set `enabled: true` and provide TLS cert secrets

**Version variables:**

- `VERSION` = operator version (e.g., `0.5.0`)
- `AUTHORINO_VERSION` = operand version (e.g., `0.13.0`)

**Release workflow requires:**

- `make prepare-release VERSION=x.y.z AUTHORINO_VERSION=a.b.c` (updates manifests, bundle, AND helm chart)

## Key File Locations

- **Main entrypoint**: `main.go`
- **API types**: `api/v1beta1/authorino_types.go`
- **Primary CRD**: `config/crd/bases/operator.authorino.kuadrant.io_authorinos.yaml`
- **Controller**: `controllers/authorino_controller.go` (top-level reconciler, finalizers, preflight checks)
- **Core reconciler**: `pkg/reconcilers/authorino_reconciler.go` (resource reconciliation logic)
- **Resource builders**: `pkg/resources/k8s_*.go` (creates K8s objects)
- **Mutators**: `pkg/reconcilers/*_mutator.go` (defines desired state for resources)
- **Constants**: `controllers/constants.go` (finalizer name: `authorinoFinalizer`, status messages)
- **Example CRs**: `config/samples/authorino-operator_v1beta1_authorino.yaml`

## Quick Start for Local Development

Complete workflow to develop and test changes locally:

```bash
# 1. Create a local kind cluster
make kind-create-cluster

# 2. Install cert-manager (required for operator's admission webhooks)
make install-cert-manager

# 3. Install CRDs (operator's Authorino CRD + Authorino operand's CRDs)
make install

# 4. Run the operator locally (connects to kind cluster via kubeconfig)
make run
# Alternative: Build and deploy operator image to cluster
# make docker-build OPERATOR_IMAGE=authorino-operator:local
# kind load docker-image authorino-operator:local --name authorino-local
# make deploy OPERATOR_IMAGE=authorino-operator:local

# 5. In another terminal, create an Authorino instance
kubectl apply -f config/samples/authorino-operator_v1beta1_authorino.yaml

# 6. Verify the Authorino instance is running
kubectl get authorino authorino-sample -o yaml
kubectl get deployment -n default

# 7. Clean up
make kind-delete-cluster
```

## Development Workflows

### Making API Changes

1. Edit `api/v1beta1/authorino_types.go`
2. Run `make generate manifests` to update generated code and CRDs
3. Run `make bundle` to generate the Operator Lifecycle Manager manifest bundle
4. Run `make helm-build` to generate the Helm charts
5. Update tests in `controllers/authorino_controller_test.go` if needed
6. Run `make test` to verify changes
7. Commit both source changes and generated files

### Adding New Reconciled Resources

1. Create builder function in `pkg/resources/k8s_*.go`
2. Create mutator in `pkg/reconcilers/*_mutator.go`
3. Add reconciliation call in `pkg/reconcilers/authorino_reconciler.go`
4. Update RBAC markers (`+kubebuilder:rbac` comments) in `controllers/authorino_controller.go`
5. Run `make manifests` to regenerate RBAC manifests

### Testing Strategy

Tests use controller-runtime's `envtest` which spins up a minimal Kubernetes API server:

- Test files: `*_test.go` alongside source files
- Main test suite: `controllers/suite_test.go`
- Integration tests: `controllers/authorino_controller_test.go`
- Unit tests: `pkg/reconcilers/*_test.go`

### Release Process

See RELEASE.md for full details. Key points:

- Use GitHub Actions workflow "Release operator"
- Specify operator version (without 'v') and compatible Authorino version
- Workflow builds manifests, bundle, and images
- Images pushed to quay.io/kuadrant/*

## Common Development Commands

**Most Used:**

```bash
make build                    # Build operator binary
make run                      # Run operator locally (uses kubeconfig)
make test                     # Run all tests with coverage
make fmt                      # Format code
make vet                      # Run go vet
make generate manifests       # After API changes: regenerate code and CRDs
make verify-manifests verify-bundle verify-fmt  # Before commit: verify everything
```

**Local Development with Kind:**

```bash
make kind-create-cluster      # Create "authorino-local" kind cluster
make install-cert-manager     # Install cert-manager (required for webhooks)
make install                  # Install CRDs (operator + Authorino operand)
make deploy OPERATOR_IMAGE=...  # Deploy operator to cluster
make kind-delete-cluster      # Clean up cluster
```

**Testing:**

```bash
make test                               # All tests with coverage
make test AUTHORINO_VERSION=0.13.0      # With specific operand version
go test ./controllers -v                # Specific package
go test ./controllers -run TestName -v  # Specific test
```

**Container Images:**

```bash
make docker-build                                     # Build image (default: quay.io/kuadrant/authorino-operator:latest)
make docker-build OPERATOR_IMAGE=myregistry/img:tag   # Custom image
make docker-push                                      # Push image
```

**Release Workflow:**

```bash
make prepare-release VERSION=x.y.z AUTHORINO_VERSION=a.b.c  # Prepare all release artifacts
make bundle VERSION=0.5.0 AUTHORINO_VERSION=0.13.0          # Generate OLM bundle
make helm-build VERSION=0.5.0 AUTHORINO_VERSION=0.13.0      # Generate Helm chart
make catalog BUNDLE_IMG=quay.io/.../bundle:v0.5.0           # Generate catalog
```

**OLM Deployment:**

```bash
make bundle-build && make bundle-push   # Build and push bundle image
make catalog-build && make catalog-push # Build and push catalog image
make deploy-catalog                     # Deploy via OLM catalog
make undeploy-catalog                   # Remove OLM deployment
```

**Helm:**

```bash
make helm-install      # Install chart locally
make helm-upgrade      # Upgrade installed chart
make helm-uninstall    # Remove chart
make helm-package      # Package chart for distribution
```

## Architecture & Key Concepts

### Controller Reconciliation Pattern

The operator uses a **two-layer reconciler pattern**:

1. **controllers/authorino_controller.go** - Top-level controller that:
    - Implements the main reconciliation loop
    - Handles finalizers for cleanup of cluster-scoped resources
    - Performs preflight checks (e.g., TLS secret validation)
    - Orchestrates calls to the core reconciler

2. **pkg/reconcilers/authorino_reconciler.go** - Core reconciler that:
    - Reconciles all Kubernetes resources (Deployment, Services, ServiceAccount, RBAC)
    - Uses mutator pattern for resource updates
    - Manages status conditions
    - Contains business logic for resource creation/updates

### Resource Reconciliation Flow

The operator reconciles the following resources for each `Authorino` CR:

1. **Services** - Authorization server (gRPC/HTTP) and OIDC discovery endpoints
2. **ServiceAccount** - For Authorino pods
3. **RBAC** - Roles, ClusterRoles, RoleBindings, ClusterRoleBindings
4. **Deployment** - Authorino authorization server pods

**Mutator Pattern**: Each resource type has dedicated mutator functions in `pkg/reconcilers/*_mutator.go`. Mutators
define the desired state of resources. The reconciler compares the actual resource with the desired state and updates if
needed. This follows the Kubernetes reconciliation pattern: observe → compare → update.

### Key Packages

- **api/v1beta1/** - API types for the `Authorino` CRD
- **controllers/** - Main reconciliation controller logic
- **pkg/reconcilers/** - Core reconciliation logic and mutators
- **pkg/resources/** - Kubernetes resource builders (Deployment, Services, RBAC)
- **pkg/log/** - Custom logger wrapper around zap
- **pkg/condition/** - Status condition helpers

### Operand vs Operator

- **Operator**: This codebase - manages Authorino instances
- **Operand**: Authorino authorization service - the workload being managed
- The operator embeds Authorino CRD manifests from the operand repository (in `config/authorino/`)
- Default Authorino image is set at compile time via `reconcilers.DefaultAuthorinoImage` but can be overridden in the
  `Authorino` CR spec

### Important Configuration Points

**Default Authorino Image:**

- Configured in `build.yaml` file (set to "latest" in main branch)
- Injected at compile time via linker flags to `pkg/reconcilers.DefaultAuthorinoImage`
- Can be overridden per-instance via `spec.image` in the `Authorino` CR

**Authorino Manifests:**

- Sourced from operand repository during build via `make authorino-manifests`
- Template: `config/authorino/kustomization.template.yaml`
- Uses `AUTHORINO_GITREF` and `AUTHORINO_IMAGE_TAG` environment variables
- Generated: `config/authorino/kustomization.yaml`

### Reconciliation & Deployment Behavior

**Reconciliation Order:**

- Resources reconciled in sequence: Services → ServiceAccount → RBAC → Deployment
- Ensures dependencies exist before Deployment creation

**Finalizer Cleanup:**

- Uses `authorinoFinalizer` (in `controllers/constants.go`)
- Cleans up cluster-scoped ClusterRoleBindings on deletion
- Required because K8s doesn't auto-GC cluster-scoped resources

**Cluster-scoped vs Namespaced:**

- `spec.clusterWide: true` → watches all namespaces, requires ClusterRoleBindings
- `spec.clusterWide: false` → watches single namespace, uses RoleBindings
- Finalizer handles ClusterRoleBinding cleanup for cluster-wide instances

## Deployment Methods

**Two ways to deploy the operator:**

1. **Direct installation** (`make install deploy`)
    - Installs CRDs and deploys operator directly via kubectl
    - **Use for:** Local development and testing
    - **Fast iteration:** Changes apply immediately

2. **OLM-based deployment** (`make deploy-catalog`)
    - Uses Operator Lifecycle Manager to install and manage the operator
    - **Build process:** Create bundle (`make bundle`) → Create catalog from bundle(s) (`make catalog`) → Deploy catalog
    - **Use for:** Production deployments, testing upgrades, environments where OLM manages operators
    - **Key concepts:**
        - **Bundle**: Metadata package for a single operator version (intermediate artifact, not deployed directly)
        - **Catalog**: Index of one or more bundles, deployed to cluster
    - **Commands:**
        - `make bundle bundle-build bundle-push` - Package operator version as bundle
        - `make catalog catalog-build catalog-push` - Build catalog index from bundle(s)
        - `make deploy-catalog` - Deploy operator via OLM catalog
        - `make undeploy-catalog` - Remove OLM deployment

## Troubleshooting

| Problem                                 | Solution                                                                                                                                                                                                            |
|-----------------------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **"TLS secret not provided/not found"** | TLS is enabled but secret missing. Either: (1) Create secret with `tls.crt`/`tls.key`, OR (2) Disable: `spec.listener.tls.enabled: false` and `spec.oidcServer.tls.enabled: false`                                  |
| **CI fails on `verify-manifests`**      | Run `make generate manifests` → commit all generated files → run `make verify-manifests` locally before pushing                                                                                                     |
| **Operator not reconciling**            | (1) `kubectl logs -n authorino-operator deployment/authorino-operator -f`<br>(2) `kubectl describe authorino <name>` (check events)<br>(3) Verify CRDs: `kubectl get crd authorinos.operator.authorino.kuadrant.io` |
| **Tests fail (envtest errors)**         | Run `make setup-envtest` (downloads minimal K8s API server for testing)                                                                                                                                             |
| **Changes not appearing**               | Did you run `make generate manifests` after API changes? Check git status for uncommitted generated files                                                                                                           |
| **Deployment reconciliation loop**      | Check for conflicting owner references or manual edits to operator-managed resources                                                                                                                                |

**Debug Commands:**

```bash
# View all resources created by operator
kubectl get all -l app.kubernetes.io/managed-by=authorino-operator

# Check Authorino CR status
kubectl get authorino -o yaml

# Operator logs with debug level (when running locally)
make run LOG_LEVEL=debug
```
