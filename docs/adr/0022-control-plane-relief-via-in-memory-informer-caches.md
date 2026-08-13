# ADR 0022: Control-Plane Relief via In-Memory Informer Caches

* **Status:** Accepted
* **Date:** 2026-08-12

## Context

When dynamically discovering probe targets, the `KubeWatcher` listens to `EndpointSlice` events. However, the protocol routing instructions (custom annotations like `probe/scheme` and `probe/path`) reside on the parent `Service` object, not the `EndpointSlice` itself.

To resolve these routing instructions, the watcher must map the slice's `kubernetes.io/service-name` label back to the original Service. If the application performs a direct live API call (`clientset.CoreV1().Services().Get(...)`) for every single EndpointSlice addition or update, it would bombard the Kubernetes API Server with requests, leading to rate-limiting and control-plane degradation in large clusters.

## Decision

We decided to utilize a **Kubernetes `ServiceLister` backed by a `SharedInformer`** within `pkg/prober/informer.go` to perform annotation lookups strictly against a synchronized local in-memory RAM cache.

## Options Considered

* **Live API Server Polling (`clientset.Get`):** *Rejected because* it creates heavy $O(N)$ API traffic during cluster scaling events and introduces network latency into the event-handling loop.
* **Duplicating Annotations:** *Rejected because* forcing users to apply annotations to both the Service and every EndpointSlice manually breaks developer experience (DX).
* **Local In-Memory Cache via `ServiceLister` (Chosen):** *Selected because* the `SharedInformerFactory` automatically maintains a real-time, synchronized RAM cache of all Services in the background. Calling `svcLister.Services(namespace).Get(name)` performs a virtually instantaneous memory lookup with zero network overhead.

## Consequences

### Positive

* **API Server Protection:** Complete elimination of network calls to the Kubernetes API server during the target resolution phase.
* **Ultra-Fast Execution:** Event handlers process target updates in microseconds by querying local RAM.
* **High Scalability:** The application can absorb massive microservice scaling bursts (thousands of EndpointSlices) without degrading cluster performance.

### Negative / Trade-offs

* **Memory Footprint:** Caching all Services in memory slightly increases the baseline memory usage of the `kube-prober` container.
* **Startup Delay:** The application must block probing operations and wait for `cache.WaitForCacheSync` to complete for both EndpointSlices and Services during initialization (`main.go`).
