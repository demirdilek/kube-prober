-include .env
export

.PHONY: help lint test test-coverage k3d-up cache-test-images prometheus-install install-argocd apply-gitops bootstrap local-deploy clean k3d-down forward-all stop-forward argocd-pass argocd-set-pass hard-reset test-targets-enable test-targets-disable test-alert-error test-alert-latency test-alert-traffic test-alert-saturation test-alert-tcp test-alert-tls-expiry test-alert-tls-handshake test-alert-grpc test-alert-clean

.DEFAULT_GOAL := help

# Container registry configuration
IMAGE_REPO=ghcr.io/demirdilek/kube-prober
IMAGE_TAG=v0.1.0

# Helm & Argo CD variables
RELEASE_NAME=kube-prober
CHART_DIR=./helm/kube-prober
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
	docker pull stefanprodan/podinfo:6.6.0
	k3d image import mccutchen/go-httpbin:v2.14.0 connectrpc/conformance:v6.6.0 -c mycluster

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


local-deploy: argocd-local-enable
	@echo "==> Building Docker image locally ($(IMAGE_TAG))..."
	docker build -t $(IMAGE_REPO):$(IMAGE_TAG) .
	@echo "==> Importing image into k3d cluster..."
	k3d image import $(IMAGE_REPO):$(IMAGE_TAG) -c mycluster
	@echo "==> Restarting deployment..."
	kubectl rollout restart deployment kube-prober



# --- 4. SRE FAULT INJECTION & TESTS ---

test-targets-enable: ## Scale up test targets to simulate traffic and latency
	@echo "Enabling test targets (httpbin)..."
	kubectl scale deployment httpbin --replicas=1 -n default 2>/dev/null || true

test-targets-disable: ## Scale down test targets to 0 replicas (clean baseline)
	@echo "Disabling test targets to avoid resource usage..."
	kubectl scale deployment httpbin --replicas=0 -n default 2>/dev/null || true

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

argocd-login: ## Login to Argo CD CLI
	@argocd login localhost:8080 --username admin --insecure

argocd-local-enable: ## Pausing Argo CD auto-sync to avoid ans sync with local changes...
	@echo "==> Pausing Argo CD auto-sync to avoid ans sync with local changes..."
	@argocd app set kube-prober --sync-policy none
	@argocd app sync kube-prober --local ./helm/kube-prober/

argocd-local-disable: ## Re-enable Argo CD auto-sync after local changes are done
	@echo "==> Re-enabling Argo CD auto-sync..."
	@argocd app set kube-prober --sync-policy automated

argocd-pass: ## Retrieve initial admin password for Argo CD UI
	@echo "==> Argo CD Initial Admin Password:"
	@kubectl -n argocd get secret argocd-initialadmin-secret -o jsonpath="{.data.password}" 2>/dev/null | base64 -d || echo "Initial secret deleted." ; echo""

argocd-set-pass: ## Set a custom Argo CD admin password
	@MYPASS="admin1234"; \
	echo "==> Updating Argo CD admin password..."; \
	HASH=$$(docker run --rm quay.io/argoproj/argocd:latest argocd account bcrypt --password "$$MYPASS"); \
	kubectl patch secret argocd-secret -n argocd -p "{\"stringData\": {\"admin.password\": \"$$HASH\", \"admin.passwordMtime\": \"$$(date -u +%FT%TZ)\"}}"; \
	echo "==> Password successfully updated to: $$MYPASS"

clean: ## Clean up temporary build files
	rm -f coverage.out coverage.html .argo.pid .prom.pid .grafana.pid

k3d-down: ## Delete local k3d cluster
	k3d cluster delete mycluster || true

hard-reset: k3d-down clean bootstrap ## Deep clean cluster and rebuild stack fresh
