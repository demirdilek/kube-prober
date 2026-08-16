#!/usr/bin/env bash
set -euo pipefail

ACTION="${1:-apply}"

if [ "$ACTION" == "clean" ] || [ "$ACTION" == "delete" ]; then
    echo "==> [TLS Handshake] Cleaning up simulated TLS handshake failure target..."
    kubectl delete statictarget tls-handshake-test -n default --ignore-not-found
    echo "==> Target tls-handshake-test removed. Probing stopped."
    exit 0
fi

echo "==> [TLS Handshake] Deploying target to simulate TLS handshake / certificate validation failure..."

kubectl apply -f - <<EOF
apiVersion: kube-prober.io/v1alpha1
kind: StaticTarget
metadata:
  name: tls-handshake-test
  namespace: default
spec:
  address: wrong.host.badssl.com:443
  scheme: tls
EOF

echo "==> Target tls-handshake-test deployed. TLSHandshakeFailed alert will trigger shortly."
echo "==> Run './trigger-tls-handshake.sh clean' to remove the target and recover."