# ADR 0016: Informer-Based Endpoint Discovery via Kubernetes API

* **Status:** Accepted
* **Date:** 2026-07-26

## Context

`Kube Prober` must dynamically detect probe targets across Kubernetes cluster namespaces without requiring manual target lists or static configuration updates.

Traditional approaches to Kubernetes service discovery present distinct trade-offs:

* **Polling the API Server:** Periodically querying the Kubernetes API (`client-go` `List`) creates heavy API server load, incurs network latency, and introduces delays in target updates.
* **Legacy `Endpoints` API:** Watching standard `Endpoints` resources leads to high control-plane overhead in large clusters due to monolithic object updates whenever a single pod status changes.

The service discovery architecture needed an event-driven mechanism to observe active target workloads while maintaining low API server overhead and ensuring instant metric cleanup when targets are removed.

## Decision

We decided to implement target discovery using the **Kubernetes `EndpointSlice` Informer pattern** (`pkg/watcher/watcher.go`) combined with annotation-driven target filtering (`probe/enabled`, `probe/scheme`, `probe/path`).

## Options Considered

* **Static Target Files / ConfigMaps:** *Rejected because* manual file updates do not support cloud-native auto-scaling or dynamic pod lifecycles.
* **Periodic API Polling (`client.CoreV1().Pods().List(...)`):** *Rejected because* polling scales poorly ($O(N)$ requests to kube-apiserver), increases CPU/network overhead, and delays detection of microservice state changes.
* **`Endpoints` API Informer:** *Rejected because* the legacy `Endpoints` API is being phased out in favor of `EndpointSlice`, which breaks endpoints into smaller, scalable chunks to reduce apiserver traffic in large clusters.
* **`EndpointSlice` SharedInformer (Chosen):** *Selected because* `SharedInformerFactory` establishes a single persistent watch HTTP connection to the API server, maintaining an in-memory cache (`Indexer`) and notifying `KubeWatcher` via event handlers (`OnAdd`, `OnUpdate`, `OnDelete`).

## Consequences

### Positive

* **Real-Time Event-Driven Updates:** Target changes (pod additions, deletions, or annotation updates) are detected in real time without continuous API server polling.
* **Low API Server Impact:** Uses local in-memory caching provided by `client-go` shared informers, minimizing control-plane bandwidth and CPU load.
* **Automatic Metric Cleanup (`DeleteTargetMetrics`):** When a service target is deleted or scaled down, the `OnDelete` handler immediately purges stale time-series metrics from the Prometheus registry, preventing metric cardinality bloat.
* **Zero-Touch Target Onboarding:** Developers enable health probing simply by adding annotations (`probe/enabled: "true"`) to their Kubernetes Services or EndpointSlices.

### Negative / Trade-offs

* **RBAC Permissions Required:** Requires `ClusterRole` permissions to `watch` and `list` `discovery.k8s.io/v1` `endpointslices` across namespaces.
* **Initial Cache Sync Delay:** On startup, the prober engine must wait for `informerFactory.WaitForCacheSync()` to ensure local caches are fully populated before scheduling probes
