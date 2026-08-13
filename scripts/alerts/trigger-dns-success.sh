#!/usr/bin/env bash
set -euo pipefail

echo "==> [DNS] Deploying healthy target to simulate successful DNS resolution..."

# Create a headless service and manually bind an EndpointSlice to Google DNS (8.8.8.8)
kubectl apply -f - <<EOF
apiVersion: v1
kind: Service
metadata:
  name: dns-success
  namespace: default
  labels:
    probe: "true"
  annotations:
    probe/scheme: "dns"
    # Resolve google.com using A record
    probe/path: "/google.com?type=A"
spec:
  ports:
    - port: 53
      name: dns
  # Omit selector so K8s does not auto-generate endpoints
---
apiVersion: discovery.k8s.io/v1
kind: EndpointSlice
metadata:
  name: dns-success-slice
  namespace: default
  labels:
    kubernetes.io/service-name: dns-success
    probe: "true"
addressType: IPv4
endpoints:
  - addresses:
      - "8.8.8.8"
ports:
  - port: 53
    name: dns
EOF

echo "==> Target dns-success deployed. Baseline metrics will populate without triggering alerts."