# AI-Driven SDLC Meta-Orchestrator

[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![Vue 3](https://img.shields.io/badge/Vue-3.5+-4FC08D?style=flat&logo=vue.js)](https://vuejs.org/)
[![Tailwind CSS](https://img.shields.io/badge/Tailwind-3.4+-38B2AC?style=flat&logo=tailwind-css)](https://tailwindcss.com/)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)
[![Status](https://img.shields.io/badge/Status-Phases_1--3_100%25_Verified-emerald)](#roadmap--milestones)

The **AI-Driven SDLC Meta-Orchestrator** is an autonomous, event-driven software development factory designed to automate complex, multi-repository feature lifecycles under a zero-trust software factory paradigm.

It unifies multi-agent methodologies (**BMAD**, **Supervisor**, **ReAct**, **Superpower**), shifts left through rigorous Acceptance Test-Driven Development (**ATDD**) with OS-level write-locking, captures cryptographic rich-media evidence (including automated Playwright `.mp4` recordings), and provides an interactive Vue 3 Mission Control dashboard with real-time terminal emulation and Human-in-the-Loop (**HITL**) steering.

---

## ⚡ Quickstart: Running in 3 Steps

### Step 1: Prerequisites
Ensure you have **Go 1.22+**, **Node.js 18+ / 20+**, and **git** installed.

### Step 2: Launch the Full Platform
You can run both the Go Daemon and the Vue 3 Frontend with a single command:
```bash
make dev
```
*(Alternatively, run `go run cmd/daemon/main.go` in one terminal and `cd web && npm run dev` in another).*

### Step 3: Open Mission Control
Open your web browser and navigate to:
```
http://localhost:5173
```
* **Frontend UI:** `http://localhost:5173`
* **Daemon REST API:** `http://localhost:8080/api/v1`
* **WebSocket Streaming Hub:** `ws://localhost:8080/ws`
* **Health Check:** `http://localhost:8080/healthz`

---

## 🏗️ High-Level System Architecture

```
┌───────────────────────────────────────────────────────────────────────────────────┐
│                       VUE 3 FRONTEND MISSION CONTROL (:5173)                      │
├────────────────────┬────────────────────┬───────────────────┬─────────────────────┤
│  Kanban Board      │  Execution Split   │ Tool Hub Catalog  │ Settings & Pipeline │
│  Dynamic Columns   │  @xterm/xterm Canvas│ 4-Step Installer  │ Custom Workflows    │
│  Mid-Process Slice │  Thought Stream    │ 1-Click Rollback  │ Multi-Source Skills │
│  Token Burn Pill   │  Evidence Vault    │ Best-Fit Matrix   │ Benchmark Table     │
└─────────▲──────────┴─────────▲──────────┴─────────▲─────────┴──────────▲──────────┘
          │ (HTTP REST /api/v1)│                    │                    │
          │ (WebSocket ws://)  │                    │                    │
┌─────────▼────────────────────▼────────────────────▼────────────────────▼──────────┐
│                         GO DAEMON CORE ENGINE (:8080)                             │
├───────────────────────────────────────────────────────────────────────────────────┤
│ • Concurrency Loop: 10 parallel workers, BullMQ compatible, <5s RTO crash recovery│
│ • Custom SDLC FSM: 9-stage General AI SDLC or arbitrary .sdlc/workflow.yaml       │
│ • Mid-Process Slice Hydrator: Execute arbitrary stage slices (e.g. Stage 5 to 6)  │
│ • Anti-Loop Circuit Breaker: Tripped on >= 3 consecutive failures to BLOCKED_FRUST│
│ • Universal Skill Adapters: Claude SKILL.md, BMAD, Superpower, MCP stdio/SSE     │
│ • Multi-Source Discovery: Remote Git Repos, Project Local, User System, Built-in  │
│ • Tool Lifecycle Engine: 4-stage installer, <1s atomic symlink rollback           │
│ • Dynamic Task Profiler: Traverses package.json, go.mod, composer.json            │
│ • Dual-Strategy Router: Best Practice Heuristics vs. Custom Rule-Based Router    │
│ • AST Code Sharder: Tree-sitter parsers (TS, Go, PHP, Py) with >90% token saving  │
│ • Shift-Left ATDD Engine: Codebase-grounded Playwright spec generator             │
│ • Strict Write-Lock Guard: Read-only locks on source code until Red Phase passes  │
│ • Rich Media Evidence: Playwright .mp4 video recorder, screenshots & HMAC-SHA256  │
└───────────────────────────────────────────────────────────────────────────────────┘
```

---

## 🧭 Key Capabilities & Operator Features

### 1. Dynamic Column Projection Kanban Board
* Visualizes active tickets across lifecycle phases: `PRD Discovery`, `ATDD Red Phase`, `Tech Doc Review (Gate)`, `Task Breakdown`, `Implementation`, `Automation & E2E`, `Manual UAT`, and `Sign-Off & Merge`.
* **Dynamic Projection:** Automatically shifts columns when custom workflows are selected (e.g., 3-stage Hotfix Fast-Track).

### 2. Mid-Process Execution & Stage Slicing
* Launch features starting at **any stage** in the SDLC (e.g., start at *Implementation* and halt at *E2E Validation*).
* Ingests external git feature branches and `TASK_PLAN.md` / `PRD.md` files directly via the UI dropzone.

### 3. Active Task Split-Pane Workspace (60/40)
* **Canvas Terminal (`xterm.js`):** High-density canvas rendering live stdout/stderr streams from ephemeral containers with `requestAnimationFrame` debouncing, autoscroll pause, and transcript downloads.
* **Virtualized Thought Stream:** Real-time stream of agent reasoning, tools invoked (`read_ast_node`, `generate_playwright_specs`), parameters, and execution timings.
* **Workspace Topology Visualizer:** SVG dependency graph displaying dynamically linked multi-repo mounts with animated dataflow dashes.
* **Evidence Media Vault:** Built-in HTML5 player for Playwright `.mp4` recordings (with 1x/1.5x/2x speed controls), side-by-side screenshot diffs, and tabbed Markdown artifact viewer.

### 4. Human-In-The-Loop (HITL) Controls
* **Anti-Loop Frustration Circuit Breaker:** Execution halts upon $\ge 3$ consecutive errors into `BLOCKED_FRUSTRATION`, displaying failing stack traces and halting token burn.
* **Live Prompt Steering Dock (`ContextInput`):** Inject corrective guidance directly into the agent's context (Cmd+Enter) to steer or unblock the agent.
* **One-Click Workspace Reset:** Safely purges Docker volumes, runs `git clean -fd && git reset --hard`, and restarts the stage cleanly.
* **Human Approval Gates:** Dedicated gate approval bars to authorize or request revisions for architecture RFCs and releases.

### 5. Multi-Source Registries & Skill Hub
* Connects Anthropic Model Context Protocol (**MCP** stdio/SSE) servers, Claude Code `SKILL.md` git repos, BMAD multi-agent skill packs, and Superpower tools.
* 4-tier cascading discovery: Project Local (`.sdlc/skills/`) $\succ$ User System (`~/.config/meta-orchestrator/skills/`) $\succ$ Remote Git $\succ$ Built-in.

### 6. UI Project Setup & Multi-Repo Symlink Engine (`/projects`)
* **Project Registration Wizard:** Register multi-repo ecosystems via UI and classify components into roles: `Frontend UI`, `Backend Service`, `Automation Test`, `API Contracts`, and `Artifacts / Docs`.
* **Local Directory Auto-Scanner:** Point to any directory; automatically detects manifests (`package.json`, `go.mod`, `playwright.config.ts`, `openapi.yaml`) and suggests functional roles.
* **Unified Project Root & Atomic Symlinks:** Provisions dedicated workspace roots (e.g. `workspaces/{project-id}/`) with atomic filesystem symlinks pointing to target repositories and writes `.sdlc/project.json`.
* **Dynamic Kanban Ingestion:** Selecting a registered project dynamically populates the New Task modal with mapped repositories and role tags.

### 7. Tech-Stack Awareness (`internal/stack`)
Every repository is scanned from its files only (nothing is executed; dependency and cache folders are skipped). The detected stack drives role suggestions, UI tags, complexity grading, UAT app kinds and a **stack brief** added to every agent turn. For each repository, the brief lists its verification commands, the conventions to keep and the commands the agent must never run.

| Area | Detected | Agent verifies with | Guardrails |
|---|---|---|---|
| Backend | Go, PostgreSQL | `go build/vet/test ./...`, `golangci-lint run` | new migrations only; no writes to shared DBs |
| Backend (JVM) | Java, Kotlin (Spring, Maven, Gradle) | `./gradlew check` or `./mvnw -q verify` once per build root (wrapper preferred) | no `mvn deploy` / `gradle publish` |
| Frontend | React, TypeScript | the package's own `typecheck`/`lint`/`test`/`build` scripts (npm, pnpm, yarn or bun), else `tsc --noEmit` | — |
| Mobile | Flutter, Dart, Android | `dart format --set-exit-if-changed`, `flutter`/`dart analyze` and `test`, `build_runner` when used; Android through Gradle `check` | no `pub publish`, store uploads or release signing |
| Identity | Keycloak (OIDC), SpiceDB | `zed validate <schema>.zed` | no admin/schema writes to live realms or SpiceDB |
| Cloud | Azure Container Apps, Cloudflare | `wrangler deploy --dry-run` | no mutating `az containerapp` / `wrangler deploy` |
| IaC | OpenTofu, Terragrunt | `tofu fmt -check`, `tofu validate`, `terragrunt hclfmt --check` | no `apply`/`destroy`/`state`; `plan` only when the operator approves |
| Delivery | GitHub Actions, containers, Playwright | `actionlint`, the e2e script or `playwright test` | no `gh workflow run`, deploy tags or `docker push` |
| Observability | OpenTelemetry, Grafana | — | no personal data or unbounded values in telemetry |
| API | OpenAPI | `npx @redocly/cli lint <spec>` | contract changes ship with handlers and regenerated clients |

Repositories that hold only IaC get the new **Infrastructure** role and have no manual-UAT surface. A Go, Java, Kotlin or Dart service with no screens becomes an **API** app in the UAT guide, with Keycloak token steps when it uses OIDC. A Flutter or Android app becomes a **mobile** app: testers install the UAT build on a device and follow manual steps, and nothing is replayed in a browser (ATDD platform `MOBILE`, `ANDROID` or `IOS`). The generated `android/`/`ios/` folders of a Flutter app count as part of that app, not as separate Java or Kotlin projects. **Adding a technology** means adding a rule to `rules` (`internal/stack/detect.go`) and, if agents need guidance, an entry in `guides` (`internal/stack/brief.go`).

### 8. Toolchain: Install What the Stack Needs (`internal/toolchain`)
Every detected stack declares the command-line tools its checks need: required tools (e.g. Go, a JDK, Flutter, OpenTofu) and recommended ones (e.g. actionlint, zed). The orchestrator checks this machine by **running** each tool, not just finding it on `PATH`, so the macOS `java` stub without a JDK counts as missing.

* **Where you see it:** *Tool Hub → Stack toolchain* covers every registered repository. *Task → Workspace & Repos → Toolchain on this machine* covers one task. Each tool shows its status, version, why it is needed and how to install it.
* **One-click installs:** when Homebrew, npm or Go can install a tool without sudo, a password or a GUI, **Install** runs the exact command shown, streams the log, verifies the tool and re-checks. Installs of the same tool never overlap, and only one install runs at a time per package manager.
* **Copy-and-run:** methods that need sudo, a password or a GUI (`apt`, `snap`, the Temurin JDK and Android Studio casks) are never run for you. You get the command to copy, plus the official install guide and **Re-check**.
* **Before a stage runs:** a code stage (ATDD, implementation, E2E) whose required tools are missing does not start. A prompt lists them, with **Install all**, **Re-check**, **Run *stage*** once they are ready, or **Run without them**. "Run without them" is remembered per task for exactly those tools; the agent skips the affected checks and says so. A console message sent to a blocked stage is answered as chat instead of being lost.
* **Agents** are told which tools are missing and never install tools themselves.
* **PATH:** at startup and after each install, the daemon adds existing package-manager bin folders (`/opt/homebrew/bin`, `~/go/bin`, `/snap/bin`, …) to its `PATH`, so tools installed while it runs are visible to it and to its agents.

API: `GET /api/v1/toolchain[?task=|?project=][&all=1]`, `POST /api/v1/toolchain/recheck`, `POST /api/v1/toolchain/install {"tool","method"}`, `GET /api/v1/toolchain/jobs/{id}`, `GET /api/v1/tasks/{id}/toolchain`, `POST /api/v1/tasks/{id}/toolchain/ack`. A blocked stage start answers `409 {"status":"needs_tools","missing":[…],"toolchain":{…}}` and broadcasts the `toolchain.needed` WebSocket event. **Adding a tool** means adding one `Catalog` entry (`internal/toolchain/catalog.go`) and asking for it from `Profile.Needs` (`internal/stack/needs.go`).


---

## 🛠️ Make Commands Reference

| Command | Description |
|---|---|
| `make dev` | **Runs both backend Go daemon (:8080) and frontend Vite server (:5173)** |
| `make run-daemon` | Starts the Go daemon server in the current terminal |
| `make run-web` | Starts the frontend Vite dev server in the current terminal |
| `make build` | Builds `bin/daemon` binary and compiles production frontend to `web/dist/` |
| `make test` | Runs all backend Go test suites with race detector and checks Vue types |
| `make clean` | Removes compiled binaries (`bin/`) and frontend build artifacts (`web/dist/`) |
| `make help` | Displays available make targets |

---

## 📚 Complete Documentation

* **[Operator Runbook (`docs/RUNBOOK.md`)](file:///Users/rezamekari/Projects/go/src/github.com/ahmadrezamusthafa/meta-orchestrator/docs/RUNBOOK.md):** Complete step-by-step operating procedures, troubleshooting guide, and edge-case handling.
* **[REST & WebSocket API Reference (`docs/API.md`)](file:///Users/rezamekari/Projects/go/src/github.com/ahmadrezamusthafa/meta-orchestrator/docs/API.md):** Full documentation for all HTTP REST endpoints, JSON payload schemas, and WebSocket message types.
* **[Product Requirements Document (`PRD001.md`)](file:///Users/rezamekari/Projects/go/src/github.com/ahmadrezamusthafa/meta-orchestrator/.sdlc/prds/PRD001.md):** Architectural pillars, security blast-radius constraints, and system specifications.
* **[Master Execution Plan (`00_MASTER_EXECUTION_PLAN.md`)](file:///Users/rezamekari/Projects/go/src/github.com/ahmadrezamusthafa/meta-orchestrator/.sdlc/tasks/plan/00_MASTER_EXECUTION_PLAN.md):** 5-phase execution plan and status matrix.

---

## 🚦 Roadmap & Milestones

* **Phase 1: Foundation Daemon, Registries & AST Core** — **`[COMPLETED & 100% VERIFIED]`** (18/18 Tasks)
* **Phase 2: Multi-Repo Workspace & ATDD Engine** — **`[COMPLETED & 100% VERIFIED]`** (13/13 Tasks)
* **Phase 3: Frontend Mission Control & HITL Observability** — **`[COMPLETED & 100% VERIFIED]`** (21/21 Tasks)
* **Phase 4: Telemetry, Shadow Benchmarking & Routing** — *`[Next Up]`* (12 Tasks)
* **Phase 5: Security, Deployment & E2E Factory Verification** — *`[Planned]`* (12 Tasks)
