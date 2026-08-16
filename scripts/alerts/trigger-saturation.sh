#!/usr/bin/env bash
set -euo pipefail

ACTION="${1:-apply}"

if [ "$ACTION" == "clean" ] || [ "$ACTION" == "delete" ]; then
    echo "==> [Saturation] Cleaning up simulated saturation targets..."
    kubectl delete statictarget -l test-type=saturation-hang -n default --ignore-not-found
    kubectl delete deployment httpbin-saturation -n default --ignore-not-found
    kubectl delete service httpbin-saturation -n default --ignore-not-found
    
    echo "==> Restoring prober worker pool capacity (WORKERS=50)..."
    kubectl set env deployment/kube-prober WORKERS=50 -n default
    kubectl rollout status deployment/kube-prober -n default --timeout=60s
    echo "==> Saturation scenario cleaned up."
    exit 0
fi

echo "==> [Saturation] Throttling kube-prober worker pool to WORKERS=2..."
kubectl set env deployment/kube-prober WORKERS=2 -n default
kubectl rollout status deployment/kube-prober -n default --timeout=60s

echo "==> [Saturation] Deploying slow HTTP server and blocking targets..."

kubectl apply -f - <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: httpbin-saturation
  namespace: default
  labels:
    app: httpbin-saturation
spec:
  replicas: 1
  selector:
    matchLabels:
      app: httpbin-saturation
  template:
    metadata:
      labels:
        app: httpbin-saturation
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
  name: httpbin-saturation
  namespace: default
spec:
  selector:
    app: httpbin-saturation
  ports:
  - name: http
    port: 8080
    targetPort: 8080
---
apiVersion: kube-prober.io/v1alpha1
kind: StaticTarget
metadata:
  name: saturation-hang-1
  namespace: default
  labels:
    test-type: saturation-hang
spec:
  address: http://httpbin-saturation.default.svc.cluster.local:8080/delay/10
  scheme: http
---
apiVersion: kube-prober.io/v1alpha1
kind: StaticTarget
metadata:
  name: saturation-hang-2
  namespace: default
  labels:
    test-type: saturation-hang
spec:
  address: http://httpbin-saturation.default.svc.cluster.local:8080/delay/10
  scheme: http
---
apiVersion: kube-prober.io/v1alpha1
kind: StaticTarget
metadata:
  name: saturation-hang-3
  namespace: default
  labels:
    test-type: saturation-hang
spec:
  address: http://httpbin-saturation.default.svc.cluster.local:8080/delay/10
  scheme: http
EOF

echo "==> Waiting for httpbin-saturation rollout..."
kubectl rollout status deployment/httpbin-saturation -n default --timeout=60s
echo "==> Targets active. Worker saturation is now locked at 100% (2/2 workers busy)."