---
name: launch-load-resilience
description: "Launch readiness step 6 — test with heavy traffic and prepare for problems. Writes load tests as code with pass/fail thresholds tied to the SLOs (baseline, stress, spike, soak), checks timeouts, retries, circuit breakers, graceful shutdown and backpressure, and prepares incident readiness: runbooks, rollback drill, failure-mode table, capacity plan and on-call handover. Built into the orchestrator."
---

# launch-load-resilience — test with heavy traffic and prepare for problems

Goal: know the service's limits **before** customers find them, and make sure that when something
breaks, it breaks small and the on-call engineer knows what to do. Use the SLOs from the
observability step and the API surface from the customer-API step.

## 1. Load model

From the PRD, tech doc, or the operator: expected peak requests per second per critical journey,
data volume, concurrency, and growth. State each number with its source, or mark it ASSUMPTION
(and use 3× expected peak as the stress target).

## 2. Load tests as code

Use the tool the organization already uses (look for k6, Gatling, Locust, JMeter, Artillery,
Vegeta scripts); otherwise k6, with scripts in `loadtest/` and a README on how to run them.

| Test | Shape | Proves |
|---|---|---|
| Baseline | expected peak, 10–15 min | SLOs hold at normal peak |
| Stress | ramp to 2–3× peak until failure | where it breaks and how (graceful 429/503, not timeouts and crashes) |
| Spike | 0 → peak in seconds | autoscaling / queueing absorb bursts |
| Soak | ~70% peak, hours | no memory leaks, connection or disk exhaustion |

- Thresholds encode the SLOs (for example `http_req_failed < 0.1%`, `p(95) < 300ms`) so the run
  passes or fails by itself.
- Realistic data: varied IDs, realistic payload sizes, auth tokens per virtual user — **synthetic
  data only**, never real customer data.
- Run against staging or a dedicated performance environment, **never production**, and only when
  the operator approves running it. If you cannot run it, deliver ready-to-run scripts and the
  exact command.

## 3. Resilience review — record each with file:line

- **Timeouts** on every outbound call (DB, cache, HTTP, queue), shorter than the caller's.
- **Retries** only on idempotent operations, with exponential backoff and jitter, capped.
- **Circuit breakers / bulkheads** around flaky dependencies; a fallback or a clear error.
- **Backpressure**: bounded queues and worker pools, load shedding with 429/503 instead of
  unbounded memory.
- **Connection pools** sized for the target concurrency.
- **Graceful shutdown**: stop accepting, drain in-flight requests and jobs, then exit; readiness
  goes false first.
- **Dependency failure**: what happens when the DB, cache or a third party is slow or down?

## 4. Prepare for problems

- **Failure-mode table**: failure — detection (which alert) — customer impact — mitigation — owner.
- **Runbooks** (`docs/runbooks/`): rollback, scale up/down, disable a feature flag, drain a queue,
  rotate a leaked credential, restore from backup.
- **Rollback drill**: steps to deploy the previous version on staging and the result (or the steps
  for the operator to run).
- **Backups & restore**: what is backed up, retention, and when a restore was last tested.
- **Capacity plan**: resources needed for expected peak and stress target; autoscaling limits.
- **Incident process**: severity levels, who is on call, communication channel, status page, and
  the post-incident review template. For incidents involving personal data, note that the
  organization's data-breach notification procedure applies — do not state deadlines unless they
  come from a provided source.

## Tools

| Tool | Need | Used for |
|---|---|---|
| `k6` | **required** | load tests with thresholds (`k6 run loadtest/<test>.js`) |
| `docker` | recommended | dependencies for a staging-like run |

Running a load test is always the operator's call, against staging or a performance environment, never production. Install from the orchestrator's Tool Hub (**Launch Readiness** → *Install all missing*), the install prompt shown when the stage starts, or `go run ./cmd/daemon packages setup launch-readiness`. Never install software yourself from inside a stage: if a tool is missing, still wire it into the pipeline, give the exact command, and list it under operator actions.

## Output

```
## Load model
## Load tests
(test — command — thresholds — result or "not run: <why>")
## Findings
(bottleneck or resilience gap — file:line — impact — fix)
## Failure modes
## Runbooks & drills
## Capacity plan
## Operator actions
## Follow-up tasks
(the `followups` JSON block from the stage brief: remaining gaps grouped into a few tasks)
## Summary
```
