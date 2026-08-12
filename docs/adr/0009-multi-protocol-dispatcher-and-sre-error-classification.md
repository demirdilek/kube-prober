# ADR 0009: Multi-Protocol Dispatcher & SRE Error Classification

* **Status:** Accepted
* **Date:** 2026-07-21

## Context

A key goal of `Kube Prober` is to monitor diverse infrastructure and microservice endpoints across Kubernetes clusters. Target workloads rely on various application and transport-layer protocols, including HTTP/HTTPS, raw TCP sockets, TLS handshakes, and gRPC health checks.

In addition, raw network errors (e.g., `net.OpError`, `x509.UnknownAuthorityError`, or gRPC status codes) are often opaque, making root-cause analysis difficult during active incidents.

The system needed a mechanism to:

1. Dynamically route health checks to protocol-specific handlers based on URL schemes (`http://`, `tcp://`, `tls://`, `grpc://`).
2. Map low-level network and runtime errors into standardized SRE failure categories equipped with actionable diagnostic hints to lower Mean Time to Resolution (MTTR).

## Decision

We decided to implement a thread-safe **Multi-Protocol Dispatcher using function pointers** (`pkg/prober/dispatcher.go`) combined with a centralized **SRE Error Categorization Engine** (`pkg/prober/prober.go`).

## Options Considered

* **Hardcoded Protocol Branching (`switch/case` in worker loops):** *Rejected because* it tightly couples worker pool logic with specific protocol execution code, making it difficult to extend support for new protocols (e.g., DNS).
* **Interface-Based Polymorphism:** *Rejected because* registering function pointers (`ProbeFunc`) provides a simpler, zero-allocation, and lightweight mechanism for protocol routing.
* **Raw Error Exposing in Telemetry:** *Rejected because* surfacing raw OS error strings in Prometheus metrics leads to high metric cardinality, messy Grafana panels, and unclear diagnostic paths for operators.
* **Dispatcher with Categorized SRE Error Buckets (Chosen):** *Selected because* the `Dispatcher` allows dynamic registration of protocol handlers (`Register(scheme, ProbeFunc)`), while `MapToCategory` unwraps the error chain to classify failures into 9 distinct SRE categories (`dns_error`, `tls_error`, `grpc_not_serving`, `connection_refused`, etc.).

## Consequences

### Positive

* **Modular Protocol Extensibility:** Adding a new protocol handler (such as DNS resolution checks) requires only implementing a function matching the `ProbeFunc` signature and registering it with the `Dispatcher`.
* **Standardized Error Telemetry:** Failed probes populate the `kube_prober_errors_total{category="..."}` counter metric, feeding structured data directly into Grafana dashboards and Prometheus rules.
* **Actionable Operator Hints:** Each `ErrorCategory` attaches a diagnostic troubleshooting hint (`Hint()`) directly to structured `slog` output and static Prometheus info metrics (`kube_prober_error_category_hint_info`), accelerating incident triage.
* **Annotation-Driven Protocol Discovery:** The `KubeWatcher` informer reads custom service annotations (`probe/scheme`, `probe/path`) to automatically route probes through the correct protocol handler without manual prober configuration.

### Negative / Trade-offs

* **Unwrapped Error Overhead:** Classifying nested errors requires inspecting the error chain via `errors.As` and `errors.Is`, which incurs minimal runtime type-assertion overhead during error paths.
