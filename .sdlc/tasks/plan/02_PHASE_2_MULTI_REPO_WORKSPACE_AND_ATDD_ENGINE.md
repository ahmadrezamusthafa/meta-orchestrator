# Phase 2 Execution Plan: Multi-Repo Workspace & ATDD Engine

**Document ID:** PLAN-002  
**Phase:** 2 of 5 (Weeks 5–8)  
**Status:** Approved for Implementation  
**Primary Focus:** Ephemeral Sandboxing, Auto-Symlinks, Write-Locking, ATDD Generation, Rich Media Evidence  
**PRD References:** Pillar 3, Pillar 4, Section 6.2, Section 7, Section 11 (Phase 2)  

---

## 1. Phase Overview & Objectives

Phase 2 builds the core execution mechanics of the **Zero-Trust Software Factory**:
1. **Multi-Repository Sandboxing:** Dynamically cloning frontend, backend, and contracts into isolated `/workspaces/{task_id}/` sandboxes with Docker Compose networks.
2. **Auto-Symlinking:** Real-time rewriting and filesystem linking of cross-repository dependencies (`package.json`, `composer.json`, `go.mod`) with collision detection.
3. **Shift-Left ATDD & Strict Write-Locking:** QA Agent generates comprehensive executable tests first; developer agents are cryptographically write-locked out of application source code until the test suite is verified to fail (Red Phase).
4. **Rich Media Evidence Engine:** Autonomous capture of Playwright `.mp4` video recordings, high-res milestone screenshots, human-readable `UAT_PREPARATION.md`, and signed `EVIDENCE.md` audit logs.

---

## 2. Work Breakdown Structure (WBS) & Tasks

```
Phase 2: Multi-Repo Workspace & ATDD Engine
├── Epic 2.1: Multi-Repository Ephemeral Workspace Engine
│   ├── TASK-2.1.1: Ephemeral Workspace Provisioner & Sandbox Manager
│   ├── TASK-2.1.2: Dynamic Docker Compose Orchestrator, Isolation Guard & Polyglot Skill Harness
│   ├── TASK-2.1.3: Auto-Symlink Dependency Resolver & Collision Detector
│   ├── TASK-2.1.4: Cross-Repository Atomic Git Branch & Commit Coordinator
│   └── TASK-2.1.5: Mid-Process Workspace Hydrator & External Slice Ingestion Engine
├── Epic 2.2: "Shift-Left" ATDD Engine & Write-Locking Protocol
│   ├── TASK-2.2.1: Shift-Left ATDD Test Generator (Codebase & PRD Grounded)
│   ├── TASK-2.2.2: OS & Filesystem Strict Write-Locking Guard & Polyglot Skill Interceptor
│   ├── TASK-2.2.3: Red-Phase Automated Test Failure Verifier
│   ├── TASK-2.2.4: Autonomous Human-Readable `UAT_PREPARATION.md` Generator
│   └── TASK-2.2.5: Tech Doc / RFC (`TECH_DOC_RFC.md`) & Atomic Task Breakdown Engine
└── Epic 2.3: Rich Media Evidence Engine & Cryptographic Signer
    ├── TASK-2.3.1: Playwright Headless/Headful Video Recording Harness (.mp4)
    ├── TASK-2.3.2: High-Resolution Viewport Screenshot Capture Service
    └── TASK-2.3.3: Tamper-Evident `EVIDENCE.md` Manifest & SHA-256 Signer
```

---

## 3. Detailed Task Specifications

### Epic 2.1: Multi-Repository Ephemeral Workspace Engine

#### TASK-2.1.1: Ephemeral Workspace Provisioner & Dynamic Repo Cloner
* **Objective:** Dynamically clone all repositories identified by the Dynamic Repository Discovery Engine into an isolated task workspace under `/workspaces/{task_id}/[repo_name]` with automated lifecycle cleanup.
* **Technical Scope:**
  - Consume identified repository list from `TaskProfile` (e.g. `frontend-portal`, `backend-core`, `api-contracts`).
  - Implement workspace manager in `internal/workspace/provisioner.go`.
  - Securely clone all identified repositories using SSH keys/tokens from secrets manager.
  - Create volume mounts and isolated task scratch directories.
  - Integrate with Lifecycle Hooks Engine (`TASK-1.2.7`): execute `pre-stage` hooks (e.g. workspace prep, custom environment provisioning) upon workspace initialization.
  - Implement workspace teardown routine with option for ephemeral retention or instant purge.
* **Deliverable Files:**
  - `internal/workspace/provisioner.go`
  - `internal/workspace/workspace.go`
  - `internal/workspace/provisioner_test.go`
* **Verification Criteria:**
  - Provision a workspace with dynamically identified repositories; verify directory tree is created with proper permissions, pre-stage hooks fire, and teardown cleanly removes disk allocations.

---

#### TASK-2.1.2: Dynamic Docker Compose Orchestrator, Isolation Guard & Polyglot Skill Harness
* **Objective:** Spin up dedicated Docker Compose networks with required backing services (PostgreSQL, MySQL, Redis, Mailpit, Mock APIs) per task, and mount the Universal Polyglot Skill Runtime supporting BMAD, Claude, Superpower, and MCP tools inside the container sandbox.
* **Technical Scope:**
  - Generate ephemeral `docker-compose.generated.yml` with unique port mappings, isolated bridge networks, and memory limits.
  - Mount and provision Polyglot Skill execution harnesses into the workspace container:
    - Bind mounts for Claude `SKILL.md` directories and execution scripts.
    - BMAD multi-agent persona skill runners.
    - Superpower bare-metal/container execution environments with restricted capability boundaries.
    - Model Context Protocol (MCP) client bridge enabling container agents to communicate with host/network MCP servers.
  - Healthcheck waiting loop ensuring database, backing services, and skill runtimes are fully healthy before agent execution begins.
  - Restrict network egress to prevent unauthorized external data exfiltration.
* **Deliverable Files:**
  - `internal/docker/compose.go`
  - `internal/docker/templates/base-compose.yml.tmpl`
  - `internal/docker/skill_mounts.go`
  - `internal/docker/healthcheck.go`
* **Verification Criteria:**
  - Spawn Docker Compose environment; assert container healthiness via Ping, verify polyglot skill mounts are accessible, and verify network isolation blocks non-whitelisted outbound requests.

---

#### TASK-2.1.3: Auto-Symlink Dependency Resolver & Collision Detector
* **Objective:** Automatically detect and resolve cross-repo dependencies so changes in backend/contracts are immediately accessible to frontend test suites without remote package releases.
* **Technical Scope:**
  - Parse `package.json` (npm/yarn/pnpm), `composer.json` (PHP), and `go.mod` across all cloned repositories in the workspace.
  - Establish relative filesystem symlinks or temporary manifest rewrites (e.g. `file:../contracts` or Composer local repository path).
  - Detect version collisions (e.g., frontend requires v2.1 while backend specifies v3.0); abort auto-symlink and raise structured warning if incompatible.
* **Deliverable Files:**
  - `internal/symlink/resolver.go`
  - `internal/symlink/detector_npm.go`
  - `internal/symlink/detector_composer.go`
  - `internal/symlink/collision.go`
  - `internal/symlink/resolver_test.go`
* **Verification Criteria:**
  - Test fixture with linked frontend and shared component package; verify symlinks are established and build succeeds; verify collision detection raises error on version mismatch.

---

#### TASK-2.1.4: Cross-Repository Atomic Git Branch & Commit Coordinator
* **Objective:** Coordinate feature branch creation, atomic commits, and pull requests across multiple repositories simultaneously.
* **Technical Scope:**
  - Create unified branch convention: `feat/{task-id}-{slug}` across all touched repositories.
  - Create synchronized git checkpoints across repositories to allow multi-repo rollback.
  - Generate coordinated GitHub/GitLab Pull Requests linking upstream and downstream branches.
* **Deliverable Files:**
  - `internal/git/coordinator.go`
  - `internal/git/atomic_commit.go`
  - `internal/git/pr_generator.go`
* **Verification Criteria:**
  - Perform test commit across 2 repositories; assert matching commit messages, branch naming, and atomic rollback on simulated failure in repository 2.

---

#### TASK-2.1.5: Mid-Process Workspace Hydrator & External Slice Ingestion Engine
* **Objective:** Enable operators to launch tasks at arbitrary intermediate SDLC stages (e.g. Stage 7 Implementation through Stage 8 E2E Automation) by ingesting pre-existing branches, external `TASK_PLAN.md`, or synthesized context stubs without re-running earlier stages.
* **Technical Scope:**
  - Integrate with `slice_hydrator.go` (`TASK-1.1.2`): check `ExecutionSlice.StartStage` and `ExecutionSlice.HaltStage`.
  - Ingestion Modes:
    - Pre-existing git feature branch: checkout remote/local branch `git checkout {source_branch}` instead of fresh branch initialization.
    - External `TASK_PLAN.md` or PRD file: parse uploaded markdown, validate against plan schema, and hydrate into orchestrator task store (`.sdlc/artifacts/{task_id}/TASK_PLAN.md`).
    - Fallback Context Synthesizer: if upstream artifacts (`PRD.md`, `TECH_DOC_RFC.md`) are absent when starting at Stage 7, synthesize minimal schema-compliant stubs to satisfy downstream validators.
  - Direct Stage Routing: bypass Stages 1–6 (PRD creation, ATDD red-phase lock, RFC gates) and directly invoke the method recommended for the target stage (e.g., BMAD or ReAct for Implementation) with unlocked filesystem write permissions.
  - Sliced Lifecycle Clean-up: when `HaltStage` is reached (e.g., Stage 8 E2E Automation), cleanly terminate execution without attempting GitHub PR creation or release sign-off.
* **Deliverable Files:**
  - `internal/workspace/slice_ingestion.go`
  - `internal/workspace/slice_synthesizer.go`
  - `internal/workspace/slice_ingestion_test.go`
* **Verification Criteria:**
  - Launch task with `StartStage = Stage 7 (Implementation)` and `HaltStage = Stage 8 (E2E Automation)` providing an external task plan and existing branch; verify upstream stages are bypassed, write-locks are immediately disengaged, implementation agents edit code, Playwright E2E tests run, and execution halts cleanly after test execution.

---

### Epic 2.2: "Shift-Left" ATDD Engine & Write-Locking Protocol

#### TASK-2.2.1: Shift-Left ATDD Test Generator (Codebase & PRD Grounded)
* **Objective:** Autonomously generate executable end-to-end and integration test suites by deeply analyzing both the newly created `PRD.md` and the existing codebase AST before any implementation code is written.
* **Technical Scope:**
  - QA Agent queries AST Sharder to inspect existing UI routes, controllers, and database models across identified repositories.
  - Cross-references acceptance criteria from `.sdlc/artifacts/{task_id}/PRD.md`.
  - Generates cross-repository Playwright TypeScript tests (`*.spec.ts`), Laravel Dusk tests, or Cypress specs.
  - Commits test suite to repository test suites and records mapping in `.sdlc/artifacts/{task_id}/ATDD_SUITE.md`.
* **Deliverable Files:**
  - `internal/atdd/generator.go`
  - `internal/atdd/playwright_template.go`
  - `internal/atdd/generator_test.go`
* **Verification Criteria:**
  - Input sample feature spec + existing repository AST; assert generated Playwright tests accurately reference existing page selectors and assert new feature criteria before implementation exists.

---

#### TASK-2.2.2: OS & Filesystem Strict Write-Locking Guard & Polyglot Skill Interceptor
* **Objective:** Programmatically prevent developer agents from modifying application source files during the initial ATDD generation phase, intercepting file mutations across all supported skill standards (BMAD, Claude `SKILL.md`, Superpower, MCP, and native tools).
* **Technical Scope:**
  - Enforce read-only filesystem attributes (`chmod -R a-w` or Linux mount namespace read-only binding) on `src/`, `app/`, `lib/` in target repositories.
  - Polyglot Skill Execution Interceptor:
    - Native file editing tools (`write_to_file`, `replace_file_content`, file patch tools).
    - Claude Skills: inspects command scripts in `scripts/` and rejects writes to locked workspace directories.
    - Superpower Skills: wraps bash/container commands with path access guards, preventing direct writes to source trees.
    - Model Context Protocol (MCP) filesystem tools: intercepts MCP tool calls (`write_file`, `edit_file`) and enforces lock policies.
    - Bash/Shell redirection: sanitizes shell execution to block `sed -i`, `echo >`, and file overwrite patterns.
  - Release write-locks only after explicit cryptographic authorization event (`event.atdd_red_verified`) from the Red-Phase verifier.
* **Deliverable Files:**
  - `internal/security/writelock.go`
  - `internal/security/polyglot_interceptor.go`
  - `internal/security/writelock_test.go`
* **Verification Criteria:**
  - Attempt file modification via Claude skill, BMAD developer tool, Superpower shell tool, and MCP file edit while locked; assert all 4 operations are intercepted and rejected with `ErrWriteLockActive`.

---

#### TASK-2.2.3: Red-Phase Automated Test Failure Verifier
* **Objective:** Execute newly generated ATDD test suites inside the container environment and prove that tests fail with expected error codes before unlocking source code.
* **Technical Scope:**
  - Run Playwright test runner against target container environment.
  - Assert that tests fail (Exit code $\ne 0$) due to missing features (HTTP 404, missing UI selector), not test harness syntax errors.
  - Log failure traces, record Red Phase verification timestamp, and emit `event.atdd_red_verified` to trigger write-lock release.
* **Deliverable Files:**
  - `internal/atdd/runner.go`
  - `internal/atdd/verifier.go`
* **Verification Criteria:**
  - Execute runner against un-implemented app; assert test fails with expected failure signature, emits `AWAITING_CODE_GEN` status, and triggers write-lock release.

---

#### TASK-2.2.4: Autonomous Human-Readable `UAT_PREPARATION.md` Generator
* **Objective:** Produce a structured, plain-language User Acceptance Testing manual for human business stakeholders.
* **Technical Scope:**
  - Generate `.sdlc/artifacts/{task_id}/UAT_PREPARATION.md`.
  - Content sections:
    - Feature summary and business objectives.
    - Test credentials, roles, and seeded data details.
    - Step-by-step user click paths and interaction flows.
    - Expected visual states, validations, and edge cases.
    - Clear pass/fail verification rubrics.
* **Deliverable Files:**
  - `internal/uat/generator.go`
  - `internal/uat/templates/uat_preparation.md.tmpl`
* **Verification Criteria:**
  - Validate output markdown against schema; confirm all required sections (credentials, click paths, rubrics) are populated and formatted.

---

#### TASK-2.2.5: Tech Doc / RFC (`TECH_DOC_RFC.md`) & Atomic Task Breakdown Engine
* **Objective:** Author comprehensive technical architecture documentation, enforce human approval gate, and produce sequenced atomic implementation work breakdown.
* **Technical Scope:**
  - Architect Agent generates `.sdlc/artifacts/{task_id}/TECH_DOC_RFC.md`:
    - Database migrations, entity relationships, and API contract specifications (OpenAPI / Protobuf).
    - Sequence diagrams and cross-repository data flows.
    - AST impact radius across all dynamically identified repositories.
  - State machine halts for `GATE_TECH_DOC_REVIEW`: requires human approval via UI before implementation proceeds.
  - Planner Agent converts approved RFC into `.sdlc/artifacts/{task_id}/TASK_PLAN.md`:
    - Ordered atomic task items with dependency graphs and persona assignments.
* **Deliverable Files:**
  - `internal/techdoc/rfc_generator.go`
  - `internal/techdoc/task_breakdown.go`
  - `internal/techdoc/rfc_generator_test.go`
* **Verification Criteria:**
  - Generate RFC and Task Plan from sample PRD; assert RFC includes schemas and migrations; assert task breakdown creates topologically sorted tasks with persona assignments.

---

### Epic 2.3: Rich Media Evidence Engine & Cryptographic Signer

#### TASK-2.3.1: Playwright Headless/Headful Video Recording Harness (.mp4)
* **Objective:** Capture full `.mp4` video recordings of the automated Playwright test execution mimicking human user journeys, supporting both full SDLC pipelines and sliced mid-process runs (e.g., Task Implementation through E2E Validation with `.mp4` video output).
* **Technical Scope:**
  - Configure Playwright browser context with video capture: `recordVideo: { dir: '.sdlc/artifacts/{task_id}/videos/', size: { width: 1280, height: 720 } }`.
  - Headful / Virtual Framebuffer Support: Execute inside container using `xvfb-run` or headless Chromium with high-fidelity rendering.
  - Mid-Process Sliced Run Handling: When invoked via a partial slice (e.g. `StartStage=TASK_IMPLEMENTATION`, `HaltStage=E2E_AUTOMATION`), immediately bind the test harness to the current workspace code, execute end-to-end user journeys, and capture the complete browser viewport.
  - Post-processing & Transcoding: Invoke ffmpeg transcoder to convert raw WebM/IVF output into web-standard, fast-start H.264 `.mp4` (`-c:v libx264 -pix_fmt yuv420p -movflags +faststart`) stored at `.sdlc/artifacts/{task_id}/videos/run_final.mp4`.
  - Real-time Event Notification: Emit WebSocket event `evidence.video.ready` with URL `/api/v1/artifacts/{task_id}/video` so the Vue 3 Evidence Vault player auto-loads the video upon completion.
* **Deliverable Files:**
  - `internal/evidence/video_recorder.go`
  - `internal/evidence/transcoder.go`
  - `internal/evidence/video_recorder_test.go`
* **Verification Criteria:**
  - Run Playwright suite with video enabled on both a full run and a sliced Stage 7–8 run; verify `.mp4` file is generated, non-zero byte size, playable via standard HTML5 `<video>` tag, and accessible via the artifact REST endpoint.

---

#### TASK-2.3.2: High-Resolution Viewport Screenshot Capture Service
* **Objective:** Capture high-resolution viewport screenshots at critical interaction milestones, form submissions, and error states.
* **Technical Scope:**
  - Instrument Playwright assertions to capture screenshots on milestone triggers and failure boundaries.
  - Store images with metadata tags (`step_checkout_submit.png`, `step_payment_confirmed.png`).
  - Generate side-by-side visual diff against baseline screenshots if available.
* **Deliverable Files:**
  - `internal/evidence/screenshot_capture.go`
  - `internal/evidence/visual_diff.go`
* **Verification Criteria:**
  - Run test flow; assert at least 3 milestone screenshots are captured, indexed, and stored in the artifact vault.

---

#### TASK-2.3.3: Tamper-Evident `EVIDENCE.md` Manifest & SHA-256 Signer
* **Objective:** Compile an immutable, cryptographically signed audit manifest incorporating all test results, commit hashes, and media URLs.
* **Technical Scope:**
  - Generate YAML frontmatter and markdown body for `EVIDENCE.md` as specified in PRD Section 6.2:
    - `task_id`, `timestamp`, `method_used`, touched repositories and commit SHAs.
    - `atdd_verification` test counts and duration.
    - `media_artifacts` video URL and screenshot lists.
    - `uat_document` reference.
  - Compute SHA-256 HMAC cryptographic signature using orchestrator daemon secret key.
  - Append signature to manifest and record in immutable database log.
* **Deliverable Files:**
  - `internal/evidence/manifest.go`
  - `internal/evidence/signer.go`
  - `internal/evidence/manifest_test.go`
* **Verification Criteria:**
  - Generate manifest; verify SHA-256 signature recalculation matches signature value; verify tampering with 1 character invalidates cryptographic check.
