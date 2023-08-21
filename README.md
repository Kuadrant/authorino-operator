# Authorino Operator

A Kubernetes Operator to manage [Authorino](https://github.com/Kuadrant/authorino) instances.

[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](http://www.apache.org/licenses/LICENSE-2.0)
[![codecov](https://codecov.io/gh/Kuadrant/authorino-operator/branch/main/graph/badge.svg?token=3O9IUKS642)](https://codecov.io/gh/Kuadrant/authorino-operator)

## Installation

The Operator can be installed by applying the manifests to the Kubernetes cluster or using [Operator Lifecycle Manager (OLM)](https://olm.operatorframework.io/)

### Applying the manifests to the cluster

1. Create the namespace for the Operator

```sh
kubectl create namespace authorino-operator
```

2. Install the Operator manifests

```sh
make install
```

3. Deploy the Operator

```sh
make deploy
```

<details>
  <summary><i>Tip:</i> Deploy a custom image of the Operator</summary>
  <br/>
  To deploy an image of the Operator other than the default <code>quay.io/kuadrant/authorino-operator:latest</code>, specify by setting the <code>OPERATOR_IMAGE</code> parameter. E.g.:

  ```sh
  make deploy OPERATOR_IMAGE=authorino-operator:local
  ```
</details>

### Installing via OLM

To install the Operator using the [Operator Lifecycle Manager](https://olm.operatorframework.io/), you need to make the
Operator CSVs available in the cluster by creating a `CatalogSource` resource.

The bundle and catalog images of the Operator are available in Quay.io:

<table>
  <tbody>
    <tr>
      <th>Bundle</th>
      <td><a href="https://quay.io/kuadrant/authorino-operator-bundle">quay.io/kuadrant/authorino-operator-bundle</a></td>
    </tr>
    <tr>
      <th>Catalog</th>
      <td><a href="https://quay.io/kuadrant/authorino-operator-catalog">quay.io/kuadrant/authorino-operator-catalog</a></td>
    </tr>
  </tbody>
</table>

1. Create the namespace for the Operator

```sh
kubectl create namespace authorino-operator
```

2. Create the [CatalogSource](https://olm.operatorframework.io/docs/concepts/crds/catalogsource) resource pointing to
   one of the images from in the Operator's catalog repo:

```sh
kubectl -n authorino-operator apply -f -<<EOF
apiVersion: operators.coreos.com/v1alpha1
kind: CatalogSource
metadata:
  name: operatorhubio-catalog
  namespace: authorino-operator
spec:
  sourceType: grpc
  image: quay.io/kuadrant/authorino-operator-catalog:latest
  displayName: Authorino Operator
EOF
```

### Installing via kind for local development

1. Create the kind cluster, build the operator image and deploy the operator.
```shell
make local-setup
```

2. Rebuild and Redeploy the operator image
```shell
make local-redeploy
```

3. Remove the kind cluster
```shell
make local-cleanup
```

## Requesting an Authorino instance

Once the Operator is up and running, you can request instances of Authorino by creating `Authorino` CRs. E.g.:

```sh
kubectl -n default apply -f -<<EOF
apiVersion: operator.authorino.kuadrant.io/v1beta1
kind: Authorino
metadata:
  name: authorino
spec:
  listener:
    tls:
      enabled: false
  oidcServer:
    tls:
      enabled: false
EOF
```

## The `Authorino` Custom Resource Definition (CRD)

API to install, manage and configure Authorino authorization services .

Each [`Authorino`](https://github.com/Kuadrant/authorino-operator/tree/main/config/crd/bases/operator.authorino.kuadrant.io_authorinos.yaml)
Custom Resource (CR) represents an instance of Authorino deployed to the cluster. The Authorino Operator will reconcile
the state of the Kubernetes Deployment and associated resources, based on the state of the CR.

### API Specification

| Field |              Type               | Description                                | Required/Default |
|-------|:-------------------------------:|--------------------------------------------|------------------|
| spec  | [AuthorinoSpec](#authorinospec) | Specification of the Authorino deployment. | Required         |

#### AuthorinoSpec

| Field                    |            Type             | Description                                                                                                                                                                                                                             | Required/Default                                      |
|--------------------------|:---------------------------:|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|-------------------------------------------------------|
| clusterWide              |           Boolean           | Sets the Authorino instance's [watching scope](https://github.com/Kuadrant/authorino/blob/main/docs/architecture.md#cluster-wide-vs-namespaced-instances) – cluster-wide or namespaced.                                                 | Default: `true` (cluster-wide)                        |
| authConfigLabelSelectors |           String            | [Label selectors](https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#label-selectors) used by the Authorino instance to filter `AuthConfig`-related reconciliation events.                                       | Default: empty (all AuthConfigs are watched)          |
| secretLabelSelectors     |           String            | [Label selectors](https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#label-selectors) used by the Authorino instance to filter `Secret`-related reconciliation events (API key and mTLS authentication methods). | Default: `authorino.kuadrant.io/managed-by=authorino` |
| replicas                 |           Integer           | Number of replicas desired for the Authorino instance. Values greater than 1 enable leader election in the Authorino service, where the leader updates the statuses of the `AuthConfig` CRs).                                           | Default: 1                                            |
| evaluatorCacheSize       |           Integer           | Cache size (in megabytes) of each Authorino evaluator (when enabled in an [`AuthConfig`](https://github.com/Kuadrant/authorino/blob/main/docs/features.md#common-feature-caching-cache)).                                               | Default: 1                                            |
| image                    |           String            | Authorino image to be deployed (for dev/testing purpose only).                                                                                                                                                                          | Default: `quay.io/kuadrant/authorino:latest`          |
| imagePullPolicy          |           String            | Sets the [imagePullPolicy](https://kubernetes.io/docs/concepts/containers/images) of the Authorino Deployment (for dev/testing purpose only).                                                                                           | Default: k8s default                                  |
| logLevel                 |           String            | Defines the level of log you want to enable in Authorino (`debug`, `info` and `error`).                                                                                                                                                 | Default: `info`                                       |
| logMode                  |           String            | Defines the log mode in Authorino (`development` or `production`).                                                                                                                                                                      | Default: `production`                                 |
| listener                 |    [Listener](#listener)    | Specification of the authorization service (gRPC interface).                                                                                                                                                                            | Required                                              |
| oidcServer               |  [OIDCServer](#oidcserver)  | Specification of the OIDC service.                                                                                                                                                                                                      | Required                                              |
| tracing                  |     [Tracing](#tracing)     | Configuration of the OpenTelemetry tracing exporter.                                                                                                                                                                                    | Optional                                              |
| metrics                  |     [Metrics](#metrics)     | Configuration of the metrics server (port, level).                                                                                                                                                                                      | Optional                                              |
| healthz                  |     [Healthz](#healthz)     | Configuration of the health/readiness probe (port).                                                                                                                                                                                     | Optional                                              |
| volumes                  | [VolumesSpec](#volumesspec) | Additional volumes to be mounted in the Authorino pods.                                                                                                                                                                                 | Optional                                              |

#### Listener

Configuration of the authorization server – [gRPC](https://github.com/Kuadrant/authorino/blob/main/docs/architecture.md#overview)
and [raw HTTP](https://github.com/Kuadrant/authorino/blob/main/docs/architecture.md#raw-http-authorization-interface)
interfaces

| Field   |      Type       | Description                                                                                                     | Required/Default                         |
|---------|:---------------:|-----------------------------------------------------------------------------------------------------------------|------------------------------------------|
| port    |     Integer     | Port number of authorization server (gRPC interface).                                                           | _**DEPRECATED**_<br/>Use `ports` instead |
| ports   | [Ports](#ports) | Port numbers of the authorization server (gRPC and raw HTTPinterfaces).                                         | Optional                                 |
| tls     |   [TLS](#tls)   | TLS configuration of the authorization server (GRPC and HTTP interfaces).                                       | Required                                 |
| timeout |     Integer     | Timeout of external authorization request (in milliseconds), controlled internally by the authorization server. | Default: `0` (disabled)                  |

#### OIDCServer

Configuration of the OIDC Discovery server for [Festival Wristband](https://github.com/Kuadrant/authorino/blob/main/docs/features.md#festival-wristband-tokens-responsewristband)
tokens.

| Field |    Type     | Description                                                                  | Required/Default |
|-------|:-----------:|------------------------------------------------------------------------------|------------------|
| port  |   Integer   | Port number of OIDC Discovery server for Festival Wristband tokens.          | Default: `8083`  |
| tls   | [TLS](#tls) | TLS configuration of the OIDC Discovery server for Festival Wristband tokens | Required         |

#### TLS

TLS configuration of server. Appears in [`listener`](#listener) and [`oidcServer`](#oidcserver).

| Field         |                                                           Type                                                            | Description                                                                             | Required/Default              |
|---------------|:-------------------------------------------------------------------------------------------------------------------------:|-----------------------------------------------------------------------------------------|-------------------------------|
| enabled       |                                                          Boolean                                                          | Whether TLS is enabled or disabled for the server.                                      | Default: `true`               |
| certSecretRef | [LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23/#localobjectreference-v1-core) | The reference to the secret that contains the TLS certificates `tls.crt` and `tls.key`. | Required when `enabled: true` |

#### Ports

Port numbers of the authorization server.

| Field |  Type   | Description                                                                                            | Required/Default |
|-------|:-------:|--------------------------------------------------------------------------------------------------------|------------------|
| grpc  | Integer | Port number of the gRPC interface of the authorization server. Set to 0 to disable this interface.     | Default: `50001` |
| http  | Integer | Port number of the raw HTTP interface of the authorization server. Set to 0 to disable this interface. | Default: `5001`  |

#### Tracing

Configuration of the OpenTelemetry tracing exporter.

| Field    |  Type  | Description                                                                                         | Required/Default |
|----------|:------:|-----------------------------------------------------------------------------------------------------|------------------|
| endpoint | String | Full endpoint of the OpenTelemetry tracing collector service (e.g. http://jaeger:14268/api/traces). | Required         |
| tags     |  Map   | Key-value map of fixed tags to add to all OpenTelemetry traces emitted by Authorino.                | Optional         |

#### Metrics

Configuration of the metrics server.

| Field |  Type   | Description                                                                                                                                                                                                    | Required/Default |
|-------|:-------:|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|------------------|
| port  | Integer | Port number of the metrics server.                                                                                                                                                                             | Default: `8080`  |
| deep  | Boolean | Enable/disable metrics at the level of each evaluator config (if requested in the [`AuthConfig`](https://github.com/Kuadrant/authorino/blob/main/docs/user-guides/metrics.md)) exported by the metrics server. | Default: `false` |

#### Healthz

Configuration of the health/readiness probe (port).

| Field |  Type   | Description                                | Required/Default |
|-------|:-------:|--------------------------------------------|------------------|
| port  | Integer | Port number of the health/readiness probe. | Default: `8081`  |

#### VolumesSpec

Additional volumes to project in the Authorino pods. Useful for validation of TLS self-signed certificates of external
services known to have to be contacted by Authorino at runtime.

| Field       |            Type             | Description                                                                                                                        | Required/Default |
|-------------|:---------------------------:|------------------------------------------------------------------------------------------------------------------------------------|------------------|
| items       | [[]VolumeSpec](#volumespec) | List of additional volume items to project.                                                                                        | Optional         |
| defaultMode |           Integer           | Mode bits used to set permissions on the files. Must be an octal value between 0000 and 0777 or a decimal value between 0 and 511. | Optional         |

#### VolumeSpec

| Field      |                                                 Type                                                  | Description                                                                             | Required/Default                                 |
|------------|:-----------------------------------------------------------------------------------------------------:|-----------------------------------------------------------------------------------------|--------------------------------------------------|
| name       |                                                String                                                 | Name of the volume and volume mount within the Deployment. It must be unique in the CR. | Optional                                         |
| mountPath  |                                                String                                                 | Absolute path where to mount all the items.                                             | Required                                         |
| configMaps |                                               []String                                                | List of of Kubernetes ConfigMap names to mount.                                         | Required exactly one of: `confiMaps`, `secrets`. |
| secrets    |                                               []String                                                | List of of Kubernetes Secret names to mount.                                            | Required exactly one of: `confiMaps`, `secrets`. |
| items      | [[]KeyToPath](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23/#keytopath-v1-core) | Mount details for selecting specific ConfigMap or Secret entries.                       | Optional                                         |

### Full example

```yaml
apiVersion: operator.authorino.kuadrant.io/v1beta1
kind: Authorino
metadata:
  name: authorino
spec:
  clusterWide: true
  authConfigLabelSelectors: environment=production
  secretLabelSelectors: authorino.kuadrant.io/component=authorino,environment=production

  replicas: 2

  evaluatorCacheSize: 2 # mb

  image: quay.io/kuadrant/authorino:latest
  imagePullPolicy: Always

  logLevel: debug
  logMode: production

  listener:
    ports:
      grpc: 50001
      http: 5001
    tls:
      enabled: true
      certSecretRef:
        name: authorino-server-cert # secret must contain `tls.crt` and `tls.key` entries

  oidcServer:
    port: 8083
    tls:
      enabled: true
      certSecretRef:
        name: authorino-oidc-server-cert # secret must contain `tls.crt` and `tls.key` entries

  metrics:
    port: 8080
    deep: true

  volumes:
    items:
      - name: keycloak-tls-cert
        mountPath: /etc/ssl/certs
        configMaps:
          - keycloak-tls-cert
        items: # details to mount the k8s configmap in the authorino pods
          - key: keycloak.crt
            path: keycloak.crt
    defaultMode: 420
```
