#!/usr/bin/env bash
set -euo pipefail

TAILSCALE_IP=$(tailscale ip -4 2>/dev/null || echo "localhost")

# Passwörter sicher auslesen (verhindert Abbruch durch pipefail)
ARGO_PASS=$(kubectl -n argocd get secret argocd-initialadmin-secret -o jsonpath="{.data.password}" 2>/dev/null | base64 -d 2>/dev/null || true)
if [ -z "$ARGO_PASS" ]; then
  ARGO_PASS="Not found / Custom Password"
fi

GRAFANA_PASS=$(kubectl -n default get secret prom-stack-grafana -o jsonpath="{.data.admin-password}" 2>/dev/null | base64 -d 2>/dev/null || true)
if [ -z "$GRAFANA_PASS" ]; then
  GRAFANA_PASS="admin"
fi

echo "========================================================"
echo " CONTROL PLANE & TELEMETRY (Tailscale / Local Access)"
echo "========================================================"
echo " ARGO CD:     https://${TAILSCALE_IP}:8080"
echo "   User:      admin"
echo "   Password:  ${ARGO_PASS}"
echo " "
echo "--------------------------------------------------------"
echo " PROMETHEUS:  http://${TAILSCALE_IP}:9090"
echo "--------------------------------------------------------"
echo " GRAFANA:     http://${TAILSCALE_IP}:3000"
echo "   User:      admin"
echo "   Password:  ${GRAFANA_PASS}"
echo " "
echo "--------------------------------------------------------"
echo " KUBE-PROBER: http://${TAILSCALE_IP}:8081/metrics"
echo "              http://${TAILSCALE_IP}:8081/healthz"
echo "              http://${TAILSCALE_IP}:8081/readyz"
echo "========================================================"
echo "==> Starting Port-Forwards in background..."

kubectl port-forward --address 0.0.0.0 -n argocd svc/argocd-server 8080:443 >/dev/null 2>&1 & echo $! > .argo.pid
kubectl port-forward --address 0.0.0.0 -n default svc/prom-stack-kube-prometheus-prometheus 9090:9090 >/dev/null 2>&1 & echo $! > .prom.pid
kubectl port-forward --address 0.0.0.0 -n default svc/prom-stack-grafana 3000:80 >/dev/null 2>&1 & echo $! > .grafana.pid
kubectl port-forward --address 0.0.0.0 -n default svc/kube-prober 8081:8080 >/dev/null 2>&1 & echo $! > .prober.pid

echo "==> Done! All services are forwarded."