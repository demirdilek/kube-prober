#!/usr/bin/env bash
set -euo pipefail

# `.env` file is optional, but if it exists, we will source it to get the SLACK_WEBHOOK_URL, PUSHOVER_API_TOKEN, and PUSHOVER_USER_KEY values.
if [ -f ".env" ]; then
  set -o allexport
  # shellcheck source=/dev/null
  source .env
  set +o allexport
fi

echo "==> Creating Alertmanager Webhook Secret directly from .env..."

# 1. Create the secret with the provided values from .env (or empty if not set)
kubectl create secret generic alertmanager-webhooks -n default \
  --from-literal=slack-url="${SLACK_WEBHOOK_URL:-}" \
  --from-literal=pushover-token="${PUSHOVER_API_TOKEN:-}" \
  --from-literal=pushover-user="${PUSHOVER_USER_KEY:-}" \
  --dry-run=client -o yaml | kubectl apply -f -

# 3. Helm Repo update & install Prometheus stack
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update

echo "==> Applying Prometheus stack..."

# 4. Install or upgrade the Prometheus stack with the provided values file
helm upgrade --install prom-stack prometheus-community/kube-prometheus-stack \
  -f prom-stack-values.yaml