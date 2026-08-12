# ADR 0020: Modular Go Package Layout and Separation of Concerns

* **Status:** Accepted
* **Date:** 2026-08-01

## Context

As `Kube Prober` grew to support multiple health check protocols (HTTP, TCP, TLS, gRPC), Kubernetes informer logic, distributed sharding, and a custom telemetry server, keeping all application logic within a single `main.go` file became unmanageable.

A monolithic `main.go` structure leads to several issues:

* **Poor readability and difficult code navigation.
* **Hard to write isolated unit tests without complex mocking of the entire application state.
* **High risk of merge conflicts when multiple features are developed concurrently.

## Decision

We decided to enforce a **Modular Package Layout**, splitting the codebase into distinct, domain-specific packages inside the `pkg/` directory (`pkg/prober`, `pkg/server`, `pkg/kube`, `pkg/env`), while restricting `main.go` strictly to application bootstrapping and dependency injection.

## Options Considered

* **Monolithic `main.go`:** *Rejected because* it severely limits maintainability, makes unit testing difficult, and violates the principle of separation of concerns.
* **Modular Domain Packages (Chosen):** *Selected because* grouping related logic into dedicated packages allows for encapsulated state, clear interfaces, and independent unit testing (`_test.go` alongside implementation files).

## Consequences

### Positive

* **Separation of Concerns:** `main.go` acts solely as the entry point, wiring dependencies together (e.g., passing the initialized `prober.Dispatcher` and `kubernetes.Clientset` to the workers).
* **Testability:** Each package can be tested in isolation (e.g., `http_test.go`, `registry_test.go`) using Go's native testing framework without booting the whole application.
* **Readability:** Developers can quickly locate specific domain logic (e.g., all Kubernetes informer logic lives in `pkg/prober/informer.go`).

### Negative / Trade-offs

* **Circular Dependencies:** Developers must be careful when designing package APIs to avoid import cycles, which the Go compiler strictly prohibits.
* **Public/Private Visibility:** Requires strict management of exported (capitalized) vs. unexported (lowercase) functions and structs to maintain clean package APIs.
