# ADR 0003: Adoption of Helm for Application Packaging and Deployment

* **Status:** Accepted
* **Date:** 2026-07-15

## Context

For deploying `Kube Prober` across different Kubernetes environments (local k3d development, CI/CD pipelines, and production), we needed a standardized, repeatable, and parameterized deployment mechanism.

The application consists of multiple interconnected Kubernetes resources:

* ** Deployments, Services, and ServiceAccounts with RBAC permissions.
* ** High Availability manifests (PodDisruptionBudget, HorizontalPodAutoscaler).
* ** Observability and telemetry configurations (PrometheusRule manifests and Grafana dashboard ConfigMaps).

Managing raw, static YAML manifests for different environments leads to code duplication, drift, and high operational complexity.

## Decision

We decided to adopt **Helm (v3)** as the primary package manager and templating engine for `Kube Prober`.

## Options Considered

* **Raw Kubernetes Manifests:** *Rejected because* maintaining static YAML files for multiple environments (e.g., local k3d vs. production) causes manifest drift and requires repetitive manual updates.
* **Kustomize:** *Rejected because* while good for overlaying YAML configurations, Helm provides better parameterization (`values.yaml`), chart versioning (`Chart.yaml`), dependency packaging, and native integration with GitOps tools like Argo CD[cite: 1].
* **Helm v3 (Chosen):** *Selected because* it offers declarative parameterization, chart versioning, environment-specific value overrides (`values-prod.yaml`), and seamless GitOps integration with Argo CD without requiring an in-cluster server-side component (Tiller-less architecture).

## Consequences

### Positive

* **Single Source of Truth:** All Kubernetes resources, RBAC roles, Prometheus alert rules, and Grafana dashboards are versioned and managed in a single chart (`helm/kube-prober`) with `Charts.yaml` for version control of the whole project.
* **Environment Parameterization:** Easy differentiation between local dev and production settings using override files (e.g., replica counts, resource requests/limits, and HPA triggers in `values-prod.yaml` vs `values.yaml`).
* **GitOps & Automation Readiness:** Argo CD natively reconciles Helm charts directly from the Git repository (`deploy/argocd/kube-prober-app.yaml`).
* **Simplified Observability Packaging:** Prometheus operator custom resources (`PrometheusRule`, `ServiceMonitor`) and Grafana dashboard ConfigMaps are bundled natively alongside the core workload.

### Negative / Trade-offs

* **Templating Complexity:** Go template syntax in Helm charts can become hard to read or debug when dealing with complex conditional blocks (`{{- if .Values... }}`).
* **Linting Requirements:** Requires additional CI steps (`helm lint`) to catch formatting and syntax errors before deployment.
