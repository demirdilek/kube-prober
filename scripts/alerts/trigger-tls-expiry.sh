#!/usr/bin/env bash
set -euo pipefail

ACTION="${1:-apply}"

if [ "$ACTION" == "clean" ] || [ "$ACTION" == "delete" ]; then
    echo "==> [TLS Expiry] Cleaning up simulated expiring TLS workload..."
    kubectl delete statictarget tls-expiry-test -n default --ignore-not-found
    kubectl delete deployment tls-expiry-server -n default --ignore-not-found
    kubectl delete service tls-expiry-server -n default --ignore-not-found
    kubectl delete configmap tls-expiry-nginx-conf -n default --ignore-not-found
    kubectl delete secret tls-expiring-cert -n default --ignore-not-found
    echo "==> Cleanup complete."
    exit 0
fi

echo "==> [TLS Expiry] Generating TLS certificate with 5 days validity..."
TEMP_DIR=$(mktemp -d)

# 1. Generate Test-CA
openssl req -x509 -newkey rsa:2048 -nodes -days 365 \
  -keyout "${TEMP_DIR}/ca.key" \
  -out "${TEMP_DIR}/ca.crt" \
  -subj "/CN=KubeProberTestCA" >/dev/null 2>&1

# 2. Server Certificate with 5-day validity
openssl req -newkey rsa:2048 -nodes \
  -keyout "${TEMP_DIR}/tls.key" \
  -out "${TEMP_DIR}/tls.csr" \
  -subj "/CN=tls-expiry-server.default.svc.cluster.local" >/dev/null 2>&1

cat <<EOF > "${TEMP_DIR}/ext.cnf"
subjectAltName = DNS:tls-expiry-server.default.svc.cluster.local,DNS:tls-expiry-server.default.svc,DNS:tls-expiry-server
EOF

openssl x509 -req -in "${TEMP_DIR}/tls.csr" \
  -CA "${TEMP_DIR}/ca.crt" -CAkey "${TEMP_DIR}/ca.key" -CAcreateserial \
  -out "${TEMP_DIR}/tls.crt" -days 5 -extfile "${TEMP_DIR}/ext.cnf" >/dev/null 2>&1

# 3. Create Secret
kubectl create secret tls tls-expiring-cert \
  --cert="${TEMP_DIR}/tls.crt" \
  --key="${TEMP_DIR}/tls.key" \
  -n default --dry-run=client -o yaml | kubectl apply -f -

rm -rf "${TEMP_DIR}"

echo "==> [TLS Expiry] Deploying HTTPS mock server..."

kubectl apply -f - <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: tls-expiry-nginx-conf
  namespace: default
data:
  default.conf: |
    server {
        listen 8443 ssl;
        server_name tls-expiry-server.default.svc.cluster.local;
        ssl_certificate /etc/ssl/certs/tls.crt;
        ssl_certificate_key /etc/ssl/certs/tls.key;
        location / {
            return 200 'TLS Expiry OK\n';
            add_header Content-Type text/plain;
        }
    }
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: tls-expiry-server
  namespace: default
  labels:
    app: tls-expiry-server
spec:
  replicas: 1
  selector:
    matchLabels:
      app: tls-expiry-server
  template:
    metadata:
      labels:
        app: tls-expiry-server
    spec:
      containers:
      - name: nginx
        image: nginx:alpine
        ports:
        - containerPort: 8443
        volumeMounts:
        - name: certs
          mountPath: /etc/ssl/certs
          readOnly: true
        - name: conf
          mountPath: /etc/nginx/conf.d
          readOnly: true
      volumes:
      - name: certs
        secret:
          secretName: tls-expiring-cert
      - name: conf
        configMap:
          name: tls-expiry-nginx-conf
---
apiVersion: v1
kind: Service
metadata:
  name: tls-expiry-server
  namespace: default
spec:
  selector:
    app: tls-expiry-server
  ports:
  - name: https
    port: 8443
    targetPort: 8443
---
apiVersion: kube-prober.io/v1alpha1
kind: StaticTarget
metadata:
  name: tls-expiry-test
  namespace: default
spec:
  address: tls-expiry-server.default.svc.cluster.local:8443
  scheme: tls
  insecureSkipVerify: true
EOF

echo "==> Waiting for tls-expiry-server rollout..."
kubectl rollout status deployment/tls-expiry-server -n default --timeout=60s
echo "==> Target deployed. Gauge kube_prober_tls_cert_expiry_days is now ~5 days and will trigger TLSCertificateExpiringSoon."