#!/usr/bin/env bash
set -euo pipefail

ACTION="${1:-apply}"

if [ "$ACTION" == "clean" ] || [ "$ACTION" == "delete" ]; then
    echo "==> [gRPC] Cleaning up simulated gRPC not-serving target..."
    kubectl delete statictarget grpc-error-test -n default --ignore-not-found
    sleep 1
    kubectl delete service grpc-not-serving -n default --ignore-not-found
    kubectl delete deployment grpc-not-serving -n default --ignore-not-found
    echo "==> Cleanup complete. Probing stopped."
    exit 0
fi

echo "==> [gRPC] Deploying reliable gRPC test server..."

kubectl apply -f - <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: grpc-not-serving
  namespace: default
  labels:
    app: grpc-not-serving
spec:
  replicas: 1
  selector:
    matchLabels:
      app: grpc-not-serving
  template:
    metadata:
      labels:
        app: grpc-not-serving
    spec:
      containers:
      - name: server
        image: registry.k8s.io/e2e-test-images/agnhost:2.45
        args: ["grpc-health-checking", "--port", "50051"]
        ports:
        - containerPort: 50051
---
apiVersion: v1
kind: Service
metadata:
  name: grpc-not-serving
  namespace: default
spec:
  selector:
    app: grpc-not-serving
  ports:
  - name: grpc
    port: 50051
    targetPort: 50051
---
apiVersion: kube-prober.io/v1alpha1
kind: StaticTarget
metadata:
  name: grpc-error-test
  namespace: default
spec:
  address: grpc-not-serving.default.svc.cluster.local:50051/UnregisteredService
  scheme: grpc
EOF

echo "==> Waiting for pod rollout..."
kubectl rollout status deployment/grpc-not-serving -n default --timeout=60s
echo "==> Target deployed. gRPC health check will now reach the port and evaluate status."