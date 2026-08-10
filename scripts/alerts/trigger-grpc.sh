#!/usr/bin/env bash
set -euo pipefail

echo "==> [gRPC] Deploying a fake Nginx gRPC server to force a clean 'Unimplemented' error..."

kubectl apply -f - <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: grpc-test-nginx-config
  namespace: default
data:
  default.conf: |
    server {
        # Strictly enable HTTP/2 for gRPC compatibility
        listen 9999 http2;
        server_name _;
        
        location / {
            # Forge a valid gRPC "Trailers-Only" error response
            add_header Content-Type "application/grpc" always;
            add_header grpc-status 12 always; # Code 12 = Unimplemented
            add_header grpc-message "forced-grpc-error" always;
            return 204; # 204 No Content ensures empty body
        }
    }
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: grpc-test
  namespace: default
spec:
  replicas: 1
  selector:
    matchLabels:
      app: grpc-test
  template:
    metadata:
      labels:
        app: grpc-test
    spec:
      containers:
      - name: nginx
        image: nginx:alpine
        ports:
        - containerPort: 9999
        volumeMounts:
        - name: config
          mountPath: /etc/nginx/conf.d
          readOnly: true
      volumes:
      - name: config
        configMap:
          name: grpc-test-nginx-config
---
apiVersion: v1
kind: Service
metadata:
  name: grpc-test
  namespace: default
  labels:
    probe: "true"
  annotations:
    probe/scheme: "grpc"
    probe/path: "Health"
spec:
  ports:
  - port: 50051
    targetPort: 9999
    name: grpc
  selector:
    app: grpc-test
EOF

echo "==> Target grpc-test deployed. The Nginx server will return a clean gRPC error (Code 12)."
echo "==> The gRPCServiceNotServing alert will trigger shortly."