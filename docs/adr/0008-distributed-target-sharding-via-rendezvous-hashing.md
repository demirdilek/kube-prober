# ADR 0008: Distributed Target Sharding via Rendezvous Hashing

* **Status:** Accepted
* **Date:** 2026-07-20

## Context

When scaling `Kube Prober` horizontally across multiple pod replicas (e.g., via HorizontalPodAutoscaler), all prober instances receive the full list of target services discovered by the Kubernetes EndpointSlice informer.

Without workload partitioning, every replica would probe every target endpoint independently. This causes several severe operational issues:

* **Duplicate Probing:** Upstream target applications face unnecessary, multiplied health-check traffic.
* **Redundant Workload:** Scaling up prober replicas increases total cluster network load without increasing probing capacity.
* **Split Metrics:** Prometheus receives duplicated time-series data from multiple sources for the same target without clear ownership.

## Decision

We decided to implement **Distributed Target Sharding using Rendezvous Hashing (Highest Random Weight - HRW)** combined with dynamic peer discovery to deterministically assign target ownership across active prober replicas.

## Options Considered

* **Centralized Work Queue (e.g., Redis/RabbitMQ):** *Rejected because* it introduces an external single point of failure (SPOF), increases infrastructure overhead, and adds network latency to target distribution.
* **Consistent Hashing Ring:** *Rejected because* standard hash ring topologies require complex virtual node management and rebalancing overhead during pod scaling events.
* **Rendezvous Hashing (Chosen):** *Selected because* HRW hashing is fully stateless, requires zero inter-pod coordination, distributes targets evenly across available peers, and minimizes target reassignment (minimal disruption) when pods scale up or down.

## Consequences

### Positive

* **Stateless Ownership Resolution:** Each pod independently determines whether it should probe a target (`ShouldProcessTarget`) using its own Pod IP (`POD_IP`) and the active peer list
* **Minimal Target Churn:** When scaling from $N$ to $N+1$ replicas, only $\frac{1}{N+1}$ of the targets are moved to the new replica, while the remaining mappings stay intact.
* **Dynamic Peer Awareness:** The `WatchPeers` informer automatically tracks ready `kube-prober` endpoint IP changes and triggers local rebalancing events without requiring restart or central orchestration.
* **Zero Overhead Scaling:** Allows horizontal scaling to handle tens of thousands of targets linearly without duplicate network requests.

### Negative / Trade-offs

* **Computation per Target:** Evaluating the hash function ($O(N)$ where $N$ is peer count) for every target requires light CPU computation during target rebalancing events.
* **IP Dependency:** Requires injection of the local Pod IP via the Kubernetes Downward API (`POD_IP`) to establish pod identity.
