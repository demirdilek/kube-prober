#!/usr/bin/env bash
set -euo pipefail

ACTION="${1:-apply}"

if [ "$ACTION" == "clean" ] || [ "$ACTION" == "delete" ]; then
    echo "==> [DNS] Cleaning up simulated DNS NXDOMAIN target..."
    kubectl delete statictarget dns-error-test -n default --ignore-not-found
    echo "==> Target dns-error-test removed. Probing stopped."
    exit 0
fi

echo "==> [DNS] Deploying target to simulate DNS NXDOMAIN error..."

kubectl apply -f - <<EOF
apiVersion: kube-prober.io/v1alpha1
kind: StaticTarget
metadata:
  name: dns-error-test
  namespace: default
spec:
  address: this-domain-does-not-exist.internal
  scheme: dns
EOF

echo "==> Target dns-error-test deployed. DNSResolutionFailed alert will trigger shortly."
echo "==> Run './trigger-dns.sh clean' to remove the target and recover."