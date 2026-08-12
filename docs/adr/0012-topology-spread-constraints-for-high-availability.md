# ADR 0012: Topology Spread Constraints for Node-Level High Availability

* **Status:** Accepted
* **Date:** 2026-07-22

## Context

Running multiple replicas of `Kube Prober` via HorizontalPodAutoscaler (HPA) or static replica counts protects against application crashes. However, if the Kubernetes scheduler places all pod replicas onto the same underlying physical or virtual node, a single node hardware failure or node drain causes a total service outage (Single Point of Failure - SPOF).

To ensure true High Availability (HA), pod replicas must be evenly distributed across distinct physical infrastructure (nodes/hosts).

## Decision

We decided to enforce **Topology Spread Constraints** (`topologySpreadConstraints`) in the deployment spec using `kubernetes.io/hostname` as the topology key and a maximum skew of 1 (`maxSkew: 1`).

## Options Considered

* **No Placement Rules (Default Scheduling):** *Rejected because* the default scheduler might pack multiple replicas onto a single high-capacity node to save cluster resources, leaving the service vulnerable to single-node failures.
* **Pod Anti-Affinity (`podAntiAffinity`):** *Rejected because* hard anti-affinity rules are binary and inflexible, often causing scheduling blocks or requiring complex rule definitions compared to modern topology spread constraints.
* **Topology Spread Constraints with `maxSkew: 1` (Chosen):** *Selected because* it dynamically enforces an even distribution of pods across physical nodes[cite: 1]. In production (`values-prod.yaml`), `whenUnsatisfiable: DoNotSchedule` strictly prevents placing uneven pod counts on the same node, forcing infrastructure expansion or multi-node placement.

## Consequences

### Positive

* **Hardware Resilience:** Guarantees that a single node crash or maintenance reboot only affects a fraction of the total prober capacity, allowing remaining nodes to maintain target probing uninterrupted.
* **Predictable Even Spreading:** A `maxSkew` of 1 ensures the difference in pod count between any two nodes never exceeds 1.
* **Flexible Environment Overrides:** Dev environments can use `ScheduleAnyway` for single/dual-node k3d clusters (`values.yaml`), while production strictly enforces `DoNotSchedule` (`values-prod.yaml`).

### Negative / Trade-offs

* **Node Infrastructure Dependency:** In production, scaling up replicas requires sufficient distinct nodes to be available in the cluster; otherwise, pods remain in a `Pending` state until new nodes are provisioned.
