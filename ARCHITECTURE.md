# Architecture Overview: retail cell

This document describes the end-to-end workflow of the **retail cell** architecture. It outlines the path from system startup through dynamic target discovery to the collection of the 4 Golden Signals (SRE).

## 1. Boot & Setup (`main.go`)

* The system reads the environment variables (e.g., `WORKERS`, `PROBE_INTERVAL_SECONDS`).
* The `Dispatcher` is initialized, and all protocol handlers (HTTP, TCP, TLS, gRPC, DNS) are registered.
* The central worker pool starts with the configured number of asynchronous goroutines, waiting non-blockingly for work.
* The internal HTTP server (`:8080`) starts to expose metrics via `/metrics` for Prometheus.

## 2. Auto-Discovery (`informer.go`)

* The `KubeWatcher` listens event-based and resource-efficiently to Kubernetes `EndpointSlices`.
* As soon as a new target with the label `probe: "true"` appears, the watcher reads the protocol configuration (e.g., `probe/scheme: dns`) blazingly fast from a local RAM cache.

## 3. Sharding & Ownership (`registry.go`)

* To prevent redundant testing when prober pods are scaled, **Rendezvous Hashing** is applied.
* Each pod autonomously calculates a hash using the target URL and its own IP address.
* Only the pod with the highest hash value assumes exclusive responsibility for this target and routes it into the internal processing pipeline.

## 4. Scheduling (`worker.go`)

* The event loop registers the assigned target and starts a dedicated `TargetScheduler`.
* The scheduler wakes up strictly according to the configured interval and pushes a `Job` with the target URL into the central work channel (`jobs`).

## 5. Execution (`dispatcher.go` & `worker.go`)

* An available worker from the pool grabs the job from the channel.
* The worker passes the target URL to the `Dispatcher`.
* The dispatcher analyzes the URL scheme and routes the request via dynamic function pointers to the correct prober.
* The prober performs the network check and returns either an empty string (success) or a standardized SRE error category.

## 6. Telemetry: The 4 Golden Signals (`metrics.go`)

Right after execution, the worker logs the result for Prometheus:

* **Saturation:** The utilization is incremented at the start of the worker and immediately decremented at the end.
* **Traffic:** The request counter (`kube_prober_traffic_total`) for this target is increased by 1.
* **Latency:** The exact duration of the network call is measured and recorded in the histogram (`kube_prober_latency_seconds`).
* **Errors:** If an error occurs, the corresponding category counter (`kube_prober_errors_total`) is incremented, and a structured log entry with a diagnostic hint is generated.
