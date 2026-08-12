# ADR 0006: Concurrent Worker Pool and Channels Architecture

* **Status:** Accepted
* **Date:** 2026-07-18

## Context

The `Kube Prober` probing engine must periodically execute health checks against hundreds or thousands of discovered Kubernetes targets across multiple protocols (HTTP, TCP, TLS, gRPC).

The concurrent execution model needed to meet strict operational constraints:

* **Bounded Resource Consumption:** Prevent unbounded goroutine spawning under heavy target load, which could lead to CPU/memory exhaustion and OOM kills.
* **Non-Blocking Task Distribution:** Decouple target discovery and scheduling from actual network probe execution.
* **Backpressure Management:** Provide explicit queueing and saturation visibility to trigger capacity alerts when worker thresholds are reached.

## Decision

We decided to implement a fixed-capacity **Worker Pool pattern combined with Go Channels** (`chan Job`) for job distribution and synchronization.

## Options Considered

* **Spawning Goroutines Per Request (`go ProbeTarget()`):** *Rejected because* unbuffered goroutine creation under high dynamic load can easily overwhelm system memory and lead to uncontrolled network socket allocation.
* **External Job Queue (e.g., Redis / RabbitMQ):** *Rejected because* it adds heavy external operational dependencies and network overhead for an in-process, high-throughput microservice.
* **Worker Pool with Go Channels (Chosen):** *Selected because* Go's native channels provide zero-dependency, thread-safe, and highly efficient task distribution. Worker goroutines consume jobs concurrently while enforcing a strict upper bound on system resource usage.

## Consequences

### Positive

* **Predictable Memory & Threading:** The maximum number of concurrent workers is capped via configuration (`WORKERS` environment variable), preventing runaway resource consumption.
* **Decoupled Architecture:** `TargetScheduler` routines push probe tasks into the channel independently, while worker goroutines consume and execute them asynchronously.
* **Native Saturation Metrics:** Tracking active workers vs. total worker capacity directly feeds the **Saturation** golden signal (`kube_prober_saturation_active_workers`).
* **Clean Shutdown:** Channels integrate seamlessly with Go's `context.Context` and `sync.WaitGroup` to allow graceful drain of in-flight probes during `SIGTERM` shutdown events.

### Negative / Trade-offs

* **Queue Latency Risk:** If the worker pool is undersized for the target volume, jobs remain buffered in the channel, increasing overall probe interval latency.
* **Capacity Tuning Required:** Operators must tune `WORKERS` and `QUEUE_SIZE` based on total probe target counts and network timeout settings.
