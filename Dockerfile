FROM registry.access.redhat.com/ubi9-minimal:latest AS builder
USER root
RUN microdnf install -y tar gzip
RUN arch=""; \
    case $(uname -m) in \
      x86_64) arch="amd64";; \
      aarch64) arch="arm64";; \
    esac; \
    curl -O -J "https://dl.google.com/go/go1.18.7.linux-${arch}.tar.gz"; \
    tar -C /usr/local -xzf go1.18.7.linux-${arch}.tar.gz; \
    ln -s /usr/local/go/bin/go /usr/local/bin/go
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

ARG version=latest
RUN CGO_ENABLED=0 GO111MODULE=on go build -a -ldflags "-X main.version=${version}" -o manager main.go

# Use Red Hat minimal base image to package the binary
# https://catalog.redhat.com/software/containers/ubi9-minimal
FROM registry.access.redhat.com/ubi9-minimal:latest
WORKDIR /
COPY --from=builder /workspace/manager .
USER 1001

ENTRYPOINT ["/manager"]
