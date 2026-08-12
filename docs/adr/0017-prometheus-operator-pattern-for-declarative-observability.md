# ADR 0017: Prometheus Operator Pattern for Declarative Observability

* **Status:** Accepted
* **Date:** 2026-07-27

## Context

To effectively monitor `Kube Prober` and evaluate the 4 Golden Signals, the system must expose metrics and define SRE alert rules. Traditional Prometheus scraping relies on centralized cluster configurations or pod annotations (`prometheus.io/scrape: "true"`).

This traditional approach decouples the application code from its monitoring configuration, making it difficult to version-control alert thresholds alongside the specific deployment manifests they monitor.

## Decision

We decided to adopt the **Prometheus Operator Pattern** by utilizing `ServiceMonitor` and `PrometheusRule` Custom Resource Definitions (CRDs) integrated directly into the application's Helm chart (`helm/kube-prober/templates/`).

## Options Considered

* **Standard Pod Annotations:** *Rejected because* they do not support packaging custom SRE alert rules natively and rely heavily on centralized Prometheus configuration management.
* **Prometheus Operator CRDs (Chosen):** *Selected because* it allows declaring the scrape configuration (`servicemonitor.yaml`) and multi-window SLO burn rate alerts (`prometheusrule.yaml`) as part of the application release. This perfectly aligns with our GitOps deployment architecture.

## Consequences

### Positive

* **Self-Contained Observability:** The application ships with its own monitoring configuration, ensuring that metrics pipelines and alert thresholds are always synchronized with the current code version.
* **GitOps Alignment:** Argo CD can manage, visualize, and track the sync status of the `ServiceMonitor` and `PrometheusRule` just like any standard Kubernetes resource.
* **Automated Discovery:** The Prometheus Operator dynamically detects the `ServiceMonitor` via the `release: prom-stack` label, updating the Prometheus server scrape targets automatically without manual reloads.

### Negative / Trade-offs

* **Strict CRD Dependency:** The Helm chart deployment will fail if the Prometheus Operator CRDs are not installed in the cluster beforehand.
