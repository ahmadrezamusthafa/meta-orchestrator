# AI-Driven SDLC Meta-Orchestrator

[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![Vue 3](https://img.shields.io/badge/Vue-3.5+-4FC08D?style=flat&logo=vue.js)](https://vuejs.org/)
[![Tailwind CSS](https://img.shields.io/badge/Tailwind-3.4+-38B2AC?style=flat&logo=tailwind-css)](https://tailwindcss.com/)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)

The **AI-Driven SDLC Meta-Orchestrator** is an autonomous, event-driven software development factory designed to automate complex, multi-repository feature lifecycles under a zero-trust software factory paradigm.

It runs Claude Code agents stage by stage through a configurable SDLC workflow, shifts left with Acceptance Test-Driven Development (**ATDD**) under a red-phase write lock, records signed test evidence (Playwright videos, screenshots and an HMAC-SHA256 manifest), and provides a Vue 3 Mission Control dashboard with a live agent console and Human-in-the-Loop (**HITL**) approvals.

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
* **Daemon REST API:** `http://127.0.0.1:8080/api/v1`
* **WebSocket Streaming Hub:** `ws://127.0.0.1:8080/ws`
* **Health Check:** `http://127.0.0.1:8080/healthz`

---

## 🏗️ High-Level System Architecture

```
┌───────────────────────────────────────────────────────────────────────────────────┐
│                       VUE 3 FRONTEND MISSION CONTROL (:5173)                      │
├────────────────────┬────────────────────┬───────────────────┬─────────────────────┤
│  Kanban Board      │  Execution Split   │ Tool Hub Catalog  │ Settings & Pipeline │
│  Dynamic Columns   │  Agent Console     │ Health Checks     │ Custom Workflows    │
│  Mid-Process Slice │  Approvals         │ Toolchain Install │ Multi-Source Skills │
│  Token Burn Pill   │  Evidence Tab      │ Host Compat Check │ Benchmark Table     │
└─────────▲──────────┴─────────▲──────────┴─────────▲─────────┴──────────▲──────────┘
          │ (HTTP REST /api/v1)│                    │                    │
          │ (WebSocket ws://)  │                    │                    │
┌─────────▼────────────────────▼────────────────────▼────────────────────▼──────────┐
│                         GO DAEMON CORE ENGINE (:8080)                             │
├───────────────────────────────────────────────────────────────────────────────────┤
│ • Stage Pool: bounded concurrent stage runs (MO_MAX_CONCURRENT_STAGES, default 10)│
│ • Workflows: 8-stage default SDLC, built-in variants, custom workflows (API/YAML) │
│ • Mid-Process Slicing: start at any stage, uploaded PRD/RFC/plan fill skipped ones│
│ • Frustration Breaker: 3 identical stage failures → BLOCKED_FRUSTRATION           │
│ • ATDD Red Phase: test-only write lock, red-phase verification, Playwright scaffold│
│ • Lifecycle Hooks: shell / webhook / Docker on stage, gate and commit events      │
│ • Skills & MCP: Claude SKILL.md, BMAD, Superpower, MCP connectors (read-only)     │
│ • Stack Awareness & Toolchain: detected stacks, verified installs, agent brief    │
│ • Routing: complexity analysis, best-practice or custom rules, fallback chains    │
│ • Evidence: per-case videos (MP4 via ffmpeg), screenshots, signed manifest        │
│ • Local-only API: loopback bind, Host/Origin checks, no wildcard CORS             │
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

### 2a. Launch Readiness for New Services & Initiatives (`internal/readiness`)
* Two built-in workflows take a service to production: **Production Launch Readiness** (an existing service) and **New Service / Initiative to Production** (the full SDLC followed by readiness).
* Six readiness stages, each confirmed by the operator at a gate before the next one starts:

  | Stage | Step | Changes code |
  |---|---|---|
  | `release_audit` | Check how we release today | no (read-only audit) |
  | `release_automation` | Make releases automatic (CI/CD as code, gated promotion, rollback) | yes |
  | `observability_alerting` | Watch the system and send alerts (logs, metrics, traces, SLOs, alerts with runbooks) | yes |
  | `security_automation` | Check security automatically (secret/SCA/SAST/image/IaC scans, triage policy) | yes |
  | `customer_api_setup` | Set up the API for customers (OpenAPI, versioning, auth, rate limits, docs) | yes |
  | `load_resilience` | Test with heavy traffic and prepare for problems (load tests, runbooks, rollback drill) | yes |

* Each stage loads a built-in playbook skill (`launch-*`), installed into `.sdlc/skills/` at startup. Edit one there and your version is kept across upgrades; disable it on the Skills page to turn it off. Skills you attach in the Routing tab are added to it.
* Each stage's **gate checklist** is in the agent's brief (its Summary marks every item ✓/✗/n/a), in the console when the stage finishes, and on the task's Stage plan.
* Stages declare the tools they run (gitleaks, trivy, semgrep, spectral, k6, …). Required ones gate the stage like other toolchain needs; install them all from the Tool Hub's **Launch Readiness** group or `go run ./cmd/daemon packages setup launch-readiness`.
* **Operator guide:** [`docs/LAUNCH_READINESS.md`](docs/LAUNCH_READINESS.md).

### 3. Active Task Split-Pane Workspace (60/40)
* **Agent Console:** the live transcript of each turn — the agent's text and thinking, every tool call with its input and result, permission requests to approve or deny, and transcript downloads.
* **File Outline:** `GET /api/v1/tasks/{id}/outline?repo=&path=` lists a file's declarations (Go, TypeScript/JavaScript/Vue, PHP, Python) from the task worktree. It is a regex outline, not a parser.
* **Workspace Topology Visualizer:** SVG dependency graph displaying dynamically linked multi-repo mounts with animated dataflow dashes.
* **Evidence Tab:** plays each test case's recording, shows step screenshots and coverage of the acceptance cases, and **Verify** recomputes every file hash and the manifest signature (`GET /api/v1/tasks/{id}/evidence/verify`). The signing key comes from `MO_EVIDENCE_SIGNING_KEY`, or is generated once at `.sdlc/evidence-signing.key` (mode 0600).

### 4. Human-In-The-Loop (HITL) Controls
* **Anti-Loop Frustration Circuit Breaker:** 3 identical stage failures in a row (fewer if the task's method sets a stricter limit) move the task to `BLOCKED_FRUSTRATION` and show the failure; Continue, a message or a reset resumes it. Within a turn, a shell command that keeps failing the same way is refused.
* **ATDD Red Phase:** during `atdd_creation` the agent may edit only test files; other writes are refused. The stage records whether the acceptance tests failed for the right reason (`red_phase`) and writes a Playwright scaffold (`atdd/acceptance.spec.ts`) whose unimplemented steps fail.
* **Lifecycle Hooks:** `.sdlc/hooks.json` and `.sdlc/hooks/<event>.sh` run on `pre-stage`, `post-stage`, `on-failure`, `on-gate`, `pre-commit` and `post-commit`; blocking hooks can stop the step.
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

API: `GET /api/v1/toolchain[?task=|?project=][&all=1]`, `POST /api/v1/toolchain/recheck`, `POST /api/v1/toolchain/install {"tool","method"}`, `GET /api/v1/toolchain/jobs/{id}`, `GET /api/v1/tasks/{id}/toolchain`, `POST /api/v1/tasks/{id}/toolchain/ack`. A task-scoped report (`?task=`, `/tasks/{id}/toolchain`) also includes the tools of its launch-readiness stages (`internal/readiness`). A blocked stage start answers `409 {"status":"needs_tools","missing":[…],"toolchain":{…}}` and broadcasts the `toolchain.needed` WebSocket event. **Adding a tool** means adding one `Catalog` entry (`internal/toolchain/catalog.go`) and asking for it from `Profile.Needs` (`internal/stack/needs.go`).

#### Repository dependencies (`internal/deps`)
Each registered repository's project dependencies (React, Vue, Go modules, Flutter packages, gems, Python and JVM libraries) are listed per ecosystem with the wanted range, the installed or locked version, and the framework-level ones flagged as key. Installing means reproducing the lockfile with the repository's own package manager (`npm ci`, `pnpm install --frozen-lockfile`, `yarn install --frozen-lockfile`/`--immutable`, `bun install --frozen-lockfile`, `go mod download`, `flutter pub get`, `bundle install`, …). It runs in the registered folder or in a task's worktree, with a 20-minute limit and one install per repository at a time, and the log is streamed. Running it again is how local drift is rolled back. When the package manager is missing, you get the command to copy and the toolchain tool to install. Changing a version, for example React 18 → 19, is a code change: **Upgrade** creates a task that edits the manifest and lockfile in a worktree, fixes breaking changes and runs the repository's checks. The change is reviewed in a pull request, and reverting it is the rollback. Registry lookups for the latest versions (`npm view`) run only when you ask for them. API: `GET /api/v1/projects/{id}/dependencies`, `POST …/dependencies/install`, `POST …/dependencies/upgrade`, `GET /api/v1/dependencies/jobs/{id}` (see [docs/API.md](docs/API.md)).

### 9. Connectors for the Stack (`internal/connectors`)
Agents can read live state from third-party services through MCP servers. Each connector is opt-in, its access is described in the UI and in the agent brief, and it defaults to read-only.

| Connector | MCP server | Agent access | Credential |
|---|---|---|---|
| GitHub | GitHub's official server (`github-mcp-server`, else its Docker image) | Read-only: repos, issues, PRs, Actions runs and logs | Fine-grained PAT (Read) or `GITHUB_TOKEN` |
| Grafana | `mcp-grafana`, else Docker `mcp/grafana` | Dashboards, alerts, Prometheus/Loki/Tempo queries (Viewer role) | Service account token |
| Azure | `@azure/mcp` with `--read-only` | Resources and Container Apps | `az login` |
| Cloudflare | Cloudflare Observability server via `mcp-remote` | Workers logs and analytics | Read-only API token |
| ClickUp | `@hauptsache.net/clickup-mcp` with `CLICKUP_MCP_MODE=read` | Tasks, comments, lists, spaces, docs | Personal API token (`pk_…`) + workspace ID |
| Trello | `@delorenj/mcp-server-trello` | Boards, lists, cards, checklists, comments (read-only via the token's `scope=read`) | API key + token |
| Asana | `@roychri/mcp-server-asana` with `READ_ONLY_MODE=true` | Tasks, projects, sections, comments, status updates | Personal access token |
| monday.com | `@mondaydotcomorg/monday-api-mcp` with `--read-only` | Boards, items, updates, columns | API token |
| Linear | `@tacticlaunch/mcp-linear` | Issues, projects, cycles, comments (read-only when the key has only the Read permission) | Personal API key |
| Notion | `@notionhq/notion-mcp-server` | Pages and databases shared with the integration | Internal integration secret (Read content) |
| Jira, Confluence, Bitbucket, GitLab, Slack | as configured | as configured | as configured |

* **Suggested from the stack:** a GitHub/GitLab/Bitbucket remote, GitHub Actions, OpenTelemetry/Grafana, Azure Container Apps and Cloudflare each suggest their connector. Project-management tools are suggested from the links a repository already has: ClickUp, Trello, Asana, monday.com, Linear and Notion URLs (and ClickUp `CU-…` task IDs) in the README, CONTRIBUTING, PR/MR templates, CI workflows or recent commit and branch names in the reflog. Git is not run, and only reflog messages are read, never authors. Suggestions appear in *Tool Hub* and *Task → Workspace & Repos*, and **Connect** opens the connector's setup with step-by-step instructions.
* **Into every agent turn:** enabled connectors that are connected or configured, and have their credentials, are passed to the Claude CLI with `--mcp-config` in a private per-turn file (mode 0600 in a 0700 folder) that is deleted when the turn ends. The agent brief lists them as `mcp__<id>__*` with their access, plus the suggested connectors that are not connected yet. Tool calls that write still ask the operator.
* **Real tests:** *Test MCP* on a stack connector checks its credential against the service's API (ClickUp also lists the workspaces the token reaches, and Trello warns when the token can write), then starts the MCP server, performs the `initialize` handshake and lists its tools. The periodic ping only checks credentials.
* **Secrets:** API responses and WebSocket events carry masked credentials only (`••••last4`). Saving a masked value keeps the stored secret, and `connectors.json`, `.mcp.json` and `.sdlc/mcp.json` are written with mode 0600.
* **Migration:** Linear and Notion used to name `@modelcontextprotocol/server-linear` and `server-notion`, which were never published. They now start `@tacticlaunch/mcp-linear` and Notion's official server, and the saved token carries over.
* **Launchers:** the native MCP server is used once installed (the Tool Hub installs managed `go install` builds of `github-mcp-server` and `mcp-grafana`, and the stack toolchain offers Homebrew for the Azure CLI); otherwise Docker is used. A command you customise is never replaced.

**Adding a connector** means adding one `stackConnectors` entry (`internal/connectors/stack_connectors.go`) and, for a suggestion, one `connectorRules` entry (`internal/api/connector_suggestions.go`).


### 10. Tool Hub: package manager (`internal/packages`)
The tools the orchestrator itself uses come from `configs/packages.json` (operator-owned): Claude Code CLI, Node.js and Git (required), Playwright and ffmpeg (recommended), Docker, Go, BMAD, GSD, Superpowers and the GitHub/Grafana MCP servers. Each can be installed, updated, pinned to a version, rolled back and uninstalled from the Tool Hub (`/api/v1/packages`) or the CLI. State lives in `$MO_HOME` (default `~/.meta-orchestrator`): `packages/state.json` (kept versions, snapshots, history) plus the managed installs.

| Kind | Packages | Install / update | Rollback | Uninstall |
|---|---|---|---|---|
| `npm-cli` | claude-code, playwright | `npm install --prefix $MO_HOME/packages/<id>/versions/<v>`; `$MO_HOME/bin/<binary>` is a symlink switched atomically | instant and offline to a kept version (the last 3 inactive are kept), or installs the requested one | managed versions and shims only; a system copy is reported, never touched |
| `go-cli` | github-mcp-server, mcp-grafana | `GOBIN=<version dir>/bin go install <path>@v<version>` (the `go` package is installed first) | as npm-cli | as npm-cli |
| `npx-installer` | bmad, gsd | `npx -y <package>@<version> …` with the manifest's install/update arguments (BMAD updates with `--action quick-update`, which keeps every installed module) | restores a tar.gz snapshot of the footprint taken before every change (last 5 kept), offline; BMAD's footprint includes every skill listed in its `skill-manifest.csv` | snapshot, the installer's own uninstaller when declared, then the footprint is removed |
| `claude-plugin` | superpowers | `claude plugin install/update` (latest only) | best effort: restores the snapshotted cache folder and rewrites the plugin's entry in `installed_plugins.json` (`.bak` kept); restart Claude Code | snapshot, then `claude plugin uninstall` |
| `system` | node, go, git, ffmpeg, docker | Homebrew (or the toolchain catalog's unattended method); apt is shown as a command, never run; never sudo. Update/uninstall only when the copy in use is a Homebrew install (an nvm, gvm or Docker Desktop copy is left to its own tool) | not supported (system package managers keep no previous versions; use nvm/fnm for Node) | `brew uninstall` |

`$MO_HOME/bin` is put first on the daemon's `PATH` at startup, so agents run the managed versions. Commands run without a shell (argv only), versions are validated, one job runs per package and per package manager (npm, go, brew, claude) at a time, each job has a 15-minute timeout and its process group is killed on cancel. Status comes from running each binary's version command (cached for a minute); the latest version is cached for an hour and fetched in the background, never while listing. A version that cannot be read is reported as `unknown`.

```bash
make setup                                   # install every missing required package, dependencies first
go run ./cmd/daemon packages list            # status, version, latest
go run ./cmd/daemon packages versions claude-code
go run ./cmd/daemon packages install claude-code 2.1.302
go run ./cmd/daemon packages rollback claude-code          # previous kept version
go run ./cmd/daemon packages update|uninstall <id>
```

### 11. Local-Only Access & Security
The daemon runs agents with shell access and holds provider and connector credentials, so it accepts only local requests:
* It binds `127.0.0.1:8080`. `MO_LISTEN_ADDR` overrides the address.
* Requests whose `Host` is not a loopback name are refused, which stops DNS rebinding.
* Browser requests and WebSocket upgrades whose `Origin` is not loopback are refused. CORS is answered only for allowed origins, never `*`.
* `MO_ALLOWED_HOSTS` (comma-separated) adds hostnames deliberately, e.g. a LAN name.
* Request bodies are capped at 32 MiB.

| Variable | Effect |
|---|---|
| `MO_LISTEN_ADDR` | Listen address (default `127.0.0.1:8080`) |
| `MO_ALLOWED_HOSTS` | Extra hostnames allowed in `Host`/`Origin` |
| `MO_MAX_CONCURRENT_STAGES` | Stage runs at once (default 10; up to 100 more wait in a queue) |
| `MO_EVIDENCE_SIGNING_KEY` | HMAC key for evidence manifests (default: generated `.sdlc/evidence-signing.key`) |
| `MO_DOCKER_MAX_CONCURRENT`, `MO_DOCKER_CPUS` | Limits on Docker work agents run |
| `MO_SHADOW_ENABLED` | Opt-in shadow benchmarking (makes paid LLM calls) |
| `REDIS_ADDR` | Routing weights and a task-snapshot mirror used to restore tasks at startup |

---

## 🛠️ Make Commands Reference

| Command | Description |
|---|---|
| `make setup` | Installs required system tools (Node.js, Git, Claude Code) and all project dependencies |
| `make install-deps` | Installs/updates Go and frontend dependencies (`npm install` in `web/`) |
| `make dev` | **Runs both backend Go daemon (:8080) and frontend Vite server (:5173)** (auto-installs dependencies on first run) |
| `make run-daemon` | Starts the Go daemon server in the current terminal |
| `make run-web` | Starts the frontend Vite dev server in the current terminal (ensures npm dependencies installed) |
| `make build` | Builds `bin/daemon` binary and compiles production frontend to `web/dist/` |
| `make test` | Runs all backend Go test suites with race detector and checks Vue types |
| `make clean` | Removes compiled binaries (`bin/`) and frontend build artifacts (`web/dist/`) |
| `make clean-all` | Removes compiled binaries, build artifacts, and frontend `node_modules` |
| `make help` | Displays available make targets |

---

## 📚 Complete Documentation

* **[Operator Runbook (`docs/RUNBOOK.md`)](docs/RUNBOOK.md):** operating procedures, troubleshooting and edge cases.
* **[REST & WebSocket API Reference (`docs/API.md`)](docs/API.md):** HTTP endpoints, JSON payloads and WebSocket message types.
* **[Launch Readiness Guide (`docs/LAUNCH_READINESS.md`)](docs/LAUNCH_READINESS.md):** taking a new service or initiative to production — workflows, gates, required tools and customizing the playbook skills.
