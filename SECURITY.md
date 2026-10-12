# Security Policy

## Supported Versions

We provide security updates and patches for the latest minor releases. We strongly recommend always running the latest patch release.

| Version | Supported          |
| ------- | ------------------ |
| 1.0.x   | :white_check_mark: |
| < 1.0.0 | :x:                |

---

## Reporting a Vulnerability

We take the security of `kube-prober` seriously. If you discover a security vulnerability, please do **not** report it via a public GitHub issue.

Instead, please report vulnerabilities using **GitHub Private Vulnerability Reporting**:

1. Navigate to the [Security tab of this repository](https://github.com/demirdilek/kube-prober/security).
2. Click on **Advisories** and select **Report a vulnerability**.
3. Provide a clear and detailed description including:
   - Type of vulnerability (e.g., DoS, memory leak, unhandled panic, privilege escalation).
   - Minimal steps or proof-of-concept to reproduce the behavior.
   - Affected components, flags, or CRD configurations.

### What to Expect

- **Acknowledgment:** You will receive an acknowledgment of your report within **48 hours**.
- **Assessment:** We will validate the issue and keep you updated on progress towards a fix.
- **Disclosure:** Once a patch is developed and verified, we will coordinate a release date and publish a security advisory with credit to the reporter.

Thank you for helping keep `kube-prober` secure.
