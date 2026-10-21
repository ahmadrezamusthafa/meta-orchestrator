---
name: launch-observability
description: "Launch readiness step 3 — watch the system and send alerts. Instruments the service with structured logs, RED/USE metrics and traces, defines SLIs and SLOs, and adds symptom-based alerts (burn-rate where possible) with a runbook link on every alert, plus dashboards as code. Built into the orchestrator."
---

# launch-observability — watch the system and send alerts

Goal: when customers are affected, the right person is paged **before** the customer complains,
and the alert tells them where to look. Use the stack the repository or organization already has
(look for Prometheus, OpenTelemetry, Datadog, New Relic, Grafana, Sentry, ELK/Loki, CloudWatch
clients and configs). Do not introduce a new vendor; if there is nothing, propose OpenTelemetry
with the exporter as configuration, and mark that choice for the operator.

## 1. Signals

- **Logs**: structured (JSON), one event per line, with `timestamp`, `level`, `service`,
  `environment`, `version`, `trace_id`, `request_id`. Log errors with context, not stack spam.
  **Never log personal data or secrets**: mask NIK/KTP, phone, email, bank/card numbers, tokens,
  passwords and authorization headers in the logger itself (a redaction hook), not at each call site.
- **Metrics** (RED for request paths, USE for resources):
  - `requests_total{route,method,status_class}`, `request_duration_seconds` histogram,
    `errors_total{route,type}`;
  - queue depth / consumer lag, job success/failure and duration for background work;
  - saturation: DB pool in use, worker utilization, memory, CPU.
  Keep label cardinality bounded: route templates, never raw URLs, user IDs or emails.
- **Traces**: propagate W3C `traceparent` across HTTP, queues and jobs; span outgoing DB, cache
  and HTTP calls.
- **Health**: `/healthz` (liveness: the process is up) and `/readyz` (readiness: dependencies
  reachable) — separate, cheap, unauthenticated, and leaking no internals.

## 2. SLIs and SLOs

Define per critical user journey (from the PRD / tech doc):

| SLI | Good event | Target (ASSUMPTION until the operator confirms) |
|---|---|---|
| Availability | non-5xx responses / all | 99.9% over 30 days |
| Latency | requests under the threshold / all | 95% < 300 ms |
| Freshness (jobs) | jobs finished within their window | 99% |

## 3. Alerts

- Alert on **symptoms customers feel** (SLO burn, error rate, latency, stuck queue), not on causes
  (CPU high) — causes go on dashboards.
- Prefer multi-window burn-rate alerts (fast: 1h/5m, slow: 6h/30m) for SLOs.
- Every alert has: severity (page vs ticket), a summary with the affected journey, the dashboard
  link and a `runbook_url`.
- Add an alert for "the service stopped reporting" (absent metrics / heartbeat) and for each
  scheduled job that did not run.
- Route: pages go to on-call, tickets to the team channel — write the routing as config if the
  repository holds it, otherwise list it for the operator.

## 4. Dashboards and runbooks as code

- One service dashboard: golden signals per route, SLO and error budget, dependencies, saturation,
  deploy markers.
- `docs/runbooks/<alert>.md` per alert: what it means, first checks (queries/links), mitigations
  (rollback, scale, disable flag), escalation.

## Guardrails

- Instrumentation must not change behavior: no blocking exports on the request path; failures to
  export are swallowed and counted.
- Add tests for the redaction hook (a log line with a fake NIK and token comes out masked) and for
  the health endpoints.
- Mark any metric/alert library version you add as **unverified** for the operator.

## Tools

| Tool | Need | Used for |
|---|---|---|
| `promtool` (Prometheus) | recommended | `promtool check rules` and `promtool test rules` on the alert rules |

For other vendors (Datadog, New Relic, CloudWatch), validate monitors or dashboards as code with the provider's own tooling (Terraform provider `plan`, or the vendor CLI) when it is in the repository. Install from the orchestrator's Tool Hub (**Launch Readiness** → *Install all missing*), the install prompt shown when the stage starts, or `go run ./cmd/daemon packages setup launch-readiness`. Never install software yourself from inside a stage: if a tool is missing, still wire it into the pipeline, give the exact command, and list it under operator actions.

## Output

```
## Signals added
## SLIs & SLOs
## Alerts
(name — condition — severity — route — runbook)
## Dashboards & runbooks
## Operator actions
(on-call routing, notification channels, vendor keys to configure)
## Follow-up tasks
(the `followups` JSON block from the stage brief: remaining gaps grouped into a few tasks)
## Summary
```
