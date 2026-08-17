#!/usr/bin/env bash
set -euo pipefail

echo "==> Cleaning up all simulated test targets, chaos scenarios, and integration workloads..."

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# 1. Alert-Trigger-Skripte im Unterordner alerts/ mit clean aufrufen
if [ -f "$DIR/alerts/trigger-all.sh" ]; then
    bash "$DIR/alerts/trigger-all.sh" clean
fi

# 2. Integration- & Discovery-Workloads bereinigen
if [ -f "$DIR/test-discovery.sh" ]; then
    bash "$DIR/test-discovery.sh" clean
fi

if [ -f "$DIR/test-grpc-happy.sh" ]; then
    bash "$DIR/test-grpc-happy.sh" clean
fi

# 3. Fallback: Alle statischen Test-Targets restlos entfernen
kubectl delete statictarget \
    dns-error-test \
    tls-handshake-test \
    tls-expiry-test \
    tls-error-test \
    tcp-error-test \
    http-error-test \
    grpc-error-test \
    latency-delay-test \
    grpc-happy-test \
    traffic-collapse-test \
    -n default --ignore-not-found

kubectl delete statictarget -l test-type=traffic-load -n default --ignore-not-found
kubectl delete statictarget -l test-type=saturation-hang -n default --ignore-not-found

# 4. Fallback: Alle Test-Deployments und Services restlos entfernen
kubectl delete deployment \
    discovery-demo \
    grpc-server \
    grpc-not-serving \
    httpbin-latency \
    httpbin-error \
    httpbin-tcp \
    httpbin-saturation \
    tls-expiry-server \
    traffic-collapse-service \
    -n default --ignore-not-found

kubectl delete service \
    discovery-demo \
    grpc-server \
    grpc-not-serving \
    httpbin-latency \
    httpbin-error \
    httpbin-tcp \
    httpbin-saturation \
    tls-expiry-server \
    traffic-collapse-service \
    -n default --ignore-not-found

# 5. Fallback: Temporäre ConfigMaps und TLS-Secrets bereinigen
kubectl delete configmap tls-expiry-nginx-conf -n default --ignore-not-found
kubectl delete secret tls-expiring-cert -n default --ignore-not-found

# 6. Fallback: Deployment-Umgebungsvariablen (Workers & TLS-Check) auf Standard zurücksetzen
if kubectl get deployment kube-prober -n default >/dev/null 2>&1; then
    echo "==> Restoring prober deployment environment defaults..."
    kubectl set env deployment/kube-prober WORKERS=50 -n default >/dev/null 2>&1 || true
fi

echo "==> All test targets, pods, and services removed. Metrics will normalize shortly."