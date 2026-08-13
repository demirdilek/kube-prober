-include .env
export

.PHONY: $(shell awk -F':' '/^[a-zA-Z0-9_-]+:/ {print $$1}' $(MAKEFILE_LIST))

.DEFAULT_GOAL := help

# Container registry configuration
IMAGE_REPO := ghcr.io/demirdilek/kube-prober
IMAGE_TAG := $(shell awk '/^appVersion:/ {print $$2}' helm/kube-prober/Chart.yaml | tr -d '"')

# Helm & Argo CD variables
RELEASE_NAME := kube-prober
CHART_DIR := ./helm/kube-prober
ARGO_APP := kube-prober
ARGO_NAMESPACE := argocd

help: ## Show this help message
	@echo "================================================================================"
	@echo "  kube-prober - Kubernetes-Native Probing & Observability Engine"
	@echo "================================================================================"
	@echo "  Goal:  Event-driven target discovery and multi-protocol health monitoring"
	@echo "         (HTTP, TCP, TLS, gRPC) exporting 4 Golden Signals to Prometheus."
	@echo ""
	@echo "  Usage: make <target>"
	@echo ""
	@echo "  Common Workflows:"
	@echo "    make bootstrap     Spin up local k3d cluster, Prometheus & Argo CD"
	@echo "    make local-deploy  Build local image, import to k3d, and restart pod"
	@echo "    make forward-all   Port-forward Argo CD (8080), Prometheus & Grafana"
	@echo "    make test-alert-*  Inject SRE fault scenarios (e.g. make test-alert-latency)"
	@echo "================================================================================"
	@echo ""
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-22s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# ---  QUALITY & TESTING ---

lint: ## Run Go, ShellCheck, and Helm linters
	@echo "==> Running Go linter..."
	@command -v golangci-lint >/dev/null 2>&1 && golangci-lint run ./... || go vet ./...
	@echo "==> Running ShellCheck..."
	@command -v shellcheck >/dev/null 2>&1 && shellcheck scripts/*.sh scripts/**/*.sh || echo "shellcheck not installed, skipping..."
	@echo "==> Running Helm lint..."
	@command -v helm >/dev/null 2>&1 && helm lint helm/kube-prober || echo "helm not installed, skipping..."

test: ## Run unit and integration tests with race detection
	@echo "==> Running tests with race detector..."
	go test -v -race ./...

test-coverage: ## Run tests and generate HTML coverage report
	@echo "==> Generating test coverage..."
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# ---  BOOTSTRAP (Run once for setup) ---

bootstrap: k3d-up cache-test-images prometheus-install install-argocd apply-gitops ## Setup cluster, cache images, and deploy core infra
	@echo "========================================================="
	@echo " kube-prober stack is fully up and running out-of-the-box! "
	@echo "========================================================="

k3d-up: ## Create local k3d cluster if it doesn't exist 
	@if k3d cluster list | grep -q "mycluster"; then \
		echo "Cluster 'mycluster' already exists."; \
	else \
		k3d cluster create mycluster --api-port 6443 -p "80:80@loadbalancer" -p "443:443@loadbalancer" --agents 2; \
	fi

k3d-down: ## Delete local k3d cluster
	k3d cluster delete mycluster || true

hard-reset: k3d-down clean bootstrap ## Deep clean cluster and rebuild stack fresh

clean: k3d-down ## Clean up temporary build files
	rm -f coverage.out coverage.html .argo.pid .prom.pid .grafana.pid

cache-test-images: 
	@echo "==> Pulling and caching external test images..."
	docker pull mccutchen/go-httpbin:v2.14.0
	k3d image import mccutchen/go-httpbin:v2.14.0 -c mycluster

prometheus-install: ## Install or upgrade Prometheus stack
	@./scripts/deploy-prometheus.sh

install-argocd: ## Install Argo CD
	@echo "==> Installing Argo CD..."
	kubectl create namespace $(ARGO_NAMESPACE) || true
	kubectl apply -n $(ARGO_NAMESPACE) --server-side --force-conflicts -f deploy/argocd/install-v3-5-1.yaml
	@echo "==> Waiting for Argo CD components to be ready..."
	kubectl wait --for=condition=available deployment/argocd-server -n $(ARGO_NAMESPACE) --timeout=300s
	kubectl wait --for=condition=available deployment/argocd-repo-server -n $(ARGO_NAMESPACE) --timeout=300s
	kubectl wait --for=condition=available deployment/argocd-applicationset-controller -n $(ARGO_NAMESPACE) --timeout=300s

apply-gitops: ## Register kube-prober Application in Argo CD
	@echo "==> Registering kube-prober Application in Argo CD..."
	kubectl apply -f deploy/argocd/kube-prober-app.yaml

# --- 3. INNER DEV LOOP (Run frequently during development) ---


local-deploy: argocd-local-enable ## Build local image, import to k3d, and restart deployment
	@echo "==> Building Docker image locally ($(IMAGE_TAG))..."
	docker build -t $(IMAGE_REPO):$(IMAGE_TAG) .
	@echo "==> Importing image into k3d cluster..."
	k3d image import $(IMAGE_REPO):$(IMAGE_TAG) -c mycluster
	@echo "==> Restarting deployment..."
	kubectl rollout restart deployment kube-prober

# ---  SRE FAULT INJECTION & TESTS ---

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

test-alert-dns: ## Simulate DNS Resolution Failure Alert
	@./scripts/alerts/trigger-dns.sh

test-dns-success: ## Simulate DNS Resolution Success (for testing recovery)
	@./scripts/alerts/trigger-dns-success.sh

test-alert-clean: ## Clean test alerts
	@./scripts/alerts/cleanup-all.sh

test-alert-all: test-targets-enable test-alert-error test-alert-latency test-alert-traffic test-alert-saturation test-alert-tcp test-alert-tls-expiry test-alert-tls-handshake test-alert-grpc test-alert-dns ## Run all alert tests

# --- OBSERVABILITY & UTILITIES ---

forward-all: ## Forward Argo CD, Prometheus & Grafana UIs for Mobile/Tailscale
	@./scripts/forward-all.sh

stop-forward: ## Stop background port-forwarding
	@pkill -f "kubectl port-forward" 2>/dev/null || true
	@rm -f .argo.pid .prom.pid .grafana.pid
	@echo "Stopped all port-forwards."

argocd-login: forward-all argocd-set-pass ## Login to Argo CD CLI
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

argocd-set-pass: ## Set a custom Argo CD admin password using the running pod
	@MYPASS="admin1234"; \
	echo "==> Updating Argo CD admin password..."; \
	HASH=$$(kubectl exec -n argocd deployment/argocd-server -- argocd account bcrypt --password "$$MYPASS"); \
	kubectl patch secret argocd-secret -n argocd -p "{\"stringData\": {\"admin.password\": \"$$HASH\", \"admin.passwordMtime\": \"$$(date -u +%FT%TZ)\"}}"; \
	echo "==> Password successfully updated to: $$MYPASS"