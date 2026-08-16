#!/usr/bin/env bash
set -euo pipefail

ACTION="${1:-apply}"

if [ "$ACTION" == "clean" ] || [ "$ACTION" == "delete" ]; then
    echo "==> Cleaning up dynamic discovery test workload..."
    kubectl delete deployment discovery-demo -n default --ignore-not-found
    kubectl delete service discovery-demo -n default --ignore-not-found
    exit 0
fi

echo "==> Deploying annotated workload (5 replicas) to test Informer & Rendezvous Sharding..."

kubectl apply -f - <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: discovery-demo
  namespace: default
  labels:
    app: discovery-demo
    probe: "true"
spec:
  replicas: 5
  selector:
    matchLabels:
      app: discovery-demo
  template:
    metadata:
      labels:
        app: discovery-demo
        probe: "true"
    spec:
      containers:
      - name: web
        image: nginx:alpine
        ports:
        - containerPort: 80
---
apiVersion: v1
kind: Service
metadata:
  name: discovery-demo
  namespace: default
  labels:
    app: discovery-demo
    probe: "true"
  annotations:
    probe/path: "/"
    probe/scheme: "http"
spec:
  selector:
    app: discovery-demo
  ports:
  - name: http
    port: 80
    targetPort: 80
EOF

echo "==> discovery-demo deployed. Check prober logs to verify EndpointSlice target distribution."