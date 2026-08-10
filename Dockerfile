# ---------------------------------------------------
# Stage 1: Build environment
# ---------------------------------------------------
FROM golang:alpine AS builder

WORKDIR /app

# Install CA certificates for outgoing HTTPS requests
RUN apk add --no-cache ca-certificates

# 1. First copy only dependencies to cache them efficiently
COPY go.mod go.sum ./
# Cache dependencies and Go build cache across builds
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download

# 2. Finally copy the actual source code
COPY . .

# Build statically compiled and stripped binary
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o kube-prober .

# ---------------------------------------------------
# Stage 2: Minimal runtime image (~18 MB)
# ---------------------------------------------------
FROM scratch

WORKDIR /app

# Copy root CA certificates for TLS verification
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy the compiled binary
COPY --from=builder /app/kube-prober /app/kube-prober

# Run application
ENTRYPOINT ["/app/kube-prober"]