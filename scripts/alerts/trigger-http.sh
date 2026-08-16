#!/usr/bin/env bash
set -euo pipefail

ACTION="${1:-apply}"

if [ "$ACTION" == "clean" ] || [ "$ACTION" == "delete" ]; then
    echo "==> [HTTP] Cleaning up simulated HTTP error target..."
    kubectl delete statictarget http-error-test -n default --ignore-not-found
    kubectl delete deployment httpbin-error -n default --ignore-not-found
    kubectl delete service httpbin-error -n default --ignore-not-found
    echo "==> Target http-error-test removed. Probing stopped."
    exit 0
fi

echo "==> [HTTP] Deploying local target to simulate HTTP 5xx server error..."

kubectl apply -f - <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: httpbin-error
  namespace: default
  labels:
    app: httpbin-error
spec:
  replicas: 1
  selector:
    matchLabels:
      app: httpbin-error
  template:
    metadata:
      labels:
        app: httpbin-error
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
  name: httpbin-error
  namespace: default
spec:
  selector:
    app: httpbin-error
  ports:
  - name: http
    port: 8080
    targetPort: 8080
---
apiVersion: kube-prober.io/v1alpha1
kind: StaticTarget
metadata:
  name: http-error-test
  namespace: default
spec:
  address: http://httpbin-error.default.svc.cluster.local:8080/status/500
  scheme: http
EOF

echo "==> Target http-error-test deployed. HTTPStatus5xx / HighErrorRate alerts will trigger shortly."
echo "==> Run './trigger-http.sh clean' to remove the target and recover."