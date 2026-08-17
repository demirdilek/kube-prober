# ADR 0023: Dual-Informer Target Discovery with Declarative CRDs for Core Infrastructure

* **Status:** Accepted
* **Date:** 2026-08-17

## Context

`Kube Prober` was originally designed to monitor dynamic application workloads by watching Kubernetes `EndpointSlices` with `probe: "true"` annotations. However, production environments require continuous monitoring of core Kubernetes control-plane components and external cluster infrastructure (e.g., `kube-apiserver` TLS expiry, CoreDNS resolution, upstream ingress controllers, and public DNS resolvers).

These critical infrastructure targets exhibit fundamental architectural differences compared to ephemeral microservices:

1. **Non-Annotatable or Out-of-Cluster Endpoints:** System endpoints like public DNS (`8.8.8.8`) or external ingress gateways cannot always be labeled via standard Kubernetes Service annotations.
2. **Precedence & Collision Risks:** If an EndpointSlice dynamically discovers an endpoint that shares the same network address as a static control-plane probe, dynamic events could override configurations (such as `insecureSkipVerify`) or remove the probe during scale-down events.
3. **Control-Plane Polling Overhead:** Reading static configurations from files or polling ConfigMaps breaks GitOps automation and lacks real-time event streaming.

## Decision

We decided to implement a **Dual-Informer Discovery Architecture** powered by a dedicated Custom Resource Definition (`StaticTarget` via `kube-prober.io/v1alpha1`) managed by a dynamic shared informer alongside the existing `EndpointSlice` informer.

Specifically:

* **Dynamic Informer (`KubeWatcher`):** Observes `EndpointSlices` to monitor ephemeral pod replicas and internal workloads.
* **Static Informer (`WatchStaticTargets`):** Listens to `StaticTarget` CRDs across all namespaces to monitor core infrastructure and control-plane services.
* **Deterministic Precedence (`Registry`):** Declarative static targets (`Static: true`) are assigned top priority in the memory registry; dynamic endpoint updates ignore duplicate addresses if a static target is active, and dynamic slice deletions are blocked from purging static infrastructure targets.

## Options Considered

* **ConfigMap with File Watcher:** *Rejected because* file mounts inside zero-OS containers (`scratch`) require external polling loops or volume reloads, which are prone to delayed updates and lack Kubernetes RBAC schema validation.
* **Hardcoded Probes in `main.go`:** *Rejected because* changes to monitored cluster infrastructure would require recompiling the binary and cutting a new container release.
* **Dual Informer with Custom CRD (Chosen):** *Selected because* it unifies GitOps workflows (managing static infrastructure via declarative YAML) while retaining real-time synchronization, zero API polling overhead, and deterministic priority over dynamic service discovery.

## Consequences

### Positive

* **Declarative Infrastructure Probing:** Cluster operators can monitor core endpoints (Kubernetes API server, CoreDNS, external DNS) using native Kubernetes manifests managed by Argo CD.
* **Strict Precedence & Protection:** Static targets cannot be accidentally purged or overwritten by EndpointSlice scale-down events.
* **Event-Driven Reconciliation:** Updates to `StaticTarget` CRDs are processed instantly via dynamic informer event handlers without pod restarts.
* **Extensible Target Configuration:** Allows target-specific attributes directly in the CRD spec (e.g., `insecureSkipVerify: true`).

### Negative / Trade-offs

* **CRD Dependency:** Requires installing the `statictargets.kube-prober.io` CustomResourceDefinition in the cluster before bootstrapping the controller.
* **RBAC Expansion:** Requires `get`, `list`, and `watch` permissions for the `kube-prober.io` API group in the `ClusterRole` manifest.
