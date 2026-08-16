#!/usr/bin/env bash
set -euo pipefail

ACTION="${1:-apply}"

if [ "$ACTION" == "clean" ] || [ "$ACTION" == "delete" ]; then
    echo "==> Cleaning up gRPC happy path target..."
    kubectl delete deployment grpc-server -n default --ignore-not-found
    kubectl delete service grpc-server -n default --ignore-not-found
    exit 0
fi

echo "==> Deploying gRPC health-check demo server..."

kubectl apply -f - <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: grpc-server
  namespace: default
  labels:
    app: grpc-server
spec:
  replicas: 1
  selector:
    matchLabels:
      app: grpc-server
  template:
    metadata:
      labels:
        app: grpc-server
    spec:
      containers:
      - name: server
        image: gcr.io/google-samples/microservices-demo/shippingservice:v0.8.0
        ports:
        - containerPort: 50051
        env:
        - name: PORT
          value: "50051"
---
apiVersion: v1
kind: Service
metadata:
  name: grpc-server
  namespace: default
  labels:
    app: grpc-server
    probe: "true"
  annotations:
    probe/scheme: "grpc"
    probe/path: ""
spec:
  selector:
    app: grpc-server
  ports:
  - name: grpc
    port: 50051
    targetPort: 50051
EOF

echo "==> Waiting for gRPC pod to become ready..."
kubectl rollout status deployment/grpc-server -n default --timeout=60s