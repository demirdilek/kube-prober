# ADR 0004: Standardization on SRE 4 Golden Signals for Observability

* **Status:** Accepted
* **Date:** 2026-07-16

## Context

For observing system health, diagnosing production issues, and establishing Service Level Objectives (SLOs) for `Kube Prober`, we required a standardized metrics framework.

Without a clear telemetric framework, microservice monitoring often suffers from metric fatigue, inconsistent alert rules, and unclear diagnostic paths during incidents.

## Decision

We decided to standardize on Google's **SRE 4 Golden Signals** (Latency, Traffic, Errors, Saturation) as the foundational metric framework for application instrumentation, Prometheus alerting, and Grafana visualization.

## Options Considered

* **USE Method (Utilization, Saturation, Errors):** *Rejected because* it focuses primarily on hardware/resource infrastructure rather than end-to-end service performance and request behavior.
* **RED Method (Rate, Errors, Duration):** *Rejected because* while effective for request-driven services, it lacks native emphasis on worker pool and capacity saturation.
* **4 Golden Signals (Chosen):** *Selected because* it provides a comprehensive 360-degree view covering request performance (Latency), throughput (Traffic), failures (Errors), and capacity boundaries (Saturation) for both the prober engine and target workloads.

## Consequences

### Positive

* **Actionable Telemetry:** Metrics are directly mapped to key SRE dimensions:
* **Latency:** Quantified via histograms (`kube_prober_latency_seconds`).
* **Traffic:** Measured via probe volume counters (`kube_prober_traffic_total`).
* **Errors:** Categorized by SRE error types (`kube_prober_errors_total` with `category` labels).
* **Saturation:** Tracked via active vs. maximum worker capacity (`kube_prober_saturation_active_workers`).
* **SLO-Driven Alerting:** Enables precise multi-window burn rate alerts (e.g., 1h/5m Availability and Latency error budget burn rates) in `PrometheusRule` manifests.
* **Standardized Dashboards:** Automatically provisioned Grafana dashboards organize panels cleanly by the 4 Golden Signals to speed up incident root-cause analysis (MTTR).

### Negative / Trade-offs

* **Metric Cardinality:** Tracking latency histograms and error categories per target endpoint increases Prometheus TSDB index size and memory usage over time, requiring active target deletion cleanup logic (`DeleteTargetMetrics`).
