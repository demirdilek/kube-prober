#!/usr/bin/env bash
set -euo pipefail

ACTION="${1:-apply}"

if [ "$ACTION" == "clean" ] || [ "$ACTION" == "delete" ]; then
    echo "==> [Latency] Cleaning up simulated slow target..."
    kubectl delete statictarget latency-delay-test -n default --ignore-not-found
    kubectl delete deployment httpbin-latency -n default --ignore-not-found
    kubectl delete service httpbin-latency -n default --ignore-not-found
    echo "==> Target latency-delay-test removed. Probing stopped."
    exit 0
fi

echo "==> [Latency] Deploying local httpbin to simulate high response latency..."

kubectl apply -f - <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: httpbin-latency
  namespace: default
  labels:
    app: httpbin-latency
spec:
  replicas: 1
  selector:
    matchLabels:
      app: httpbin-latency
  template:
    metadata:
      labels:
        app: httpbin-latency
    spec:
      containers:
      - name: httpbin
        image: mccutchen/go-httpbin:v2.14.0
        ports:
        - containerPort: 8080
---
apiVersion: v1
kind: Service
metadata:
  name: httpbin-latency
  namespace: default
spec:
  selector:
    app: httpbin-latency
  ports:
  - name: http
    port: 8080
    targetPort: 8080
---
apiVersion: kube-prober.io/v1alpha1
kind: StaticTarget
metadata:
  name: latency-delay-test
  namespace: default
spec:
  address: http://httpbin-latency.default.svc.cluster.local:8080/delay/2
  scheme: http
EOF

echo "==> Waiting for local httpbin rollout..."
kubectl rollout status deployment/httpbin-latency -n default --timeout=60s
echo "==> Target deployed. HighLatency (p95/p99) alert will trigger reliably without external rate limits."