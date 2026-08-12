# ADR 0021: 12-Factor(Heroku) App Configuration via Environment Variables

* **Status:** Accepted
* **Date:** 2026-08-03

## Context

`Kube Prober` needs dynamic configuration values for execution boundaries (e.g., `WORKERS`, `QUEUE_SIZE`, `HTTP_TIMEOUT_SECONDS`) and identity (`POD_IP`).

In cloud-native environments, application configuration should be strictly separated from code. Embedding configuration files (like YAML or JSON) requires external parsing libraries and complicates dynamic configuration injection via Kubernetes ConfigMaps or Helm.

## Decision

We decided to strictly adhere to the **12-Factor App methodology (Factor III: Config)** by managing all deployment-specific configurations exclusively through Environment Variables, parsed by a custom, zero-dependency `pkg/env` utility package.

## Options Considered

* **Third-Party Config Libraries (e.g., Viper / Koanf):** *Rejected because* they pull in heavy dependencies, increase binary size, and are over-engineered for a microservice that only requires basic integer/string toggles.
* **Static Config Files (`config.yaml`):** *Rejected because* they violate 12-Factor principles, requiring file mounts in Kubernetes rather than simple environment variable injection.
* **Environment Variables with Custom Fallbacks (Chosen):** *Selected because* native Go `os.Getenv` paired with a custom `env.GetInt` fallback wrapper is extremely lightweight, fully decoupled from external libraries, and integrates seamlessly with Kubernetes `env` definitions in the Deployment manifest.

## Consequences

### Positive

* **Kubernetes Native Integration:** Seamlessly maps to Helm values and Kubernetes Deployment `env` blocks without requiring complex volume mounts.
* **Zero Dependency:** The `pkg/env` package relies solely on the Go standard library (`os`, `strconv`).
* **Portability:** The same Docker image (`scratch`) can be promoted across environments (Dev, Staging, Prod) by merely changing the injected environment variables.

### Negative / Trade-offs

* **No Hot-Reloading:** Changes to the configuration require a pod restart to take effect, as environment variables are only read during the application boot sequence in `main.go`.
