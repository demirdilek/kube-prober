# ADR 0011: Adoption of Horizontal Pod Autoscaler (HPA) for Dynamic Workload Scaling

* **Status:** Accepted
* **Date:** 2026-07-22

## Context

The workload of `Kube Prober` directly depends on the number of active Kubernetes services and EndpointSlices discovered across the cluster. As the cluster scales or target probing intervals change, resource demands (CPU and Memory) fluctuate significantly.

A static replica allocation presents distinct drawbacks:

* **Over-Provisioning:** Allocating high replica counts permanently wastes cluster resources during low-load periods.
* **Under-Provisioning / Saturation:** Running too few replicas during target spikes leads to worker pool queue saturation (`kube_prober_saturation_active_workers`), increased probe latency, or Out-Of-Memory (OOM) kills[cite: 1].

## Decision

We decided to configure the **Kubernetes Horizontal Pod Autoscaler (HPA v2)** to dynamically adjust the prober replica count based on real-time resource utilization.

## Options Considered

* **Static Replica Count:** *Rejected because* it cannot adapt to dynamic cluster growth, leading to resource inefficiency or worker pool saturation.
* **Manual Scaling:** *Rejected because* manual operator intervention is too slow to react to automated cluster auto-scaling events or microservice rollouts.
* **Horizontal Pod Autoscaler (Chosen):** *Selected because* HPA automatically scales the prober deployment between a defined minimum (`minReplicas: 2` for high availability) and maximum (`maxReplicas: 10`) based on CPU and Memory thresholds (e.g., 75% utilization).

## Consequences

### Positive

* **Seamless Compatibility with Target Sharding:** HPA scaling events trigger Rendezvous Hashing rebalances via peer discovery (`WatchPeers`), redistributing target ownership smoothly across newly scaled pods without duplicate probes.
* **High Availability & Cost Efficiency:** Ensures a baseline of 2 replicas in production (`values-prod.yaml`) for fault tolerance while scaling up on-demand under heavy load.
* **Integrated Declarative Manifests:** The HPA specification is fully parameterizable via the Helm chart (`helm/kube-prober/templates/hpa.yaml`).

### Negative / Trade-offs

* **Metrics Server Dependency:** Requires a functional `metrics-server` in the Kubernetes cluster to expose CPU and Memory metrics to the `autoscaling/v2` API.
* **Target Rebalancing Churn:** Scaling up or down causes temporary target ownership reassignment across pods, though minimized by the $O(1)$ stability properties of Rendezvous Hashing.
