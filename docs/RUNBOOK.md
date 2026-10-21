# Operator Runbook: Meta-Orchestrator Platform

This runbook provides end-to-end instructions for launching, operating, steering, and troubleshooting the **AI-Driven SDLC Meta-Orchestrator** platform.

---

## 1. Prerequisites & Environment Setup

### Required Runtimes
* **Go:** Version `1.22+` (Download: https://go.dev/dl/)
* **Node.js:** Version `18.x` or `20.x` LTS with `npm` (Download: https://nodejs.org/)
* **Git:** Version `2.40+`

### Recommended Tools
* **Docker & Docker Compose:** Version `24+` (Used for container isolation and Playwright `.mp4` video recording).
* **Redis Server:** (Optional for in-memory single-node development; required for multi-node BullMQ distributed queue persistence).

---

## 2. Quickstart: Running the Platform

You can run the platform using `make` commands or by starting the Go backend and Vue 3 frontend in separate terminals.

### First Initialization on a New Device
For a brand new device or fresh clone, you can run:
```bash
make setup
```
This automatically verifies and installs missing system tools (Node.js, Git, Claude Code CLI), downloads Go modules, and installs frontend npm packages.

### Method A: Single Command via `make dev` (Recommended)
From the repository root:
```bash
make dev
```
*(On first run, `make dev` automatically verifies your environment, downloads Go modules, and installs missing frontend dependencies via `npm install` before launching).*

This starts:
* **Go Daemon:** Listening on `http://127.0.0.1:5181` (REST API & WebSockets)
* **Vite Dev Server:** Listening on `http://127.0.0.1:5180` (Frontend Mission Control)

### Method B: Separate Terminals

#### Terminal 1 — Start the Go Daemon
```bash
# From repository root
go run cmd/daemon/main.go
```
*Expected Console Output:*
```
================================================================
  Meta-Orchestrator Daemon Core (Phase 1 Foundation)
================================================================
[Config] Active SDLC: general_ai_sdlc | Router Strategy: BEST_PRACTICE
[Tools] Registered 6 installable packages
[Recovery] Successfully recovered 0 active tasks in < 5s RTO
[HTTP] WebSocket & Health API server listening on :5181
```

#### Terminal 2 — Start the Frontend
```bash
cd web
npm install        # (Only needed on initial setup)
npm run dev
```
*Expected Console Output:*
```
  VITE v6.4.3  ready in 180 ms

  ➜  Local:   http://127.0.0.1:5180/
  ➜  Network: use --host to expose
```

Open your browser to: **`http://127.0.0.1:5180`**

---

## 3. Core Operator Workflows

### 3.1 Launching Autonomous Tasks

1. Navigate to **Mission Control** (`/`).
2. Click the **`+ New Task`** button in the top toolbar.
3. Fill in the task parameters:
   * **Task Title:** Feature objective (e.g. `Implement Stripe billing idempotency`).
   * **Specification:** Acceptance criteria, API contracts, or ticket body.
   * **Target Repositories:** Check applicable repositories (e.g. `frontend-portal`, `backend-core`).
4. **Choose Execution Scope:**
   * **Full SDLC Pipeline:** Executes all 8 stages from initial PRD discovery through final sign-off.
   * **Partial Stage Slice (Mid-Process):**
     * Select **Start Stage** (e.g. `Stage 5: Task Implementation`) to bypass upstream PRD/ATDD phases.
     * Select **Halt Stage** (e.g. `Stage 6: Automation & E2E Validation`) to pause after testing.
     * *(Optional)* Provide an existing Git branch name (e.g. `feat/order-checkout-v2`) or drop an external `TASK_PLAN.md` file into the ingestion dropzone.
     * *(Optional)* Check/uncheck **Record Playwright .mp4 test video**.
5. Select **Router Strategy** (`Best Practice Heuristics` or `Custom Router Rules`) and **Method** (`BMAD`, `Supervisor`, `ReAct`, `Superpower`, or `Auto`).
6. Click **`Launch Task`**. The task immediately appears on the board and starts executing.

---

### 3.2 Human-In-The-Loop (HITL) Observability & Steering

Click on any task card on the Kanban board to enter the **Active Task Detail View** (`/tasks/{id}`).

#### A. Viewing Live Execution
* **Split-Pane Layout:** Drag the center divider to adjust between the 60% Execution View and 40% Control & Artifact Pane.
* **Canvas Terminal (`xterm.js`):**
  * Displays raw, colorized stdout/stderr logs from the ephemeral container.
  * Scrolling up automatically pauses autoscroll; click **`Resume Scroll`** to lock back to the bottom.
  * Use the **`Download Log`** icon button to export the session transcript.
* **Thought Stream:** Inspect real-time agent reasoning vectors, tool calls (`read_ast_node`, `generate_playwright_specs`), parameters, and duration metrics.
* **Workspace Topology Graph:** View dynamically discovered inter-repo symlinks and Docker network bridges with animated SVG dataflow indicators.
* **Evidence Vault:**
  * Watch the captured Playwright `.mp4` test video with 1.0x/1.5x/2.0x playback speed scrubbers.
  * Compare baseline and post-test UI screenshots.
  * Read parsed SDLC artifacts (`PRD.md`, `ATDD_SUITE.md`, `TECH_DOC_RFC.md`, `UAT_PREPARATION.md`, `EVIDENCE.md`).

#### B. Intervening in Execution
* **Prompt Steering Dock (`ContextInput`):**
  * Type guidance into the bottom dock and press **Cmd+Enter** (or Ctrl+Enter) to inject steering instructions directly into the LLM context.
* **Handling Agent Frustration (`BLOCKED_FRUSTRATION`):**
  * When the circuit breaker trips (e.g. $\ge 3$ consecutive test assertion failures), execution freezes and a red sticky banner appears.
  * Click **`Inject Guidance & Resume`** to provide clarifying instructions and immediately resume the agent.
* **One-Click Workspace Reset:**
  * Click the **`Reset`** icon button or **`Purge Volumes & Reset Workspace`** in the frustration alert.
  * Confirms purging Docker volume mounts, executing `git clean -fd && git reset --hard`, and cleanly restarting the active stage.
* **Gate Approvals (`WAITING_GATE_APPROVAL`):**
  * When a review gate is encountered (e.g. Stage 3 Tech Doc RFC or Stage 8 Final Sign-off), the **Gate Approval Bar** displays.
  * Click **`Authorize & Advance`** to proceed, or **`Reject / Revise`** with feedback.

---

### 3.3 Tool Hub & Dependency Management

* **First run:** `make setup` installs every required package that is missing (Node.js, Git, Claude Code CLI), dependencies first.
* **Install / update / pick a version / roll back / uninstall** any package from the Tool Hub or with `go run ./cmd/daemon packages <command> [id] [version]`. Managed CLIs (Claude Code, Playwright, MCP servers) switch versions with an atomic symlink swap in `$MO_HOME/bin`; BMAD and GSD roll back by restoring a snapshot taken before every change; Superpowers rollback is best effort and needs a Claude Code restart; system packages (Node, Git, ffmpeg, Docker) cannot be rolled back.
* See README §10 and `docs/API.md` §2.3 for details.

---

### 3.4 Custom SDLC Workflow Builder (`/settings/workflows`)

1. Browse active pipelines (`General AI SDLC`, `Hotfix Fast-Track`, `Microservice API`, `Production Launch Readiness`, `New Service / Initiative to Production`).
2. Use the **Sequential Stage Pipeline Designer** to add, reorder, or remove stages.
3. Configure stage properties:
   * Persona Role (Architect, Developer, QA Engineer, Product Manager).
   * Execution Method (`BMAD`, `Supervisor`, `ReAct`, `Superpower`).
   * **Strict Write-Locking Protocol:** Toggle source tree write-locks during the stage.
   * **Human Approval Gate:** Toggle whether execution pauses for manual approval.
4. Inspect the **Live Kanban Projection Preview** to verify the projected board columns.
5. Click **`Export to .sdlc/workflow.yaml`** to save to disk.

---

### 3.4a Launch Readiness for a New Service or Initiative

1. Install the readiness tools: Tool Hub → **Launch Readiness** → **Install all missing** (or `go run ./cmd/daemon packages setup launch-readiness`). Required: gitleaks, trivy, semgrep (security), spectral (customer API), k6 (load).
2. **New Task** → workflow **Production Launch Readiness** (existing service) or **New Service / Initiative to Production** (from the PRD). Assign the service's repositories.
3. In the routing step, check each readiness row's **Tools** (✓ installed / ✗ missing) and its built-in playbook skill.
4. Run each stage. When it finishes, confirm the **Confirm before approving** checklist in the console, then approve or reject with feedback.
5. Full guide: [`docs/LAUNCH_READINESS.md`](LAUNCH_READINESS.md).

---

### 3.5 Multi-Source Registries Hub (`/settings/registries`)

* **Skills Hub:**
  * Connect external Git repositories containing Claude Code `SKILL.md`, BMAD packs, or Anthropic MCP servers via the **`Add Remote Git Skill Repo`** modal.
* **Prompt Templates:**
  * Inspect system default prompts vs project-local overrides (`.sdlc/prompts/`) and view slot variable schemas.
* **Lifecycle Hooks Engine:**
  * Author pre-stage, post-stage, on-gate, pre-commit, or on-failure hooks.
  * Select execution engine (`Shell Script`, `Docker Container`, `Webhook POST`) and failure policy (`BLOCK` vs `WARN`).

---

## 4. Troubleshooting & FAQ

### Q1: Port 5181 is already in use
* Check what process is using port 5181: `lsof -i :5181`.
* Either stop the conflicting process or adjust the daemon port in `cmd/daemon/main.go` and `web/vite.config.ts`.

### Q2: Frontend shows "Daemon Offline" or "Reconnecting..."
* Ensure the Go daemon is running (`go run cmd/daemon/main.go`).
* Verify health check responds: `curl http://127.0.0.1:5181/healthz`. Should return `{"status":"healthy","version":"1.0.0"}`.
* In browser DevTools, verify the WebSocket connection to `ws://127.0.0.1:5181/ws` or `ws://127.0.0.1:5180/ws` is open.

### Q3: Terminal Canvas does not resize properly
* Click the "Resume Scroll" button or resize your browser window; `@xterm/addon-fit` automatically recalculates column and row geometries on window resize events.

### Q4: How do I run the full test suite?
```bash
make test
```
Runs both the Go backend race-detector unit test suites and frontend Vue TypeScript strict typechecking.

### Q5: Agents' Docker commands use too much CPU
Heavy Docker commands that agents run (`build`, `run`, `exec`, `compose up`, `pull`, …) share a machine-wide limit; extra ones wait for a free slot, and the console says what they are waiting on. Read-only commands (`ps`, `images`, `inspect`, `compose ps/config`) never wait. Containers agents start with `docker run`/`create` get a `--cpus` limit unless the command sets its own. Set before starting the daemon:

| Variable | Default | Meaning |
|---|---|---|
| `MO_DOCKER_MAX_CONCURRENT` | `2` | Heavy Docker commands running at once across all tasks |
| `MO_DOCKER_CPUS` | `2` | `--cpus` added to containers agents start; `0` adds none |

Only commands that go through the console's approval flow are limited (this includes "allow all"). Docker called indirectly, e.g. from a `make` target or script, is not detected.
