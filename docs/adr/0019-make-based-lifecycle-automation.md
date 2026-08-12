# ADR 0019: Make-Based Lifecycle Automation

* **Status:** Accepted
* **Date:** 2026-07-29

## Context

Managing a local Kubernetes development environment (k3d), deploying GitOps controllers (Argo CD), running tests, and simulating SRE alerts requires executing a sequence of complex CLI commands.

Developers need a simple, unified, and discoverable interface for the "inner development loop" without having to memorize exact `kubectl`, `helm`, or `docker` command flags.

## Decision

We decided to use a standard **`Makefile`** as the primary task runner and automation tool for cluster bootstrapping, local deployments, and fault injection.

## Options Considered

* **Disjointed Bash Scripts:** *Rejected because* managing multiple separate scripts lacks discoverability and native target-dependency resolution (e.g., ensuring the cluster exists before deploying Argo CD).
* **Advanced Dev Tools (Skaffold / Tilt):** *Rejected for initial phases because* while powerful, they introduce additional tooling dependencies and complexity. (Migration to Skaffold is planned for Phase 5 of the Roadmap).
* **Makefile (Chosen):** *Selected because* it natively supports Unix/Linux terminal environments, requires no additional package installations, and easily chains complex Kubernetes commands through simple targets like `make bootstrap` or `make test-alert-latency`.

## Consequences

### Positive

* **Frictionless Onboarding:** A single command (`make bootstrap`) spins up the entire local k3d cluster, builds the images, installs Prometheus, and deploys Argo CD.
* **Simplified SRE Testing:** Alert scenarios (e.g., High Latency, Target Saturation) can be injected instantly via intuitive targets (`make test-alert-*`).
* **Dependency Management:** Make targets define clear prerequisites (e.g., `bootstrap` automatically triggers `k3d-up`, `cache-test-images`, and `prometheus-install`).

### Negative / Trade-offs

* **Manual Redeployments:** Unlike Skaffold, the current Make-based workflow lacks native hot-reloading (live file sync) for Go code; developers must explicitly run `make local-deploy` to build and roll out changes.
