# Authorino Operator

[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](http://www.apache.org/licenses/LICENSE-2.0)

## Overview

A Kubernetes Operator to manage [Authorino](https://github.com/Kuadrant/authorino) deployments.

## Custom Resources

* [Authorino](https://github.com/Kuadrant/authorino-operator/blob/3592a17868250de7079f26584059d09bbb51ff70/config/crd/bases/operator.authorino.kuadrant.io_authorinos.yaml) to install, manager and configure authorization services. Each CRD represents an instance of Authorino running on the cluster.

### Authorino's API Specification

#### Spec

| Field | Type | Description |
|----------|:----:|---------|
| image    | string | Authorino image to be deployed, it will default to `quay.io/3scale/authorino:latest` if not specified |
| replicas | number | Number of replicas desired for the given instance, default to one if not specified |
| logLevel | string | Defines the level of log you want to enable in authorino (`debug`, `info` and `error`), if not specified it defaults to `info` |
| logMode | string | Defines the log mode in authorino (`development` or `production`), default to one if not specified `production` |
| imagePullPolicy | string | Defines the [imagePullPolicy](https://kubernetes.io/docs/concepts/containers/images/) for when a deployment is created  |
| clusterWide | boolean | Defines the scope of an instance of Authorino, it can be either `namespaced` or `cluster-wide` for further information look at the [Authorino deployment guide](https://github.com/Kuadrant/authorino/blob/main/docs/deploy.md#5-deploy-authorino-instances). It will default to `cluster-wide` if not specified |
| listener | [Listener](#listener)  | Specification of the authorization service (gRPC interface) |
| oidcServer | [OIDC Server](#oidcserver) | Specification of the OIDC service |

#### Listener

Specifies the details to communicate to an external authenticiation server, check out [Authorino's architecture](https://github.com/Kuadrant/authorino/blob/main/docs/architecture.md) for further details.

| Field | json field | Type | Info  |
|----------|---------|:----:|---------|
| Port     | port | number | Specify to port to the server, if not speficied it will default to port `50051` |
| Tls     | tls | object | [TLS](#tls) |

#### OIDCServer

Specifies the details to communicate to an OIDC server, check out [Authorino's architecture](https://github.com/Kuadrant/authorino/blob/main/docs/architecture.md) for further details.

| Field | Type | Description  |
|----------|:----:|---------|
| port | number | Specify to port to the server, if not speficied it will default to port `8083` |
| tls | [TLS](#tls) | Specification of the TLS certificate |

#### TLS 

Defines the TLS certificate to a server

| field | Type | Description  |
|----------|:----:|---------|
| enabled | boolean | Defines the TLS certificate is enabled, which will make the cert secret mandatory (Defaults to true) |
| certSecretRef   | [v1.LocalObjectReference](https://v1-15.docs.kubernetes.io/docs/reference/generated/kubernetes-api/v1.15/#localobjectreference-v1-core) | The reference to the secret that contains the TLS certificates `tls.crt` and `tls.key`.|

Example:
```yaml
apiVersion: operator.authorino.kuadrant.io/v1beta1
kind: Authorino
metadata:
  name: authorino-sample
spec:
  image: quay.io/3scale/authorino:latest
  replicas: 1
  imagePullPolicy: Always
  clusterWide: true
  listener:
    port: 
    tls:
      enabled: true
      certSecretRef:
        name: authorino-cert # secret must contain `tls.crt` and `tls.key` entries
  oidcServer:
    port:
    tls:
      enabled: true 
      certSecretRef: 
        name: authorino-cert # secret must contain `tls.crt` and `tls.key` entries
```

## Installation

The Operator can be installed manually by deploying its resources on to a cluster or using the [Operator Lifecycle Manager](https://olm.operatorframework.io/)


### Deploying to a cluster

To deploy the operator onto a cluster follow the steps below:

1) Create a namespace to deploy the operator
```bash
kubectl create namespace authorino-operator
```

2) Install the operator manifests
```bash
make install
```
3) Deploy the operator 
```bash
make deploy
```

> if you want to deploy a different image of the operator, you can specify the image by adding the `OPERATOR_IMAGE` variable.
>
> * _The image needs to be present in the cluster_
>
> eg.: `make deploy OPERATOR_IMAGE=authorino-operator:local`

### Installing via OLM

To install the operator using the [Operator Lifecycle Manager](https://olm.operatorframework.io/), you need to make the operator CSVs available on the cluster via a CatalogSource resource. 

The bundles and catalog with the versions of the operator are available in the `quay.io` repo 

* [Bundles](https://quay.io/repository/3scale/authorino-operator-bundle)
* [Catalogs](https://quay.io/repository/3scale/authorino-operator-catalog)

Create a [CatalogSource](https://olm.operatorframework.io/docs/concepts/crds/catalogsource/) pointing to one of the images available in the operator's catalogs repo 

```bash
# Create a namespace to deploy the operator
kubectl create namespace authorino-operator

# Create the catalogsource resource
kubectl -n authorino-operator apply -f -<<EOF
apiVersion: operators.coreos.com/v1alpha1
kind: CatalogSource
metadata:
  name: operatorhubio-catalog
  namespace: authorino-operator
spec:
  sourceType: grpc
  image: quay.io/3scale/authorino-operator-catalog:v0.0.1
  displayName: Authorino Operator
EOF
```

## Creating an instance of authorino

Once the operator is up and running you can deploy an instance of Authorino by creating an Authorino CRD.

Check out the authorino documentation for more information about authorino.

```bash
kubectl -n myapi apply -f -<<EOF
apiVersion: operator.authorino.kuadrant.io/v1beta1
kind: Authorino
metadata:
  name: authorino
spec:
  replicas: 1
  clusterWide: false
  listener:
    tls:
      enabled: false
  oidcServer:
    tls:
      enabled: false
EOF
```

