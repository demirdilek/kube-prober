# ADR 0002: Adoption of k3d for Local Kubernetes Development

* **Status:** Accepted
* **Date:** 2026-07-14

## Context

For developing, testing, and verifying the `Retail Edge` control plane and probing engine locally, we required a lightweight, fast, and reproducible local Kubernetes environment.

The local setup needed to fulfill the following requirements:

* **Seamless integration with local Docker workflows (building and importing dev images without remote registry pushes).
* **Low resource overhead on developer machines while supporting multi-node cluster topographies (to test HPA and pod disruption budgets).
* **Fast cluster startup and reset cycles via automated Makefile scripts (`make bootstrap`, `make k3d-up`).

## Decision

We decided to adopt **k3d** (k3s running inside Docker) as the primary local Kubernetes development platform.

## Options Considered

* **Minikube:** *Rejected because* of higher resource consumption, slower startup times, and heavier VM/driver complexity when managing container images locally.
* **Kind (Kubernetes in Docker):** *Rejected because* k3d provided better container lifecycle management and faster multi-node cluster provisioning via Docker containers.
* **k3d (Chosen):** *Selected because* it runs k3s natively inside Docker containers. It offers near-instant cluster spin-up, simple multi-node scaling, and direct local image imports (`k3d image import`), making local development fast and frictionless.

## Consequences

### Positive

* **Efficient Docker Workflow:** Local images built with `docker build` can be imported directly into the cluster (`k3d image import`) without deploying a local image registry.
* **Fast Lifecycle Management:** Multi-node clusters can be created, updated, or wiped in seconds using `Makefile` targets (`make bootstrap`, `make k3d-down`).
* **Realistic Multi-Node Testing:** Easy configuration of multiple agent nodes allows testing topology spread constraints, sharding, and node failover scenarios on local machines.

### Negative / Trade-offs

* **K3s Control Plane Differences:** Lightweight k3s components (e.g., Traefik, local storage) differ slightly from full upstream Kubernetes control planes, requiring minor Helm value adjustments for compatibility (e.g., disabling default k3s ingress/controllers during stack setup).
