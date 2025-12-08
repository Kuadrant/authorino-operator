# Build the authorino binary
# https://catalog.redhat.com/software/containers/ubi10/go-toolset
FROM --platform=$BUILDPLATFORM registry.access.redhat.com/ubi10/go-toolset:1.25 AS builder
USER root
WORKDIR /workspace

# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download
# Copy the go source
COPY main.go main.go
COPY api/ api/
COPY controllers/ controllers/
COPY pkg/ pkg/

ARG OPERATOR_VERSION=latest
ARG DEFAULT_AUTHORINO_IMAGE=quay.io/kuadrant/authorino:latest
ARG GIT_SHA=unknown
ARG DIRTY=unknown
ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} GOARM=${TARGETVARIANT} \
    go build -a -ldflags "-X main.version=${OPERATOR_VERSION} -X main.gitSHA=${GIT_SHA} -X main.dirty=${DIRTY} -X github.com/kuadrant/authorino-operator/pkg/reconcilers.DefaultAuthorinoImage=${DEFAULT_AUTHORINO_IMAGE}" \
    -o manager main.go

# Use Red Hat minimal base image to package the binary
# https://catalog.redhat.com/software/containers/ubi9-minimal
FROM registry.access.redhat.com/ubi9-minimal:latest
WORKDIR /
COPY --from=builder /workspace/manager .
USER 1001

ENTRYPOINT ["/manager"]
