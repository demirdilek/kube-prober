# ADR 0015: Structured Logging with Go Standard Library (`log/slog`) and Contextual Correlation

* **Status:** Accepted
* **Date:** 2026-07-25

## Context

As `Kube Prober` executes distributed health checks across multiple targets and protocols, high-volume log output must be easily searchable, parseable, and correlated with metrics and traces during operational incidents.

Unstructured string logging (e.g., `fmt.Printf` or standard `log.Println`) creates major operational bottlenecks:

* **Difficulty in querying specific probe executions, protocol failures, or target labels in log aggregators (e.g., Grafana Loki, Elasticsearch).
* **High parsing overhead for log collectors trying to extract key-value fields.
* **Lack of diagnostic context (e.g., target URI, SRE error category, actionable hint) attached to error events.

## Decision

We decided to adopt Go's native **`log/slog`** package configured with a **JSON Handler** as the standard structured logging framework across all packages (`pkg/prober`, `pkg/watcher`, `pkg/server`).

## Options Considered

* **Third-Party Libraries (e.g., `uber-go/zap` or `sirupsen/logrus`):** *Rejected because* introducing external logging dependencies increases the module maintenance burden and binary footprint. Since Go 1.21, `log/slog` provides high-performance, structured, key-value logging natively within the standard library.
* **Unstructured / Plain-Text Logging (`log` package):** *Rejected because* unstructured text logs require brittle regular expressions to parse in log management systems and lack standardized log levels (INFO, WARN, ERROR).
* **Native `log/slog` with JSON Handler (Chosen):** *Selected because* it offers zero-dependency, highly performant, type-safe structured logging outputting standardized JSON formats directly to `stdout`.

## Consequences

### Positive

* **Zero External Dependencies:** Keeps the application lightweight and reduces supply-chain risk by relying entirely on the Go standard library.
* **Cloud-Native Log Ingestion:** Outputting structured JSON logs to `stdout` enables automatic field extraction by Kubernetes log collectors (Promtail, Fluentbit, Vector) without custom regex parsing pipelines.
* **Contextual Correlation & SRE Hints:** Log entries automatically include key operational metadata:
  * Target identification (`target`, `scheme`, `path`).
  * SRE classification (`category`).
  * Actionable diagnostic hints (`hint`) directly in the error log record to speed up incident triage.
* **Log Level Control:** Supports dynamic log level configuration (`DEBUG`, `INFO`, `WARN`, `ERROR`) via environment variables to control verbosity between development and production environments.

### Negative / Trade-offs

* **Human Readability in Raw Terminal:** Raw JSON logs printed to the terminal during local execution can be less human-readable than colorized plain text; requires pipe filtering (e.g., `jq`) or selecting `TEXT` handler format in local development modes.
