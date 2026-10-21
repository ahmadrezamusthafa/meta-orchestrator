---
name: launch-security-scanning
description: "Launch readiness step 4 — check security automatically. Adds blocking security checks to the pipeline (secret scanning, dependency/SCA, SAST, container and IaC scanning, SBOM, DAST baseline on staging), a triage and suppression policy, and a review of authentication, authorization, input validation and personal-data handling. Built into the orchestrator."
---

# launch-security-scanning — check security automatically

Goal: a known-vulnerable dependency, a leaked secret or an obviously unsafe pattern cannot reach
production unnoticed, and the team knows what to do with each finding. Wire the checks into the
pipeline from the release-automation step, using what the organization already runs (look for
Snyk, SonarQube, GitHub Advanced Security, Dependabot, Renovate, Trivy, Semgrep, gitleaks configs)
before adding anything new.

## 1. Automated checks

| Check | When | Blocks on | Examples (pick what fits the stack) |
|---|---|---|---|
| Secret scanning | every PR + pre-commit | any finding | gitleaks, trufflehog |
| Dependency / SCA | every PR + nightly | high/critical with a fix available | `govulncheck`, `npm audit`, `bundler-audit`, `pip-audit`, Trivy fs, Dependabot/Renovate for updates |
| SAST | every PR | high-confidence high severity | Semgrep, CodeQL, gosec, Brakeman |
| Container image | on build | critical with a fix | Trivy / Grype on the built image |
| IaC / config | every PR | high | Checkov, tfsec / Trivy config, kube-linter |
| SBOM | on release | — (produce & store) | Syft / Trivy, CycloneDX or SPDX |
| DAST baseline | after staging deploy | high | OWASP ZAP baseline against staging, never production |

Start new blocking checks with a **baseline**: existing findings are recorded and triaged, new ones
block. Never fail the build on findings with no fix available — report them.

## 2. Triage policy (write it down in `SECURITY.md` or `docs/security/`)

- Severity → response time (ASSUMPTION until the operator sets it): critical 7 days, high 30 days.
- Suppressions live in the tool's ignore file with: finding ID, reason, owner, expiry date. No
  silent or permanent ignores.
- Who gets notified and where findings are tracked.

## 3. Manual review of the code — record each finding with file:line

- **Access control**: every endpoint authenticated unless deliberately public; authorization checks
  the resource's owner/tenant (no IDOR across tenants); admin paths separated.
- **Input validation**: schema validation at the edge; parameterized queries; no shell/template
  injection; upload type/size limits; SSRF guards on outbound URLs.
- **Secrets management**: none in code, images or logs; loaded from the secret manager; rotation
  possible without a deploy.
- **Personal data** (Indonesia's personal data protection law, UU PDP 27/2022, and ISO/IEC
  27001/27701 obligations): identify every field holding personal data (NIK/KTP, KK, phone, email,
  address, bank/card numbers). Check it is encrypted at rest where sensitive, masked in logs and
  API responses that do not need it, has a retention rule, and that access is logged. Do not cite
  specific articles or control numbers unless you can quote them from a provided source — say
  "unverified" instead.
- **Transport & headers**: TLS only; HSTS; CORS restricted to known origins; secure cookie flags.
- **Dependencies**: no abandoned or typo-squatted packages; lockfiles committed.

## Guardrails

- Never print a secret you find — give the file path and line, say it must be rotated, and
  recommend removing it from history with the operator's approval.
- Never run DAST or any scan against production.
- Do not invent CVE IDs, CVSS scores or tool versions: quote them from tool output or mark them
  **unverified**.

## Tools

| Tool | Need | Used for |
|---|---|---|
| `gitleaks` | **required** | secrets in the code and its git history (`gitleaks git`, `gitleaks dir`) |
| `trivy` | **required** | dependencies (`trivy fs`), images (`trivy image`), IaC (`trivy config`), SBOM |
| `semgrep` | **required** | SAST baseline (`semgrep scan --config auto` or the repository's rules) |
| `osv-scanner` | recommended | second opinion on lockfiles |
| `govulncheck` | recommended (Go) | vulnerabilities the Go code actually reaches |
| `syft` | recommended | SBOM in CycloneDX or SPDX |
| `checkov` | recommended (IaC) | Terraform, Kubernetes, Helm misconfigurations |
| `docker` | recommended | OWASP ZAP baseline (`zaproxy/zap-stable` image) against **staging only** |

Required tools gate the stage: it waits until they are installed or the operator chooses to run without them. Install from the orchestrator's Tool Hub (**Launch Readiness** → *Install all missing*), the install prompt shown when the stage starts, or `go run ./cmd/daemon packages setup launch-readiness`. Never install software yourself from inside a stage: if a tool is missing, still wire it into the pipeline, give the exact command, and list it under operator actions.

## Output

```
## Checks added
(check — tool — trigger — blocks on — file)
## Baseline findings
(severity — finding — location — fix | suppressed until <date> because …)
## Code review findings
(risk area — file:line — issue — recommendation)
## Personal data inventory
(field — where stored — protection — gaps)
## Operator actions
## Follow-up tasks
(the `followups` JSON block from the stage brief: remaining gaps grouped into a few tasks)
## Summary
```
