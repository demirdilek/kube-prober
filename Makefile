-include .env
export

.PHONY: help lint test test-coverage k3d-up cache-test-images docker-build prometheus-install install-argocd apply-gitops bootstrap local-deploy clean k3d-down forward-all stop-forward argocd-pass hard-reset test-targets-enable test-targets-disable test-alert-error test-alert-latency test-alert-traffic test-alert-saturation test-alert-tcp test-alert-tls-expiry test-alert-tls-handshake test-alert-grpc test-alert-clean

.DEFAULT_GOAL := help

# Container registry configuration
IMAGE_REPO=ghcr.io/demirdilek/kube-prober
IMAGE_TAG=v0.1.0

# Argo CD & Helm variables
ARGO_APP=kube-prober
ARGO_NAMESPACE=argocd
ARGOCD_MANIFEST_URL ?= https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml

help: ## Show this help message
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-22s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# --- 1. QUALITY & TESTING ---

lint: ## Run golangci-lint or go vet for code quality
	@echo "==> Running linter..."
	@if command -v golangci-lint > /dev/null; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed, running go vet..."; \
		go vet ./...; \
	fi

test: ## Run unit and integration tests with race detection
	@echo "==> Running tests with race detector..."
	go test -v -race ./...

test-coverage: ## Run tests and generate HTML coverage report
	@echo "==> Generating test coverage..."
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# --- 2. BOOTSTRAP (Run once for setup) ---

bootstrap: k3d-up cache-test-images prometheus-install install-argocd apply-gitops ## Setup cluster, cache images, and deploy core infra
	@echo "========================================================="
	@echo " Kube Prober stack is fully up and running out-of-the-box! "
	@echo "========================================================="

k3d-up: 
	@if k3d cluster list | grep -q "mycluster"; then \
	echo "Cluster 'mycluster' already exists."; \
	else \
	k3d cluster create mycluster --api-port 6443 -p "80:80@loadbalancer" -p "443:443@loadbalancer" --agents 2; \
	fi

cache-test-images: 
	@echo "==> Pulling and caching external test images..."
	docker pull mccutchen/go-httpbin:v2.14.0
	k3d image import mccutchen/go-httpbin:v2.14.0 -c mycluster
	docker pull connectrpc/conformance:v1.1.6
	k3d image import connectrpc/conformance:v1.1.6 -c mycluster

prometheus-install: 
	@./scripts/deploy-prometheus.sh

install-argocd: 
	@echo "==> Installing Argo CD..."
	kubectl create namespace $(ARGO_NAMESPACE) || true
	kubectl apply -n $(ARGO_NAMESPACE) --server-side --force-conflicts -f $(ARGOCD_MANIFEST_URL)
	@echo "==> Waiting for Argo CD components to be ready..."
	kubectl wait --for=condition=available deployment/argocd-server -n $(ARGO_NAMESPACE) --timeout=300s

apply-gitops: 
	@echo "==> Registering Kube Prober Application in Argo CD..."
	kubectl apply -f deploy/argocd/kube-prober-app.yaml

# --- 3. INNER DEV LOOP (Run frequently during development) ---

docker-build: lint test 
	@echo "==> Building Docker image..."
	docker build -t $(IMAGE_REPO):$(IMAGE_TAG) .

local-deploy: docker-build ## Fast local rebuild and Argo CD deployment
	@echo "==> Importing local image into k3d cache..."
	k3d image import $(IMAGE_REPO):$(IMAGE_TAG) -c mycluster
	@echo "==> Updating image tag via Argo CD and forcing sync..."
	argocd app set $(ARGO_APP) --helm-set image.tag=$(IMAGE_TAG) --core
	argocd app sync $(ARGO_APP) --core
	@echo "==> Restarting pods to pull the newly imported image..."
	kubectl rollout restart deployment $(ARGO_APP)

# --- 4. SRE FAULT INJECTION & TESTS ---

test-targets-enable: ## Enable HTTP test targets via Argo CD
	argocd app set $(ARGO_APP) --helm-set httpbin.enabled=true --core
	argocd app sync $(ARGO_APP) --core

test-targets-disable: ## Disable HTTP test targets via Argo CD
	argocd app unset $(ARGO_APP) --helm-set httpbin.enabled --core
	argocd app sync $(ARGO_APP) --core

test-alert-error: ## Simulate High Error Rate (HTTP 500)
	@./scripts/alerts/trigger-error.sh

test-alert-latency: ## Simulate High Latency (/delay/2)
	@./scripts/alerts/trigger-latency.sh

test-alert-traffic: ## Simulate Traffic Collapse (scale to 0)
	@./scripts/alerts/trigger-traffic.sh

test-alert-saturation: ## Simulate Worker Capacity Saturation (WORKERS=2)
	@./scripts/alerts/trigger-saturation.sh

test-alert-tcp: ## Simulate Raw TCP Probing
	@./scripts/alerts/trigger-tcp.sh

test-alert-tls-expiry: ## Simulate TLS Certificate Expiry Alert
	@./scripts/alerts/trigger-tls.sh expiry

test-alert-tls-handshake: ## Simulate TLS Handshake Failure Alert
	@./scripts/alerts/trigger-tls.sh handshake

test-alert-grpc: ## Simulate gRPC NOT_SERVING Alert
	@./scripts/alerts/trigger-grpc.sh

test-alert-clean: ## Clean up all simulated alert targets and reset overrides
	@./scripts/alerts/cleanup-all.sh

# --- 5. OBSERVABILITY & UTILITIES ---

forward-all: ## Forward Argo CD, Prometheus & Grafana UIs for Mobile/Tailscale
	@./scripts/forward-all.sh

stop-forward: ## Stop background port-forwarding
	@pkill -f "kubectl port-forward" 2>/dev/null || true
	@rm -f .argo.pid .prom.pid .grafana.pid
	@echo "Stopped all port-forwards."

argocd-pass: ## Retrieve initial admin password for Argo CD UI
	@echo "==> Argo CD Initial Admin Password:"
	@kubectl -n argocd get secret argocd-initialadmin-secret -o jsonpath="{.data.password}" 2>/dev/null | base64 -d || echo "Initial secret deleted." ; echo""

clean: ## Clean up temporary build files
	rm -f coverage.out coverage.html .argo.pid .prom.pid .grafana.pid

k3d-down: ## Delete local k3d cluster
	k3d cluster delete mycluster || true

hard-reset: k3d-down clean bootstrap ## Deep clean cluster and rebuild stack fresh