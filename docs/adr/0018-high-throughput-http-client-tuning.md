# ADR 0018: High-Throughput HTTP Client Tuning and Connection Pooling

* **Status:** Accepted
* **Date:** 2026-07-28

## Context

`Kube Prober` executes thousands of concurrent HTTP probes against target services across the cluster. Using the default Go HTTP client under such high concurrency can lead to severe operational issues.

Specifically, the default connection pool settings limit persistent connections, causing the prober to constantly open and close TCP sockets. This results in massive TLS handshake overhead and leaves thousands of sockets in the `TIME_WAIT` state, eventually leading to ephemeral port exhaustion (socket starvation) on the worker node.

## Decision

We decided to explicitly tune the Go `http.Transport` settings for aggressive connection reuse within the application's entry point (`main.go`).

## Options Considered

* **Go Default `http.Client`:** *Rejected because* the default `MaxIdleConnsPerHost` is strictly limited to 2. For a probing engine hitting the same targets repeatedly, this causes massive connection churn and network stack overload.
* **Tuned `http.Transport` (Chosen):** *Selected because* explicit limits (`MaxIdleConns: 1000`, `MaxIdleConnsPerHost: 100`, `IdleConnTimeout: 90s`) ensure persistent TCP connections are kept alive and reused across multiple probe intervals.

## Consequences

### Positive

* **Socket Exhaustion Prevention:** Drastically reduces the number of ephemeral ports stuck in the `TIME_WAIT` state by effectively reusing active TCP connections.
* **Reduced Latency Jitter:** Bypassing repetitive TCP 3-way handshakes and TLS negotiations makes latency measurements more stable and accurate for the actual application layer.
* **Parameterization:** Connection limits and timeouts are dynamically configurable via environment variables (`MAX_IDLE_CONNS`, `MAX_IDLE_CONNS_PER_HOST`).

### Negative / Trade-offs

* **Idle Memory Overhead:** Keeping up to 1000 idle connections open consumes a minor amount of baseline system memory in the container.
* **Stale Connection Handling:** If an upstream target aggressively drops idle connections, the prober might encounter intermittent `EOF` errors on idle sockets, though the Go `http.Client` typically handles retries on idempotent requests automatically.
