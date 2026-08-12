# ADR 0013: Adoption of Zero-OS Base Image (`scratch`) and Multi-Stage Builds

* **Status:** Accepted
* **Date:** 2026-07-23

## Context

For containerizing `Kube Prober`, we needed a secure, minimal, and high-performance container image packaging strategy for production deployments.

Standard Linux container base images (such as Ubuntu, Debian, or even Alpine) bring several operational and security drawbacks:

* **Attack Surface & Vulnerabilities (CVEs):** Included shell utilities, OS package managers, and system libraries increase the security attack surface and trigger frequent vulnerability scanner alerts.
* **Image Footprint & Bandwidth:** Large base images increase container registry storage, slow down CI/CD build pipelines, and delay pod startup times during autoscaling events in Kubernetes.
* **Runtime Overhead:** Unneeded OS utilities consume unnecessary disk space and memory in production nodes.

## Decision

We decided to adopt a **two-stage Multi-Stage Docker Build** that compiles the Go binary statically and outputs a final, minimalist **`scratch` base image** containing only the compiled binary and essential CA root certificates.

## Options Considered

* **Full OS Base Image (e.g., `ubuntu` / `debian`):** *Rejected because* of massive image sizes (>100 MB), high vulnerability (CVE) surface area, and unnecessary system utilities in runtime.
* **Minimal OS Base Image (e.g., `alpine`):** *Rejected because* while lightweight (~10 MB), it still includes a shell (`/bin/sh`), package manager (`apk`), and basic POSIX utilities that are not required for a statically compiled Go microservice.
* **Zero-OS `scratch` Image (Chosen):** *Selected because* `scratch` is an empty base image with zero OS overhead. Combined with static compilation (`CGO_ENABLED=0`), it produces an ultra-secure, minimalist image (~29 MB) containing strictly the compiled `kube-prober` binary and updated Root CA certificates (`ca-certificates.crt`) for outgoing HTTPS/TLS probes.

## Consequences

### Positive

* **Minimal Attack Surface:** Completely eliminates shell-based attacks (e.g., `exec` probes into running containers) and OS-level vulnerabilities, significantly hardening pod security posture.
* **Ultra-Fast Container Deployment:** Small image footprint (~29 MB) dramatically speeds up image pulling across Kubernetes worker nodes during HPA scaling events or deployment rollouts.
* **Reproducible & Statically Linked Builds:** Multi-stage Docker caching (`--mount=type=cache`) isolates build dependencies (`golang:alpine` builder stage) from the final runtime artifact and speeds up building.
* **Compliance & Security Scans:** Clean container scanning results due to the complete absence of OS libraries and package managers.

### Negative / Trade-offs

* **No In-Container Debugging Tools:** Because `scratch` contains no shell (`sh`/`bash`) or basic CLI tools (`curl`/`ping`), operators cannot run `kubectl exec` directly inside the container; debugging requires ephemeral debug containers (`kubectl debug`) or telemetry logs.
* **Manual CA Certificate Injection:** TLS verification requires explicitly copying SSL certificates (`/etc/ssl/certs/ca-certificates.crt`) from the build stage into the final image to allow secure HTTPS/TLS probing.
