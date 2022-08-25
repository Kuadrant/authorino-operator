# Build the operator binary
FROM registry.access.redhat.com/ubi8/go-toolset:1.17.10 as builder
USER root
WORKDIR /workspace
ARG authorinoversion=latest
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

RUN CGO_ENABLED=0 GO111MODULE=on go build -a -ldflags "-X api/v1beta1.authorino_types.AuthorinoVersion=${authorinoversion}" -o manager main.go

# Use Red Hat minimal base image to package the binary
# https://catalog.redhat.com/software/containers/ubi8-minimal
FROM registry.access.redhat.com/ubi8/ubi-minimal:latest
WORKDIR /
COPY --from=builder /workspace/manager .
USER 1001

ENTRYPOINT ["/manager"]
