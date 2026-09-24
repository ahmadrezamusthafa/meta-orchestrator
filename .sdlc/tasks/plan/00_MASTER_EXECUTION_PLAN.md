# Master Execution Plan: AI-Driven SDLC Meta-Orchestrator

**Document ID:** PLAN-000  
**Status:** Approved for Implementation  
**Target Milestone:** V1.0 Core Platform & Autonomous Factory  
**Reference Documents:**  
- PRD: [PRD-001](../../prds/PRD001.md)  
- UI Specification: [UI_SPEC_001](../../specs/UI_SPEC_001.md)  
**Execution Horizon:** 16-Week Multi-Phase Roadmap  

---

## 1. Executive Program Architecture

The **AI-Driven SDLC Meta-Orchestrator** is an autonomous, event-driven software development factory designed to automate complex, multi-repository feature lifecycles under a zero-trust software factory paradigm.

This Master Execution Plan decomposes the system into **5 discrete execution phases**, comprising **19 Epics** and **79 granular, testable tasks**. Every task strictly aligns with the 7 architectural pillars defined in **PRD-001** and the UI/UX tokens, components, and edge cases defined in **UI_SPEC_001**.

```
┌────────────────────────────────────────────────────────────────────────────────────────┐
│                                  MASTER WORKSTREAM PHASING                             │
├────────────────────┬────────────────────┬────────────────────┬─────────────────────────┤
│ Phase 1: W1 - W4   │ Phase 2: W5 - W8   │ Phase 3: W9 - W12  │ Phase 4 & 5: W13 - W16  │
│ [COMPLETED 100%]   │ [TO BE IMPLEMENTED]│ [TO BE IMPLEMENTED]│ [TO BE IMPLEMENTED]     │
│ Foundation, Daemon │ Multi-Repo Sandboxes│ Vue 3 Mission Ctrl │ Telemetry, Shadow Engine│
│ Registries, AST    │ ATDD Write-Locks   │ HITL & Evidence    │ Security & Verification │
└────────────────────┴────────────────────┴────────────────────┴─────────────────────────┘
```

### Program Phase Execution Status

| Phase | Title | Epics | Tasks | Status |
|---|---|---|---|:---:|
| **Phase 1** | Foundation Daemon, Registries & AST Core | Epic 1.1, 1.2, 1.3 | 18 Tasks | **`COMPLETED & 100% VERIFIED`** |
| **Phase 2** | Multi-Repo Workspace & ATDD Engine | Epic 2.1, 2.2, 2.3 | 16 Tasks | **`TO BE IMPLEMENTED (Next Up)`** |
| **Phase 3** | Frontend Mission Control & HITL | Epic 3.1, 3.2, 3.3, 3.4, 3.5, 3.6 | 21 Tasks | **`TO BE IMPLEMENTED`** |
| **Phase 4** | Telemetry, Shadow Benchmarking & Routing | Epic 4.1, 4.2, 4.3, 4.4 | 12 Tasks | **`TO BE IMPLEMENTED`** |
| **Phase 5** | Security, Deployment & Verification | Epic 5.1, 5.2, 5.3 | 12 Tasks | **`TO BE IMPLEMENTED`** |
| **Total** | **Full Meta-Orchestrator Platform** | **19 Epics** | **79 Tasks** | **23% Platform Complete** |

---

## 2. Requirements Traceability Matrix

| PRD Pillar / UI Spec Section | Scope Item | Covered in Phase & Task | Primary Deliverable |
|---|---|---|---|
| **PRD Pillar 1** | Dynamic Task Profiler & Method Router | Phase 1 (EPIC-1.3) | `TaskProfile` generator, Router FSM |
| **PRD Pillar 1 / Pillar 7 / UI Spec 1.5** | Granular Method Benchmarking & Complexity Matching | Phase 1 (EPIC-1.3), Phase 4 (EPIC-4.3) | `best_methods_matrix.json`, Shadow Benchmark Runner |
| **PRD Sec 4.2 / UI Spec 1.2** | Mid-Process Execution & Stage Slicing | Phase 1 (EPIC-1.1), Phase 2 (EPIC-2.1), Phase 3 (EPIC-3.2) | FSM slice hydrator, `NewTaskModal` stage range selector |
| **PRD Sec 4.3 / UI Spec 1.2, 1.6** | Custom SDLC Workflow Architecture (`workflow.yaml` & Dynamic Kanban) | Phase 1 (EPIC-1.1), Phase 3 (EPIC-3.2, 3.6), Phase 5 (EPIC-5.3) | Custom FSM engine, Workflow Builder UI, Dynamic Columns |
| **PRD Pillar 2 / UI Spec 1.7** | Multi-Source Polyglot Registries (BMAD, Claude, Superpower, MCP, Hooks) | Phase 1 (EPIC-1.2), Phase 2 (EPIC-2.1), Phase 3 (EPIC-3.6), Phase 5 (EPIC-5.3) | Universal skill adapter, Hooks engine, Registries Hub UI |
| **PRD Pillar 1 / Pillar 3** | Dynamic Multi-Repo Identification Engine | Phase 1 (EPIC-1.3), Phase 2 (EPIC-2.1) | AST import graph & manifest cross-repo resolver |
| **PRD Pillar 1** | AST-Based Context Sharding | Phase 1 (EPIC-1.3) | Tree-sitter parsers (TS, PHP, Py, Go) |
| **PRD Pillar 1** | Artifact-Driven Memory Protocol | Phase 1 (EPIC-1.3) | `.sdlc/artifacts/{task_id}/` (`PRD.md`, `TECH_DOC_RFC.md`, etc.) |
| **PRD Pillar 1** | Cost-Optimized Model Routing | Phase 1 (EPIC-1.3) | Tier 1/2/3 model dispatch & token budgeters |
| **PRD Pillar 1 / UI Spec 1.5, 2.12** | Dual-Strategy Router (Best Practice vs. Custom Router) & Explainer | Phase 1 (EPIC-1.3), Phase 3 (EPIC-3.6), Phase 4 (EPIC-4.2) | Heuristic engine, Rule-based custom router, Explainer pill |
| **PRD Pillar 1 / UI Spec 1.5, 2.11** | Multi-Provider AI Abstraction (Claude, Antigravity, ChatGPT, OpenCode) & Cascading Config | Phase 1 (EPIC-1.3), Phase 3 (EPIC-3.6), Phase 5 (EPIC-5.3) | Provider drivers, `.sdlc/config.yaml` resolver, Provider Settings UI |
| **PRD Pillar 2** | Modular Profile Registry | Phase 1 (EPIC-1.2) | `profiles.json` JSON-Schema engine |
| **PRD Pillar 2** | Method FSM Registry | Phase 1 (EPIC-1.2) | `methods.json` (BMAD, Supervisor, ReAct, Superpower) |
| **PRD Pillar 2 / UI Spec 1.4, 2.10** | Tool & Dependency Lifecycle Manager (Install, Update, Rollback, Best-Fit) | Phase 1 (EPIC-1.2), Phase 3 (EPIC-3.5), Phase 5 (EPIC-5.3) | `tools.json`, Tool Hub UI, Step-by-step Wizard, `meta-orch tools` CLI |
| **PRD Pillar 3** | Ephemeral Workspace Engine & Sliced Ingestion | Phase 2 (EPIC-2.1) | Isolated `/workspaces/{task_id}/` + Docker Compose |
| **PRD Pillar 3** | Auto-Symlink Dependency Resolver | Phase 2 (EPIC-2.1) | Dynamic `node_modules` / `composer` local symlinker |
| **PRD Pillar 3** | Cross-Repository Atomic Execution | Phase 2 (EPIC-2.1) | Synchronized multi-repo git branches & PRs |
| **PRD Pillar 4 / Sec 4** | Shift-Left ATDD Grounded in Codebase & PRD | Phase 2 (EPIC-2.2) | Playwright/Cypress/Dusk test generators referencing AST |
| **PRD Pillar 4 / Sec 4** | Tech Doc / RFC Creation & Approval Gate | Phase 2 (EPIC-2.2), Phase 3 (EPIC-3.4) | `TECH_DOC_RFC.md` & `AWAITING_TECH_DOC_APPROVAL` gate |
| **PRD Pillar 4 / Sec 4** | Atomic Task Breakdown Planner | Phase 2 (EPIC-2.2) | `TASK_PLAN.md` sequenced WBS generator |
| **PRD Pillar 4** | Strict Write-Locking Protocol | Phase 2 (EPIC-2.2) | Red-Phase kernel/permission lock on source code |
| **PRD Pillar 4** | Autonomous `UAT_PREPARATION.md` | Phase 2 (EPIC-2.2) | Human-readable manual generator |
| **PRD Pillar 4** | Rich Media Evidence (.mp4 Video & Screenshots) | Phase 2 (EPIC-2.3) | Headful/headless Playwright video capture |
| **PRD Pillar 4** | Signed `EVIDENCE.md` Manifest | Phase 2 (EPIC-2.3) | SHA-256 cryptographically signed audit log |
| **PRD Pillar 5** | Go Daemon & Concurrency Event Loop | Phase 1 (EPIC-1.1) | Asynchronous worker pool, Redis BullMQ queue |
| **PRD Pillar 5** | Anti-Loop Frustration Threshold | Phase 1 (EPIC-1.1) | Auto-halt on >=3 failures (`BLOCKED_FRUSTRATION`) |
| **PRD Pillar 5** | State Persistence & <5s RTO | Phase 1 (EPIC-1.1) | Redis state machine checkpointing & recovery |
| **PRD Pillar 6 / UI Spec 1** | Global App Shell (8pt Grid) | Phase 3 (EPIC-3.1) | Navigation Rail, Status Header, Viewport Canvas |
| **PRD Pillar 6 / UI Spec 1** | Mission Control Kanban Dashboard | Phase 3 (EPIC-3.2) | Dynamic Column Projection board with filters & search |
| **PRD Pillar 6 / UI Spec 1** | Active Task Split-Pane View | Phase 3 (EPIC-3.3) | 60/40 Split layout, resizable divider |
| **PRD Pillar 6 / UI Spec 2** | `xterm.js` Live Terminal Console | Phase 3 (EPIC-3.3) | Raw stdout/stderr stream, RAF debouncing |
| **PRD Pillar 6 / UI Spec 2** | Streaming Thought Feed | Phase 3 (EPIC-3.3) | Virtualized timeline, collapsible tool calls |
| **PRD Pillar 6 / UI Spec 2** | Workspace Graph Visualizer | Phase 3 (EPIC-3.3) | SVG dependency graph with animated IO dashes |
| **PRD Pillar 6 / UI Spec 2** | Evidence Vault Media Player | Phase 3 (EPIC-3.3) | HTML5 `.mp4` video scrubber, screenshot diffing |
| **PRD Pillar 6 / UI Spec 2** | HITL Live Context Injection | Phase 3 (EPIC-3.4) | Multi-line injection bar, prompt interceptor |
| **PRD Pillar 6 / UI Spec 2** | One-Click Workspace Reset | Phase 3 (EPIC-3.4) | Docker volume purge, git checkout revert |
| **PRD Pillar 6 / UI Spec 4** | Edge Case Banners & Skeletons | Phase 3 (EPIC-3.4) | Frustration banner, WS toast, loading skeletons |
| **PRD Pillar 7** | TPF & TTR Telemetry Engine | Phase 4 (EPIC-4.1) | Token-Per-Feature and cycle time accounting |
| **PRD Pillar 7** | Dynamic Router Feedback Loop | Phase 4 (EPIC-4.2) | Historical heuristic weight adjustment engine |
| **PRD Pillar 7** | Shadow Benchmarking Engine | Phase 4 (EPIC-4.3) | Background ticket replay & cost-perf matrix |
| **PRD Pillar 7** | Analytics & Performance Dashboard | Phase 4 (EPIC-4.4) | Executive leaderboard, token burn rate charts |
| **PRD Sec 7 & 10** | Zero-Trust Isolation & Hardening | Phase 5 (EPIC-5.1) | Secret scrubbing, blast-radius sandbox guards |
| **PRD Sec 10 & 11**| System E2E ATDD & Packaging | Phase 5 (EPIC-5.2, 5.3) | Full factory end-to-end suite, Docker setup |

---

## 3. Plan Documents Index

The detailed execution breakdown is organized across five modular plan files:

1. [01_PHASE_1_FOUNDATION_DAEMON_REGISTRIES_AST.md](./01_PHASE_1_FOUNDATION_DAEMON_REGISTRIES_AST.md)
   - Go concurrency core, Redis BullMQ event loop, state serialization (<5s RTO).
   - Profile, Skill, Prompt, Method, Tool, and Lifecycle Hooks registries with JSON-Schema validation.
   - Multi-Source Discovery (Remote Git, Project Local, System Global, Built-in) for Skills, Prompts, and Hooks.
   - Custom SDLC State Machine Driver (`.sdlc/workflow.yaml`) and partial stage hydrator.
   - AST Sharding Engine (Tree-sitter), Multi-Provider Abstraction, Cascading Config, Dual Router.
2. [02_PHASE_2_MULTI_REPO_WORKSPACE_AND_ATDD_ENGINE.md](./02_PHASE_2_MULTI_REPO_WORKSPACE_AND_ATDD_ENGINE.md)
   - Dynamic ephemeral workspace provisioner & Docker Compose orchestrator.
   - Multi-repo auto-symlink resolution engine & cross-repo git synchronization.
   - Mid-process workspace hydrator for arbitrary stage slicing (e.g. Implementation through E2E).
   - Lifecycle Hook integration (`pre-stage`, `post-stage`, `pre-commit`).
   - Universal Polyglot Skill runtime mounts (BMAD, Claude, Superpower, MCP) and write-locking guards.
   - ATDD test generation, OS-level write-locking, Playwright `.mp4` video capture, and signed `EVIDENCE.md`.
3. [03_PHASE_3_FRONTEND_MISSION_CONTROL_AND_HITL.md](./03_PHASE_3_FRONTEND_MISSION_CONTROL_AND_HITL.md)
   - Vue 3 + Tailwind v3.4+ dark-mode architecture on strict 8pt grid.
   - Dynamic Column Projection Kanban board adapting to General AI SDLC or Custom SDLC.
   - `NewTaskModal` with Workflow Template Selector & Mid-Process Stage Range Selector.
   - 60/40 Split Execution Workspace: `xterm.js` terminal, Thought Feed, SVG Topology Graph, Evidence Vault.
   - Tool Hub Catalog, Step-by-Step Install Wizard, Version Rollback & Best-Fit Matrix.
   - AI Provider Management, Cascading Project Settings Drawer, and Router Strategy Configurator.
   - Custom SDLC Workflow Builder (`/settings/workflows`) & Multi-Source Registries Manager (`/settings/registries`).
4. [04_PHASE_4_TELEMETRY_SHADOW_BENCHMARKING_AND_ROUTING.md](./04_PHASE_4_TELEMETRY_SHADOW_BENCHMARKING_AND_ROUTING.md)
   - Token-Per-Feature (TPF) and Time-to-Resolution (TTR) telemetry collection.
   - Dynamic routing heuristics and historical feedback weighting.
   - Multi-dimensional Shadow Benchmarking across SDLC Stages $\times$ Task Complexity $\times$ Methods $\times$ Models.
   - Autonomous compilation and hot-reloading of `configs/best_methods_matrix.json`.
   - Performance & Analytics Dashboard with Method Benchmark Matrix.
5. [05_PHASE_5_SECURITY_DEPLOYMENT_E2E_VERIFICATION.md](./05_PHASE_5_SECURITY_DEPLOYMENT_E2E_VERIFICATION.md)
   - Zero-trust security guards, vault-mediated secret injection, log sanitization.
   - Chaos engineering & automated crash-recovery verification.
   - Multi-container Docker deployment & `meta-orch` CLI (workflows, skills, hooks, tools, config).

---

## 4. Definition of Done (DoD) & Acceptance Quality Gates

Each task and phase must satisfy the following zero-compromise criteria before advancing:

1. **Shift-Left ATDD Compliance:** Unit and integration tests must precede implementation and achieve >85% code coverage.
2. **Schema Validity:** All registry files, state transitions, and artifacts must validate against strict JSON-Schema and YAML specifications.
3. **Observability Standards:** All background daemon operations must emit structured events over WebSocket and be visible within the Vue 3 UI without console errors.
4. **Performance Budgets:** 
   - WebSocket streaming latency: `< 100ms`.
   - UI frame rate: `60 fps` under heavy streaming via RAF debouncing.
   - Daemon crash recovery: `< 5s RTO`.
5. **Audit Trail Completeness:** All executed operations must produce cryptographically verifiable audit logs (`EVIDENCE.md`).
