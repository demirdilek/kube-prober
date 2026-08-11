#!/usr/bin/env bash
set -euo pipefail

# 1. `.env` laden, falls die Variablen nicht bereits in der Shell-Sitzung exportiert sind
if [ -f ".env" ]; then
  export $(grep -v '^#' .env | xargs)
fi

echo "==> Creating Alertmanager Webhook Secret directly from .env..."

# 2. Native K8s Secret anlegen (idempotent durch --dry-run=client)
kubectl create secret generic alertmanager-webhooks -n default \
  --from-literal=slack-url="${SLACK_WEBHOOK_URL:-}" \
  --from-literal=pushover-token="${PUSHOVER_API_TOKEN:-}" \
  --from-literal=pushover-user="${PUSHOVER_USER_KEY:-}" \
  --dry-run=client -o yaml | kubectl apply -f -

# 3. Helm Repo updaten
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update

echo "==> Applying Prometheus stack..."

# 4. Direkt die saubere Helm-Values anwenden (kein envsubst mehr nötig)
helm upgrade --install prom-stack prometheus-community/kube-prometheus-stack \
  -f prom-stack-values.yaml