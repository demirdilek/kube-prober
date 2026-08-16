#!/usr/bin/env bash
set -euo pipefail

ACTION="${1:-apply}"

if [ "$ACTION" == "clean" ] || [ "$ACTION" == "delete" ]; then
    echo "==> [TCP] Cleaning up simulated TCP target..."
    kubectl delete statictarget tcp-error-test -n default --ignore-not-found
    kubectl delete deployment httpbin-tcp -n default --ignore-not-found
    kubectl delete service httpbin-tcp -n default --ignore-not-found
    echo "==> Target tcp-error-test removed. Probing stopped."
    exit 0
fi

echo "==> [TCP] Deploying pod to simulate immediate TCP Connection Refused (RST)..."

kubectl apply -f - <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: httpbin-tcp
  namespace: default
  labels:
    app: httpbin-tcp
spec:
  replicas: 1
  selector:
    matchLabels:
      app: httpbin-tcp
  template:
    metadata:
      labels:
        app: httpbin-tcp
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
  name: httpbin-tcp
  namespace: default
spec:
  selector:
    app: httpbin-tcp
  ports:
  - name: closed-port
    port: 59999
    targetPort: 59999
---
apiVersion: kube-prober.io/v1alpha1
kind: StaticTarget
metadata:
  name: tcp-error-test
  namespace: default
spec:
  address: httpbin-tcp.default.svc.cluster.local:59999
  scheme: tcp
EOF

echo "==> Waiting for httpbin-tcp rollout..."
kubectl rollout status deployment/httpbin-tcp -n default --timeout=60s
echo "==> Target deployed. TCPConnectionRefused alert will trigger shortly."
echo "==> Run './trigger-tcp.sh clean' to remove the target."