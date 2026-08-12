# ADR 0005: Adoption of GitOps with Argo CD for Continuous Delivery

* **Status:** Accepted
* **Date:** 2026-07-17

## Context

For deploying and operating `Kube Prober` reliably across Kubernetes environments, we required an automated, declarative, and auditable deployment pipeline.

Manual deployments via CLI (`kubectl apply` or `helm install`) introduce risks of configuration drift, lack of visibility into deployed versions, and manual operator errors.

## Decision

We decided to adopt **Argo CD** as our primary GitOps continuous delivery tool to automate application deployment and state reconciliation from Git.

## Options Considered

* **Imperative CLI Deployments (`kubectl` / `helm` in CI/CD):** *Rejected because* it lacks automatic drift detection, self-healing capabilities, and auditability.
* **Flux CD:** *Rejected because* Argo CD provides a superior web user interface for visualization, multi-application management, and native synchronization controls via custom manifests (`deploy/argocd/kube-prober-app.yaml`).
* **Argo CD (Chosen):** *Selected because* it continuously monitors the live cluster state against the desired Helm configuration in Git, enforcing automated reconciliation, drift detection, and easy rollback mechanisms.

## Consequences

### Positive

* **Automated State Reconciliation:** Any manual modification in the cluster is automatically corrected to match the desired state in Git (self-healing).
* **Declarative Control:** Application definitions are stored as version-controlled code (`deploy/argocd/kube-prober-app.yaml`).
* **Visibility & UI:** Provides a centralized control plane interface to observe deployment health, sync statuses, and resource trees.

### Negative / Trade-offs

* **Control Plane Overhead:** Requires running Argo CD components inside the cluster (`argocd` namespace), consuming additional memory and CPU resources.
