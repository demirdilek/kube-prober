# ADR 0014: Pod Security Context Hardening and Non-Root Execution

* **Status:** Accepted
* **Date:** 2026-07-24

## Context

Running containers with elevated privileges or default root user settings in Kubernetes poses significant security risk. If a containerized process is compromised, root access inside the container can be leveraged to attempt container breakouts, tamper with the host filesystem, or escalate privileges within the worker node.

For `Kube Prober` we needed to strictly enforce Kubernetes Pod Security Standards (Restricted Profile) to ensure least-privilege execution across all environments.

The security architecture needed to enforce:

1. Non-root execution to prevent privilege escalation.
2. An immutable root filesystem to prevent runtime code injection or malicious script execution.
3. Complete removal of Linux kernel capabilities that are unneeded for a network-probing application.

## Decision

We decided to enforce a strict **Pod Security Context** directly within the Helm deployment manifest (`helm/kube-prober/templates/deployment.yaml`) utilizing `runAsNonRoot: true`, `runAsUser: 65534`, `readOnlyRootFilesystem: true`, and `capabilities.drop: ["ALL"]`.

## Options Considered

* **Default Container Security Context (Root Execution):** *Rejected because* running as root (`UID 0`) violates Kubernetes SRE security best practices and exposes the host node to severe risks in case of remote code execution vulnerabilities.
* **Partial Hardening (Non-Root Only):** *Rejected because* allowing writable root filesystems or retaining default Linux capabilities leaves open attack vectors for dropping malicious binaries or modifying runtime files.
* **Strict Pod Security Context Hardening (Chosen):** *Selected because* it satisfies the Kubernetes Restricted Pod Security Standard:
  * **`runAsNonRoot: true` & `runAsUser: 65534`:** Forces the container to run strictly as the unprivileged `nobody` user.
  * **`readOnlyRootFilesystem: true`:** Locks down the root filesystem, rendering it completely read-only.
  * **`capabilities.drop: ["ALL"]` & `allowPrivilegeEscalation: false`:** Drops all kernel privileges and prevents child processes from gaining more privileges than their parent.

## Consequences

### Positive

* **Defense in Depth:** Even if an attacker achieves execution within the container, they cannot write to disk, escalate privileges, or interact with kernel capabilities.
* **Compliance & Auditability:** Out-of-the-box compliance with enterprise Kubernetes Security Standards and admission controllers (e.g., Kyverno, OPA Gatekeeper, or native Pod Security Admission).
* **Minimalist Runtime:** Complements the `scratch` zero-OS base image strategy by guaranteeing zero root privilege surface area.

### Negative / Trade-offs

* **No Ephemeral File Writes:** The application cannot write temporary files or logs to the local filesystem unless explicit `emptyDir` in-memory volumes are mounted. (Since `Kube Prober` streams structured logs directly to `stdout`, this restriction requires no extra volume overhead).
* **Strict Socket Capabilities:** Lowering Linux capabilities prevents raw ICMP socket usage (`ping`), requiring probe checks to rely on Layer 4/7 TCP, HTTP, TLS, and gRPC sockets instead.
