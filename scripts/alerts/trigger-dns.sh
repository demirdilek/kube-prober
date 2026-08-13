#!/usr/bin/env bash
set -euo pipefail

echo "==> [DNS] Deploying target to simulate DNS NXDOMAIN error..."

kubectl apply -f - <<EOF
apiVersion: v1
kind: Service
metadata:
  name: dns-error-test
  namespace: default
  labels:
    probe: "true"
  annotations:
    probe/scheme: "dns"
    # Ask a real DNS server for a fake domain to force a true *net.DNSError
    probe/path: "/this-domain-does-not-exist.internal?type=A"
spec:
  ports:
    - port: 53
      name: dns
---
apiVersion: discovery.k8s.io/v1
kind: EndpointSlice
metadata:
  name: dns-error-test-slice
  namespace: default
  labels:
    kubernetes.io/service-name: dns-error-test
    probe: "true"
addressType: IPv4
endpoints:
  - addresses:
      - "8.8.8.8"
ports:
  - port: 53
    name: dns
EOF

echo "==> Target dns-error-test deployed. DNSResolutionFailed alert will trigger shortly."