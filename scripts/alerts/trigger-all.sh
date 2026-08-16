#!/usr/bin/env bash
set -euo pipefail

ACTION="${1:-apply}"

SCRIPTS=(
    "trigger-dns.sh"
    "trigger-tls-handshake.sh"
    "trigger-tls-expiry.sh"
    "trigger-tcp.sh"
    "trigger-http.sh"
    "trigger-grpc.sh"
    "trigger-latency.sh"
    "trigger-traffic.sh"
    "trigger-saturation.sh"
)

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

for script in "${SCRIPTS[@]}"; do
    if [ -f "$DIR/$script" ]; then
        bash "$DIR/$script" "$ACTION"
    fi
done