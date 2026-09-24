# Phase 5 Execution Plan: Security, Deployment & E2E System Verification

**Document ID:** PLAN-005  
**Phase:** 5 of 5 (Weeks 13–16)  
**Status:** Approved for Implementation  
**Primary Focus:** Zero-Trust Security, Secret Scrubbing, Chaos Recovery, E2E Factory Verification, Docker Deployment  
**PRD References:** Section 7, Section 8, Section 10, Section 11  

---

## 1. Phase Overview & Objectives

Phase 5 hardens the Meta-Orchestrator for enterprise production:
1. **Zero-Trust Security & Secret Governance:** Automatic redaction of sensitive credentials from real-time WebSocket streams, terminal logs, and thought traces; container network sandboxing to prevent host-level escapes.
2. **End-to-End Autonomous Factory Verification:** Automated testing of the full multi-repo lifecycle—from initial ticket ingestion through ATDD test writing, write-locking, implementation, `.mp4` video evidence capture, and signed `EVIDENCE.md` audit packaging.
3. **Chaos Recovery & Resiliency Testing:** Validating process crash resumption (<5s RTO), Redis state recovery, and HITL frustration loop resolution.
4. **Production Packaging & Operator Tooling:** Multi-stage production container builds and the `meta-orch` CLI for operator management.

---

## 2. Work Breakdown Structure (WBS) & Tasks

```
Phase 5: Security, Deployment & E2E System Verification
├── Epic 5.1: Zero-Trust Security, Secret Vaulting & Isolation Hardening
│   ├── TASK-5.1.1: Secret Sanitization Engine & Real-Time Stream Scrubber
│   ├── TASK-5.1.2: Container Sandbox Network Isolation & Blast Radius Guards
│   └── TASK-5.1.3: Cryptographic Signature Verification & Tamper Detection
├── Epic 5.2: End-to-End System Testing & Chaos Recovery
│   ├── TASK-5.2.1: Full-Lifecycle Multi-Repo Autonomous E2E Test Suite
│   ├── TASK-5.2.2: Chaos Recovery & Crash Resumption Test Suite (<5s RTO)
│   └── TASK-5.2.3: Frustration Loop Trigger & HITL Recovery Validation
└── Epic 5.3: Production Packaging, Deployment & Operator CLI
    ├── TASK-5.3.1: Multi-Stage Production Dockerfile & Compose Orchestration
    ├── TASK-5.3.2: `meta-orch` Operator Management CLI
    └── TASK-5.3.3: Production Readiness & Disaster Recovery Verification
```

---

## 3. Detailed Task Specifications

### Epic 5.1: Zero-Trust Security, Secret Vaulting & Isolation Hardening

#### TASK-5.1.1: Secret Sanitization Engine & Real-Time Stream Scrubber
* **Objective:** Ensure no API tokens, SSH keys, passwords, or cloud credentials ever leak into agent thought streams, WebSocket events, terminal outputs, or saved artifacts.
* **Technical Scope:**
  - Build high-throughput stream filter scanning for regex patterns (AWS keys, OpenAI keys, GitHub tokens, Bearer tokens, private keys).
  - Scrub secrets dynamically in both outbound WebSocket broadcasts and persistent disk storage.
  - Integrate with external secret vault (e.g. HashiCorp Vault or environment injection) using ephemeral token mapping.
* **Deliverable Files:**
  - `internal/security/scrubber.go`
  - `internal/security/patterns.go`
  - `internal/security/scrubber_test.go`
* **Verification Criteria:**
  - Inject 20 distinct secret patterns into standard stdout; assert all patterns are redacted as `[REDACTED_SECRET]` before reaching WebSocket clients or logs.

---

#### TASK-5.1.2: Container Sandbox Network Isolation & Blast Radius Guards
* **Objective:** Confine all automated code generation and test execution strictly within ephemeral container namespaces with restricted host privileges.
* **Technical Scope:**
  - Mount Docker socket with read-only proxy or dedicated daemon to prevent container-escape privileges.
  - Configure isolated Docker bridge networks without direct access to host network or internal corporate metadata endpoints (`169.254.169.254`).
  - Enforce Linux cgroups CPU/memory caps on worker containers.
* **Deliverable Files:**
  - `internal/docker/security.go`
  - `internal/docker/network_guard.go`
* **Verification Criteria:**
  - Execute privileged command exploit attempt from within an agent sandbox; assert container cannot access host filesystem or internal metadata IP.

---

#### TASK-5.1.3: Cryptographic Signature Verification & Tamper Detection
* **Objective:** Implement cryptographic validation verifying the integrity of all artifacts before pull request authorization.
* **Technical Scope:**
  - Implement HMAC-SHA256 signature verifier for `EVIDENCE.md` frontmatter.
  - Calculate checksums of all referenced screenshots and `.mp4` video files.
  - Block PR merge approval if any artifact checksum fails verification.
* **Deliverable Files:**
  - `internal/security/verifier.go`
  - `internal/security/verifier_test.go`
* **Verification Criteria:**
  - Verify valid evidence passes check; alter 1 byte of recorded video file and assert verifier immediately flags tampering and blocks merge.

---

### Epic 5.2: End-to-End System Testing & Chaos Recovery

#### TASK-5.2.1: Full-Lifecycle Multi-Repo Autonomous E2E Test Suite
* **Objective:** Execute an automated end-to-end integration test validating the entire SDLC pipeline without human intervention on a standard feature.
* **Technical Scope:**
  - Ingest mock feature ticket across two linked repositories (frontend Vue + backend Go/Laravel).
  - Validate pipeline execution through:
    1. Scope profiling & token budget allocation.
    2. Ephemeral workspace creation & auto-symlinking.
    3. ATDD test suite generation & verified Red Phase failure.
    4. Write-lock release & implementation code generation.
    5. Green Phase pass, `.mp4` video recording, and screenshot capture.
    6. Signed `EVIDENCE.md` and `UAT_PREPARATION.md` generation.
* **Deliverable Files:**
  - `tests/e2e/autonomous_pipeline_test.go`
  - `tests/e2e/fixtures/mock_ticket.json`
* **Verification Criteria:**
  - Full E2E suite passes in headless CI with exit code 0; verifies generated artifacts are 100% compliant with schemas.

---

#### TASK-5.2.2: Chaos Recovery & Crash Resumption Test Suite (<5s RTO)
* **Objective:** Subject the orchestrator daemon to simulated system crashes, network partitions, and resource exhaustion to verify the <5s RTO SLA.
* **Technical Scope:**
  - Automatically terminate daemon with `SIGKILL` at randomized points across each phase.
  - Restart daemon process and measure duration to state resumption.
  - Assert no duplicate token consumption, no broken git working trees, and intact Redis checkpoint restoration.
* **Deliverable Files:**
  - `tests/chaos/crash_recovery_test.go`
  - `tests/chaos/harness.go`
* **Verification Criteria:**
  - 10 repeated crash-and-restart cycles recover successfully with average RTO `< 3.2s` (under 5s target).

---

#### TASK-5.2.3: Frustration Loop Trigger & HITL Recovery Validation
* **Objective:** Validate system response when encountering 3 consecutive test failures and prove successful recovery via human context injection.
* **Technical Scope:**
  - Inject deliberately broken code causing repetitive test failure.
  - Assert daemon halts at attempt 3, sets state `BLOCKED_FRUSTRATION`, pauses token burn, and renders UI alert.
  - Simulate human operator submitting corrective instruction via WebSocket `hitl.inject`.
  - Assert agent applies guidance, re-runs tests, and successfully transitions to Green Phase.
* **Deliverable Files:**
  - `tests/e2e/frustration_recovery_test.go`
* **Verification Criteria:**
  - Loop halts on attempt 3 without exceeding token limit; injection of corrective prompt allows clean resumption and eventual pass.

---

### Epic 5.3: Production Packaging, Deployment & Operator CLI

#### TASK-5.3.1: Multi-Stage Production Dockerfile & Compose Orchestration
* **Objective:** Package the Meta-Orchestrator for enterprise self-hosted deployment.
* **Technical Scope:**
  - Multi-stage Dockerfile for Go daemon (distroless/scratch base).
  - Multi-stage Dockerfile for Vue 3 UI (Vite build + Nginx alpine).
  - Production `docker-compose.prod.yml` configuring Redis, PostgreSQL, daemon worker, web frontend, and Docker-in-Docker socket proxy.
* **Deliverable Files:**
  - `Dockerfile.daemon`
  - `Dockerfile.web`
  - `docker-compose.prod.yml`
  - `nginx/default.conf`
* **Verification Criteria:**
  - Run `docker compose -f docker-compose.prod.yml up -d`; verify all containers start healthy and UI is accessible on port 80/443.

---

#### TASK-5.3.2: `meta-orch` Operator Management CLI & Tool Lifecycle Runner
* **Objective:** Build a command-line tool for system administrators to inspect daemon health, manage active workspaces, and execute step-by-step tool installations, updates, rollbacks, and diagnostic health checks.
* **Technical Scope:**
  - Implement CLI in Go under `cmd/cli/` using Cobra.
  - Task & Workspace Commands:
    - `meta-orch tasks list`
    - `meta-orch tasks inspect <task-id>`
    - `meta-orch tasks reset <task-id>`
    - `meta-orch registries reload`
    - `meta-orch secrets inject <key> <val>`
  - Tool & Dependency Lifecycle Commands:
    - `meta-orch tools list`: Display all registered tools, install status, version, and health.
    - `meta-orch tools install <tool> [--version <ver>] [--best-fit]`: Interactive step-by-step installation wizard with pre-flight checks and live progress bar.
    - `meta-orch tools update <tool> [--latest]`: Atomic version upgrade with backward-compatibility checks.
    - `meta-orch tools rollback <tool> [--to <version>]`: Instant rollback to previous known-good working version.
    - `meta-orch tools doctor [--best-fit]`: Diagnose environment compatibility, verify symlinks, and suggest/apply best-fit version matrix.
  - AI Provider & Cascading Config Commands:
    - `meta-orch config get [--project <path>]`: View computed cascading config showing inheritance tree.
    - `meta-orch config set <key> <val> [--project <path>]`: Set global or project-specific override (`.sdlc/config.yaml`).
    - `meta-orch providers test [<provider>]`: Ping provider APIs (Claude, Antigravity, ChatGPT, OpenCode) and output latency and model health status.
  - Custom Workflow & Multi-Source Registry Commands:
    - `meta-orch workflows list`: Display available built-in and project-level custom workflows.
    - `meta-orch workflows validate <workflow.yaml>`: Validate schema of custom SDLC definition.
    - `meta-orch skills add <repo-url> [--branch <branch>]`: Register and sync a remote Git skill repository.
    - `meta-orch skills list [--sources]`: List all discovered skills across Remote, Local, System, and Built-in tiers.
    - `meta-orch hooks list [--project <path>]`: Display active lifecycle hooks and trigger points.
    - `meta-orch hooks test <event_name>`: Dry-run lifecycle hook execution with mock context.
* **Deliverable Files:**
  - `cmd/cli/main.go`
  - `internal/cli/tasks.go`
  - `internal/cli/tools.go`
  - `internal/cli/config.go`
  - `internal/cli/registries.go`
  - `internal/cli/workflows.go`
  - `internal/cli/hooks.go`
* **Verification Criteria:**
  - Run `meta-orch skills list --sources` and `meta-orch workflows validate .sdlc/workflow.yaml`; verify CLI correctly lists multi-source skills and validates custom SDLC workflow schemas.

---

#### TASK-5.3.3: Production Readiness & Disaster Recovery Verification
* **Objective:** Final sign-off audit verifying all PRD Non-Functional Requirements (NFRs), security controls, and disaster recovery runbooks.
* **Technical Scope:**
  - Run benchmark confirming 10 parallel multi-repo workspaces run concurrently on 16 vCPU / 64GB RAM node.
  - Document operational Disaster Recovery Runbook (`DR_RUNBOOK.md`).
  - Document system operator manual (`OPERATOR_GUIDE.md`).
* **Deliverable Files:**
  - `docs/OPERATOR_GUIDE.md`
  - `docs/DR_RUNBOOK.md`
  - `docs/COMPLIANCE_SIGN_OFF.md`
* **Verification Criteria:**
  - Performance test certifies 10 concurrent workspaces run within resource limits; documentation review confirms all operational steps are reproducible.
