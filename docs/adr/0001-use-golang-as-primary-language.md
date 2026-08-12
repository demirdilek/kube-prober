# ADR 0001: Use Go as primary language

* **Status:** Accepted
* **Date:** 2026-07-13

## Context

For the `Kube Prober` project, we needed to select a core programming language to build a high-performance, Kubernetes-native control plane and probing engine. The system requires concurrent multi-protocol health checks (HTTP, TCP, TLS, gRPC), event-driven target discovery via Kubernetes `EndpointSlices`, and high-throughput Prometheus metric generation for the **4 Golden Signals** (Latency, Traffic, Errors, Saturation).

The chosen language had to fulfill several strict architectural constraints:

* **Cloud-Native Ecosystem & Kubernetes Integration:** Seamless first-class support for `client-go`, informers, and structured API interactions.
* **Concurrency Model:** Efficient handling of thousands of concurrent, lightweight asynchronous probe routines without exhausting system resources.
* **Performance & Footprint:** Low memory overhead, fast startup times, and deterministic execution speed suitable for minimalist container runtimes (`scratch`).

## Decision

We have selected **Go (Golang)** as the primary programming language for the entire backend architecture.

## Options Considered

* **Python:** *Rejected because* of its higher memory footprint, slower execution speeds under extreme concurrency, and the overhead of the Global Interpreter Lock (GIL) when managing thousands of parallel asynchronous network probes. High dependency on runtime packages and potential collisions with Ubuntu system environments.
* **Node.js (TypeScript):** *Rejected because* of event-loop blocking risks during intensive crypto/TLS handshakes, dynamic memory allocation overhead, and less mature direct bindings with low-level Kubernetes operators compared to the Go ecosystem.
* **Rust:** *Rejected because* of a significantly steeper learning curve, longer compilation times, and slower initial development velocity, despite offering superior memory safety and raw performance.
* **Go (Chosen):** *Selected because* it provides first-class Kubernetes integration (`client-go`), an exceptionally lightweight concurrency model via goroutines and channels, simple static compilation into minimalist container images (`scratch`), and built-in enterprise-grade tooling for Prometheus metrics instrumentation.

## Consequences

### Positive

* **Native Ecosystem Compatibility:** Flawless integration with Kubernetes client libraries (`client-go`, `informers`, `discoveryv1`) for event-driven target synchronization.
* **Low Resource Footprint:** Extremely lightweight runtime and small memory allocations, allowing the service to run efficiently inside restricted container environments.
* **High Concurrency Performance:** Goroutines and robust channel primitives make parallel probing of hundreds of endpoints trivial and performant.
* **Simplified Operations:** Compiles into a single statically linked binary, allowing deployment on minimal base images like `scratch`.

### Negative / Trade-offs

* **Manual Error Handling:** Verbose error handling patterns (`if err != nil`) require strict coding conventions.
* **Garbage Collection Overhead:** Standard Go GC, while highly optimized, can occasionally introduce minor latency jitter compared to manual memory management or zero-allocation languages like Rust.
