# Production Readiness Roadmap

This document outlines the planned improvements, architectural refinements, and feature milestones for `kube-prober`.

---

## Phase 1: High Priority (Bugs & Security Hardening)

- [X] **RBAC Permissions Update (`helm/kube-prober/templates/rbac.yaml`)**
  - Add `endpointslices` under the `discovery.k8s.io` API group to prevent HTTP 403 (Forbidden) errors during Informer synchronization inside Kubernetes clusters.
- [X] **Kubernetes Probes Configuration (`helm/kube-prober/templates/deployment.yaml`)**
  - Integrate native `livenessProbe` (`/healthz`) and `readinessProbe` (`/readyz`) endpoints into the deployment spec.
- [X] **Pod Security Context Hardening (`helm/kube-prober/templates/deployment.yaml`)**
  - Enforce non-root execution (`runAsNonRoot: true`), read-only root filesystems (`readOnlyRootFilesystem: true`), and drop all unneeded Linux capabilities (`capabilities.drop: ["ALL"]`).

---

## Phase 2: Medium Priority (Features & Reliability)

- [X] **Dynamic Path Resolution via Informer (`pkg/prober/registry.go`)**
  - Parse custom Service annotations (`probe/path`) dynamically via the Informer instead of hardcoding `/healthz` in the target URL builder.
- [X] **High Availability (HA) Setup**
  - Add `PodDisruptionBudget` (PDB) and `HorizontalPodAutoscaler` (HPA) manifests to the Helm chart.
- [X] **Metrics Clean-Up on Target Deletion**
  - Unregister or clean up Prometheus metrics (Gauge/Counter labels) upon target deletion to avoid stale metrics and memory leaks.

---

## Phase 3: Low Priority & Enterprise Expansion

- [X] **Distributed Target Sharding via Consistent Hashing**
  - Implement a sharding mechanism (e.g., consistent hashing or modulo partitioning based on pod ordinal/IPs) across prober replicas when scaled via HPA to prevent duplicate probing and horizontally distribute workload.
- [X] **SLO / SLI & Error Budget Exporting**
  - Expose calculated multi-window burn rates directly as Prometheus metrics and ship pre-configured `PrometheusRule` manifests.
- [ ] **Protocol Extension (TCP / TLS / gRPC / DNS)**
  - Extend `prober.Dispatcher` with additional protocol handlers (e.g., gRPC Health Checking, TCP banner checks, TLS certificate expiry tracking).
- [ ] **Core SRE Protocols for Probing & Monitoring**
  - [X] **TCP:** Layer 4 connectivity & banner checks for databases/caches.
  - [X] **TLS / SSL:** Certificate expiry tracking and handshake validation.
  - [X] **gRPC:** Internal microservices & control plane (`grpc.health.v1.Health`).
  - [] **DNS:** Resolution time and correctness for critical service lookups.
- [ ] **SRE Dashboard & Observability Hardening**
  - [x] Add 4 Golden Signals visualization panels.
  - [X] Add dedicated TLS Certificate Expiry tracking panel (stat/gauge).
  - [X] Add TCP Target Availability & Protocol Error Breakdown panels.
  - [X] Add SLO Error Budget Burn Rate panel based on recording rules.
- [ ] **Chaos Engineering Test Suites**
  - Define Chaos Mesh or LitmusChaos scenarios to validate telemetry accuracy during simulated network latency, packet loss, and pod eviction events.
- [ ] **v1.2.0 — Multi-Zone Vantage Point Probing & Follow-the-Sun Alerting:**
  - [ ] **Locality-Aware Probing:** Inject `MY_NODE_ZONE` (via K8s Downward API / Node labels) into prober instances.
  - [ ] **Multi-Vantage Point Metrics:** Expand Prometheus metrics with `source_zone` labels (`kube_prober_latency_seconds{source_zone="..."}`) to measure global latency per region.
  - [ ] **Zonal Sharding Pools:** Filter peer lists in `watchProberPeers` by zone so each region runs its own Rendezvous Hashing ring over global targets.
  - [ ] **Follow-the-Sun Alert Routing:** Configure PrometheusRules and Alertmanager `active_time_intervals` (EU / US / APAC shifts) to route critical alerts dynamically to the active on-call team based on UTC business hours.

---

## Phase 4: Enterprise & SaaS Expansion (Commercial Product Roadmap)

- [ ] **Multi-Tenancy & Tenant Isolation**
  - Implement tenant-aware target grouping and metric isolation so multiple companies can securely share a single prober cluster without overlapping visibility.
- [ ] **Dynamic API & Authentication Layer**
  - Build a secure control-plane API (REST/gRPC) protected by API Keys or JWT tokens, allowing external clients to programmatically register, update, or delete probe targets without direct Kubernetes access.
- [ ] **Dynamic Multi-Tenant Alert Routing**
  - Extend the Alertmanager and webhook integration so individual tenants can configure and manage their own notification endpoints (Slack, PagerDuty, Webhooks) independently.
- [ ] **Metering & Usage Tracking**
  - Expose internal metrics for active probe counts, request volumes, and resource consumption to support billing, tiering, and usage tracking for a SaaS business model.
- [ ] **Enterprise Secrets Management Integration**
  - Native integration with HashiCorp Vault or external secret operators for automated, secure injection of tenant-specific webhook credentials and tokens.

## Phase 5: Cloud Native Standards & Developer Experience

- [ ] **OpenTelemetry (OTel):** Migrate from vendor-specific Prometheus client libraries to the OpenTelemetry standard for emitting the 4 Golden Signals natively from the Go application.
- [ ] **Skaffold Integration:** Standardize the inner development loop by transitioning from the custom Makefile to Google's Skaffold for hermetic, fast-iterating local Kubernetes deployments.
- [ ] **SLOs as Code:** Define Service Level Objectives and Error Budgets declaratively using tools like Sloth to automatically generate complex, multi-window burn rate alerts.
- [ ] **Context-Aware Structured Logging:** Integrate Go's native `slog` with OpenTelemetry to inject Trace IDs into log entries, enabling seamless navigation from a firing alert directly to the failing request.

## Phase 6: Service Mesh Integration & Global Observability**

- [ ] **Service Mesh Evaluation:** Evaluate and test a lightweight service mesh (e.g., Linkerd or Cilium) in the k3d cluster to monitor service-to-service communication.
- [ ] **Global Mesh Metrics Integration:** Extend `kube-prober` to query central mesh metrics alongside direct endpoint probing for a complete global system overview.
- [ ] **Unified 4 Golden Signals:** Combine internal application metrics with network-level service mesh telemetry into a single, comprehensive monitoring dashboard.
