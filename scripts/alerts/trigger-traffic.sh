#!/usr/bin/env bash
set -euo pipefail

echo "==> [Traffic] Deploying HTTP target with misrouted port to simulate Traffic Collapse..."

kubectl apply -f - <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: httpbin-traffic-test
  namespace: default
spec:
  replicas: 1
  selector:
    matchLabels:
      app: httpbin-traffic-test
  template:
    metadata:
      labels:
        app: httpbin-traffic-test
    spec:
      containers:
        - name: httpbin
          image: mccutchen/go-httpbin:v2.14.0
          imagePullPolicy: IfNotPresent
          ports:
            - containerPort: 8080
---
apiVersion: v1
kind: Service
metadata:
  name: httpbin-traffic-test
  namespace: default
  labels:
    probe: "true"
  annotations:
    probe/scheme: "http"
    probe/path: "/status/200"
spec:
  ports:
    - port: 80
      # Intentionally pointing to a dead port to simulate a total connection failure (Traffic Collapse)
      targetPort: 9999
      name: http
  selector:
    app: httpbin-traffic-test
EOF

echo "==> Target httpbin-traffic-test deployed. Alert TrafficCollapse will trigger shortly."