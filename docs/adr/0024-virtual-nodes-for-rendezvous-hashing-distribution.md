# ADR 0024: Virtual Nodes for Gaussian-Balanced Rendezvous Hashing Distribution

* **Status:** Accepted
* **Date:** 2026-08-20

## Context

When scaling `kube-prober` horizontally across multiple replicas, target endpoints are partitioned using Highest Random Weight (HRW) Rendezvous Hashing to prevent duplicate probing.

In the initial implementation, each replica was represented by a single node identity (Pod IP). With finite target sample sizes or dynamic replica scaling (e.g., 5–10 pods), this caused measurable distribution skew, leading to uneven worker pool utilization across prober instances.

We needed a coordination-free mechanism to achieve near-uniform target distribution governed by a tight Gaussian normal distribution, without introducing central consensus stores or external schedulers.

## Decision

We decided to implement **Virtual Nodes (VNodes)** within the Rendezvous Hashing algorithm (`pkg/prober/sharding.go`), assigning 100 virtual nodes per active peer replica (`vnodesPerPeer = 100`).

Target ownership is evaluated deterministically by computing:
$$\text{weight}(T, P_i, v) = \text{FNV-1a}(T \mathbin{\Vert} 0 \mathbin{\Vert} P_i \mathbin{\Vert} \text{"\#"} \mathbin{\Vert} v)$$

By leveraging the Central Limit Theorem across 100 virtual sampling points per replica, the target distribution tightly conforms to a **Gaussian normal distribution** centered around the ideal mean ($\mu = \frac{\text{Targets}}{\text{Peers}}$) with minimal standard deviation ($\sigma$).

The pod holding the highest weight across all peer virtual nodes claims exclusive target ownership.

## Options Considered

* **Single-Node Rendezvous Hashing:** *Rejected because* of high distribution variance and load imbalance across small replica counts.
* **Centralized Coordinator (e.g., etcd / Raft):** *Rejected because* it introduces an external Single Point of Failure (SPOF), state synchronization latency, and operational complexity.
* **Virtual Nodes with Gaussian Normalization (Chosen):** *Selected because* it maintains 100% stateless execution, requires zero inter-pod communication, and tightly balances target load across active replicas (<5% standard deviation) while preserving minimal reassignment churn during HPA scaling events.

## Consequences

### Positive

* **Gaussian Load Balancing:** Target allocation tightly clusters around the mathematical mean, eliminating outlier replica starvation or overload.
* **Stateless & Zero Coordination:** Ownership calculation remains purely mathematical and local to each prober instance.
* **Zero-Allocation Hashing:** Optimized memory traversal in FNV-1a prevents heap allocations during dynamic rebalance loops.

### Negative / Trade-offs

* **Evaluation Overhead:** Increases local hash calculation complexity to $O(N \times V)$ where $N$ is peer count and $V$ is virtual node count (100), which remains negligible in CPU overhead.
