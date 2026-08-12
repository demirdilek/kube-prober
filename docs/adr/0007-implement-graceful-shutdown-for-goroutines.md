# ADR 0007: Implementation of Graceful Shutdown for Goroutines and HTTP Server

* **Status:** Accepted
* **Date:** 2026-07-19

## Context

When running in Kubernetes, application pods are frequently created, rescheduled, or terminated during rolling updates, HPA scaling events, or node maintenance.

If `Kube Prober` terminates instantly upon receiving an OS termination signal (`SIGTERM`), several critical issues occur:

* ** Active in-flight probes (HTTP, TCP, TLS, gRPC) are forcefully killed, leading to false-positive error spikes in metrics.
* ** Background scheduler goroutines and worker pool loops are interrupted mid-execution, leaving channels and resources in an uncleaned state.
* ** The internal telemetry server drops active connection scrapes from Prometheus.

## Decision

We decided to implement a structured, context-driven **Graceful Shutdown pattern** using Go's `signal.NotifyContext`, `context.WithTimeout`, and `sync.WaitGroup`.

## Options Considered

* **Immediate Process Termination (`os.Exit(0)` on signal):** *Rejected because* abruptly cutting off network probes generates false error alerts and breaks in-flight HTTP transactions.
* **Kubernetes `preStop` Hooks Only:** *Rejected because* relying solely on container lifecycle hooks does not resolve internal application goroutine leaks or pending socket connection drains.
* **Context Cancellation & WaitGroup Drain (Chosen):** *Selected because* listening natively for `SIGTERM` and `SIGINT` signals allows cancelling child contexts (`ctx.Done()`), closing job channels, stopping periodic schedulers, and waiting for all worker goroutines to complete within a dedicated grace period (5 seconds) before process termination.

## Consequences

### Positive

* **Zero In-Flight Probe Loss:** Active network probes are given time to complete cleanly, preventing false-positive SRE alerts during deployments.
* **Clean Goroutine Lifecycle:** Scheduler loops stop producing new jobs immediately upon signal reception, and worker pools exit naturally via `sync.WaitGroup.Wait()`.
* **Controlled HTTP Server Shutdown:** The telemetry and probe server (`pkg/server`) invokes `srv.Shutdown(ctx)` to finish active HTTP scrapings before closing port 8080.
* **Kubernetes Native Alignment:** Integrates seamlessly with Kubernetes pod termination grace periods (`terminationGracePeriodSeconds`).

### Negative / Trade-offs

* **Slight Delay in Pod Termination:** Pods require up to a few seconds to shut down completely instead of exiting instantly.
* **Timeout Boundary Required:** If a network probe hangs indefinitely without respecting context timeouts, it could block shutdown until the hard timeout (5 seconds) cancels the context forcefully.
