# ---------------------------------------------------
# Stage 1: Build environment - Special thanks to @liggitt, @BenTheElder for inspiring
# ---------------------------------------------------
ARG GOLANG_IMAGE=golang:latest
FROM --platform=$BUILDPLATFORM ${GOLANG_IMAGE} AS builder

ARG TARGETOS
ARG TARGETARCH
ARG GOTOOLCHAIN
ENV GOTOOLCHAIN=${GOTOOLCHAIN}

WORKDIR /app

# 1. Cache dependencies
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
  --mount=type=cache,target=/root/.cache/go-build \
  go mod download

# 2. Copy source code
COPY . .

# Build statically compiled and stripped binary matching target architecture
RUN --mount=type=cache,target=/go/pkg/mod \
  --mount=type=cache,target=/root/.cache/go-build \
  CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -ldflags="-s -w" -o kube-prober .

# ---------------------------------------------------
# Stage 2: Minimal runtime image
# ---------------------------------------------------
FROM scratch

WORKDIR /app

# Copy root CA certificates for TLS verification
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt 

# Copy the compiled binary
COPY --from=builder /app/kube-prober /app/kube-prober

# Run application
ENTRYPOINT ["/app/kube-prober"]
