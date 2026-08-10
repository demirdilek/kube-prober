#!/usr/bin/env bash
set -euo pipefail

# Add and update Helm repository
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update

echo "==> Applying Prometheus stack with in-memory secret injection..."

# Pipe the rendered YAML directly into helm. 
# The '-f -' flag tells helm to read the values from standard input.
envsubst < prom-stack-values.local.yaml | helm upgrade --install prom-stack prometheus-community/kube-prometheus-stack \
  -f prom-stack-values.yaml \
  -f -