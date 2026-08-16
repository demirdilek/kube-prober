#!/usr/bin/env bash
set -euo pipefail

ACTION="${1:-apply}"

if [ "$ACTION" == "clean" ] || [ "$ACTION" == "delete" ]; then
    echo "==> [Traffic Collapse] Cleaning up simulated collapsed traffic target..."
    kubectl delete statictarget traffic-collapse-test -n default --ignore-not-found
    kubectl delete deployment traffic-collapse-service -n default --ignore-not-found
    kubectl delete service traffic-collapse-service -n default --ignore-not-found
    echo "==> Target removed and baseline restored."
    exit 0
fi

echo "==> [Traffic Collapse] Deploying service scaled to 0 replicas to simulate total traffic failure..."

kubectl apply -f - <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: traffic-collapse-service
  namespace: default
  labels:
    app: traffic-collapse-service
spec:
  replicas: 0
  selector:
    matchLabels:
      app: traffic-collapse-service
  template:
    metadata:
      labels:
        app: traffic-collapse-service
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
  name: traffic-collapse-service
  namespace: default
spec:
  selector:
    app: traffic-collapse-service
  ports:
  - name: http
    port: 80
    targetPort: 80
---
apiVersion: kube-prober.io/v1alpha1
kind: StaticTarget
metadata:
  name: traffic-collapse-test
  namespace: default
spec:
  address: http://traffic-collapse-service.default.svc.cluster.local:80/
  scheme: http
EOF

echo "==> Target deployed. 100% of requests will fail, triggering TrafficCollapse alert in ~1 minute."
echo "==> Run './trigger-traffic.sh clean' to remove the target."