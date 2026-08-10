#!/usr/bin/env bash
set -euo pipefail

echo "==> [Errors] Deploying HTTP 500 error target..."

kubectl apply -f - <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: httpbin-error
  namespace: default
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
          imagePullPolicy: IfNotPresent
          ports:
            - containerPort: 8080
---
apiVersion: v1
kind: Service
metadata:
  name: httpbin-error
  namespace: default
  labels:
    probe: "true"
  annotations:
    probe/scheme: "http"
    probe/path: "/status/500"
spec:
  ports:
    - port: 80
      targetPort: 8080
      name: http
  selector:
    app: httpbin-error
EOF

echo "==> Target httpbin-error deployed. Alert HighErrorRate will trigger shortly."