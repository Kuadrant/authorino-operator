# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

The Authorino Operator is a Kubernetes Operator built using the Operator SDK and controller-runtime framework. It manages instances of [Authorino](https://github.com/Kuadrant/authorino), a Kubernetes-native authorization service. The operator deploys and configures Authorino instances based on `Authorino` Custom Resource (CR) definitions.

## Architecture

### Core Components

- **API (`api/v1beta1/`)**: Defines the `Authorino` CRD schema (Custom Resource Definition)
  - `authorino_types.go`: Contains the `AuthorinoSpec` and related configuration structures
  - The CRD supports configurations for listener ports (gRPC/HTTP), OIDC server, metrics, tracing, RBAC, and volume mounts

- **Controllers (`controllers/`)**: Kubernetes controller logic
  - `authorino_controller.go`: Main reconciliation loop that orchestrates deployment, services, and RBAC
  - Uses finalizers to clean up cluster-scoped resources on deletion
  - Handles status updates and error conditions

- **Reconcilers (`pkg/reconcilers/`)**: Business logic for reconciling Kubernetes resources
  - `authorino_reconciler.go`: Core reconciliation logic for Deployments, Services, and RBAC
  - **Mutator Pattern**: Uses mutator functions to declaratively specify desired state changes
    - `deployment_mutator.go`: Functions like `DeploymentReplicasMutator`, `DeploymentImageMutator`, etc.
    - `service_mutator.go`: Service-specific mutators
    - Mutators are composable - multiple mutators can be chained together
  - `status.go`: Status management utilities
  - The reconciler manages three services per Authorino instance:
    - Auth service (gRPC/HTTP authorization endpoints)
    - OIDC service (for Festival Wristband tokens)
    - Metrics service

- **Resources (`pkg/resources/`)**: Factory functions for Kubernetes resources
  - `k8s_deployment.go`: Deployment resource creation
  - `k8s_services.go`: Service resource creation
  - `k8s_rbac.go`: RBAC resource creation (ClusterRoles, RoleBindings, ServiceAccounts)
  - `map_utils.go`: Utilities for working with labels and annotations

### Reconciliation Flow

1. Controller receives an `Authorino` CR event
2. Controller calls reconcilers in sequence:
   - `ReconcileAuthorinoDeployment()`: Creates/updates the Authorino Deployment
   - `ReconcileAuthorinoServices()`: Creates/updates the three Services (auth, OIDC, metrics)
   - `ReconcileAuthorinoPermissions()`: Sets up RBAC (ClusterRoleBindings and RoleBindings based on `clusterWide` setting)
3. Each reconciler uses mutator functions to compute the desired state
4. Status is updated to reflect the current state

### Key Design Patterns

- **Mutator Functions**: The codebase uses a functional approach to resource updates. Each mutator function checks if a specific field needs updating and returns a boolean indicating if changes were made. This makes the reconciliation logic composable and testable.

- **Cluster-Wide vs Namespaced**: The `clusterWide` field determines whether Authorino watches all namespaces or just its own. This affects RBAC setup (ClusterRoleBinding vs RoleBinding).

- **Default Image Management**: The default Authorino image is injected at compile time via ldflags (`pkg/reconcilers.DefaultAuthorinoImage`). This allows releases to be pinned to specific Authorino versions.

## Common Development Commands

### Building and Testing

```bash
# Run all tests with coverage
make test

# Run tests for a specific package
KUBEBUILDER_ASSETS='$(shell bin/setup-envtest use -p path 1.29.0)' \
  go test -v ./pkg/reconcilers/

# Format code
make fmt

# Run linter
make vet

# Generate code (DeepCopy methods, CRDs, RBAC manifests)
make generate

# Generate manifests (CRDs, RBAC, webhooks)
make manifests

# Build the operator binary
make build

# Build Docker image
make docker-build OPERATOR_IMAGE=<your-image>
```

### Local Development with Kind

```bash
# Create a local Kind cluster
make kind-create-cluster

# Delete the local Kind cluster
make kind-delete-cluster

# Install dependencies (cert-manager required for webhooks)
make install-cert-manager

# Install the operator CRDs
make install

# Deploy the operator to the cluster
make deploy

# Deploy with a custom image
make deploy OPERATOR_IMAGE=authorino-operator:local

# Undeploy the operator
make undeploy

# Uninstall CRDs
make uninstall
```

### OLM Bundle and Catalog

```bash
# Generate the OLM bundle
make bundle VERSION=<version> AUTHORINO_VERSION=<authorino-version>

# Build the bundle image
make bundle-build

# Push the bundle image
make bundle-push
```

### Helm Chart

```bash
# Build the Helm chart
make helm-build VERSION=<version> AUTHORINO_VERSION=<authorino-version>
```

### Release Preparation

```bash
# Prepare a release (creates build file, generates manifests, bundle, and Helm chart)
make prepare-release VERSION=<version> AUTHORINO_VERSION=<authorino-version>
```

## Configuration and Environment Variables

- **VERSION**: Git commit SHA or semantic version for the operator
- **AUTHORINO_VERSION**: Version of Authorino to use (semantic version, git SHA, branch, or 'latest')
- **OPERATOR_IMAGE**: Full image reference for the operator (default: `quay.io/kuadrant/authorino-operator:latest`)
- **CONTAINER_ENGINE**: Container runtime to use (default: `docker`)
- **REGISTRY/ORG/REPO**: Override default registry settings (default: `quay.io/kuadrant/authorino-operator`)

## Testing Strategy

- Uses Ginkgo/Gomega for BDD-style tests
- Tests use `envtest` (from controller-runtime) to run against a minimal Kubernetes API server
- Key test files:
  - `controllers/authorino_controller_test.go`: Controller integration tests
  - `pkg/reconcilers/authorino_reconciler_test.go`: Reconciler unit tests
  - Mutator tests: `*_mutator_test.go` files test individual mutator functions

## Important Files and Directories

- `config/`: Kustomize manifests for deployment
  - `config/crd/bases/`: Generated CRD YAML files
  - `config/manager/`: Operator Deployment manifests
  - `config/rbac/`: RBAC manifests for the operator itself
  - `config/samples/`: Example Authorino CR manifests
  - `config/authorino/`: Upstream Authorino manifests (fetched from Authorino repo)
- `bundle/`: OLM bundle manifests
- `charts/`: Helm chart
- `make/*.mk`: Makefile includes for specific functionality (helm, kind, catalog)
- `build.yaml`: Build configuration file created during releases (stores replaces directive and authorino image)

## Working with the Codebase

### Adding a New Field to the Authorino CRD

1. Update `api/v1beta1/authorino_types.go` with the new field
2. Run `make generate` to update DeepCopy methods
3. Run `make manifests` to regenerate CRD YAML
4. Run `make bundle` to generate the Operator Lifecycle Manager manifest bundle
5. Run `make helm-build` to generate the Helm charts
6. Update reconciler logic in `pkg/reconcilers/` to handle the new field
7. Create a mutator function if the field affects Deployment/Service resources
8. Add tests for the new functionality
9. Update the README.md API specification table

### Modifying Reconciliation Logic

1. Identify which reconciler method needs changes (Deployment, Services, or Permissions)
2. Add or modify mutator functions in the appropriate `*_mutator.go` file
3. Register the mutator in the reconciler's mutator chain
4. Add unit tests in `*_mutator_test.go`
5. Run integration tests with `make test`

### Default Authorino Image Updates

The default Authorino image is injected at compile time. When building:
- Set `AUTHORINO_VERSION` to specify which Authorino version to use
- The build process updates `config/manager/manager.yaml` with the `RELATED_IMAGE_AUTHORINO` env var
- This env var is used by the operator to know which Authorino image to deploy when not explicitly specified in the CR

## Verification

Before submitting changes:

```bash
# Verify manifests are up to date
make verify-manifests

# Verify bundle is up to date
make verify-bundle

# Verify code formatting
make verify-fmt
```
