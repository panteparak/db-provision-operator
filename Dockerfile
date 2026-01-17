# syntax=docker/dockerfile:1

# Build the manager binary
FROM golang:1.24 AS builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace

# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum

# Download dependencies with BuildKit cache mount
# This cache persists across builds and is much faster than layer caching
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Copy the go source
COPY cmd/main.go cmd/main.go
COPY api/ api/
COPY internal/ internal/

# Build with BuildKit cache mounts for both modules and build cache
# - /go/pkg/mod: Go module cache (avoids re-downloading)
# - /root/.cache/go-build: Go build cache (avoids recompiling unchanged packages)
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} \
    go build -a -o manager cmd/main.go

# Production image with database client tools for backup/restore operations
# Using Alpine instead of distroless to include pg_dump, mysqldump, etc.
FROM alpine:3.21

# Install PostgreSQL and MySQL client tools required for backup/restore
# Use BuildKit cache mount for APK packages to speed up rebuilds
RUN --mount=type=cache,target=/var/cache/apk \
    apk add --no-cache \
    postgresql16-client \
    mysql-client \
    ca-certificates \
    tzdata

# Create non-root user matching distroless UID/GID
RUN adduser -D -u 65532 -g 65532 nonroot

WORKDIR /
COPY --from=builder /workspace/manager .

# Run as non-root user
USER 65532:65532

ENTRYPOINT ["/manager"]
