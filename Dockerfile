# Build the authorino binary
# https://catalog.redhat.com/software/containers/ubi9/go-toolset
FROM registry.access.redhat.com/ubi9/go-toolset:1.20 AS builder
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

ARG VERSION=latest
ARG DEFAULT_AUTHORINO_IMAGE=quay.io/kuadrant/authorino:latest
RUN CGO_ENABLED=0 GO111MODULE=on go build -a -ldflags "-X main.version=${VERSION} -X github.com/kuadrant/authorino-operator/controllers.DefaultAuthorinoImage=${DEFAULT_AUTHORINO_IMAGE}" -o manager main.go

# Use Red Hat minimal base image to package the binary
# https://catalog.redhat.com/software/containers/ubi9-minimal
FROM registry.access.redhat.com/ubi9-minimal:latest
WORKDIR /
COPY --from=builder /workspace/manager .
USER 1001

ENTRYPOINT ["/manager"]
