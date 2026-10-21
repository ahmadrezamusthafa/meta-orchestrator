# Meta-Orchestrator REST API & WebSocket Protocol Reference

**Version:** 1.0.0  
**Base URL:** `http://127.0.0.1:5181/api/v1`  
**WebSocket Endpoint:** `ws://127.0.0.1:5181/ws`  

---

## 1. Overview & Architecture

The Meta-Orchestrator Daemon provides a high-throughput, low-latency REST and WebSocket interface:
* **REST API:** Handles CRUD operations for autonomous tasks, mid-process stage slicing, dependency lifecycle actions, AI provider credentials, benchmark matrices, and artifact inspection.
* **WebSocket Server:** Streams agent console entries, state transitions, approvals, evidence and frustration-halt events.
* **Access:** loopback only. Requests with a non-loopback `Host` or browser `Origin` get `403` (extra names via `MO_ALLOWED_HOSTS`); CORS is reflected for allowed origins only; bodies are capped at 32 MiB.

---

## 2. REST Endpoints

### 2.1 Tasks & Mid-Process Stage Slicing

#### `GET /api/v1/tasks`
Retrieve all active and completed tasks with optional query filters.

* **Query Parameters:**
  * `search` *(string, optional)*: Filter by task ID or title substring (e.g. `TASK-8942`).
  * `method` *(string, optional)*: Filter by assigned execution method (`BMAD`, `Supervisor`, `ReAct`, `Superpower`).
  * `repo` *(string, optional)*: Filter by repository name (e.g. `frontend-portal`).

* **Response (`200 OK`):**
```json
[
  {
    "id": "TASK-8942",
    "workflow_id": "general_ai_sdlc",
    "title": "Implement Stripe payment gateway & webhook idempotency",
    "description": "Add Stripe billing integration across frontend-portal and backend-core...",
    "current_stage_id": "task_implementation",
    "current_stage_index": 4,
    "state": "RUNNING",
    "assigned_repos": ["frontend-portal", "backend-core", "api-contracts"],
    "profile_name": "senior_fullstack_dev",
    "selected_method": "BMAD",
    "token_usage": {
      "prompt_tokens": 18420,
      "completion_tokens": 6210,
      "total_tokens": 24630,
      "estimated_cost_usd": 0.142
    },
    "max_token_budget": 50000,
    "artifact_dir": ".sdlc/artifacts/TASK-8942",
    "metadata": {
      "complexity": "HIGH",
      "router_strategy": "BEST_PRACTICE",
      "router_source": "BP",
      "router_rationale": "High complexity fullstack task routed to BMAD methodology using Claude 3.5 Sonnet"
    },
    "created_at": "2026-09-25T13:10:00Z",
    "updated_at": "2026-09-25T13:45:00Z"
  }
]
```

---

#### `POST /api/v1/tasks`
Launch a new autonomous task. Supports full pipeline execution or **Mid-Process Stage Slicing** (`active_slice`).

* **Request Body:**
```json
{
  "title": "Implement OAuth2 Refresh Token Rotation",
  "description": "Handle silent token refresh and session invalidation",
  "workflow_id": "general_ai_sdlc",
  "assigned_repos": ["frontend-portal", "backend-core"],
  "router_strategy": "BEST_PRACTICE",
  "complexity": "HIGH",
  "max_token_budget": 50000,
  "active_slice": {
    "start_stage_id": "task_implementation",
    "halt_stage_id": "e2e_validation",
    "produce_video": true
  },
  "source_branch": "feat/order-checkout-v2"
}
```

* **Response (`201 Created`):** Returns the initialized `Task` object.
* **Routing plan:** send the reviewed analysis from `POST /api/v1/tasks/analyze` as `complexity`, `complexity_source`,
  `complexity_rationale`, `task_type` and `routing_plan` (`[{stage_id, method, model, tier, overridden}]`); the task is
  stored with `metadata.routing_confirmed = "true"` and every stage runs with its plan entry. Without `routing_plan` the
  server grades complexity with the keyword heuristic (no model call), stores the router's proposal and marks it
  `routing_confirmed = "false"` for the operator to review.

---

#### `POST /api/v1/tasks/analyze`
Grade a draft task's complexity with AI (the Tier 3 model in your chain; keyword heuristic when it is unavailable) and
propose a method and model per stage. Passing `complexity` re-plans with the operator's grade and skips the AI.

* **Request Body:** `{"title", "description", "assigned_repos", "complexity"?, "task_type"?, "start_stage"?, "halt_stage"?, "workflow_id"?}`
* **Response (`200 OK`):**
```json
{
  "assessment": {"complexity": "LOW", "task_type": "docs", "rationale": "README wording only.", "signals": ["readme"],
                 "source": "ai", "model": "claude/claude-haiku-4-5"},
  "plan": [{"stage_id": "prd_discovery", "method": "react", "model": "claude/claude-sonnet-5-5", "tier": "tier2",
            "reasoning": "Best practice: Low complexity: ...", "strategy": "best_practice"}],
  "methods": [{"id": "react", "name": "ReAct", "description": "..."}],
  "models": [{"model": "claude/claude-sonnet-5-5", "tiers": ["tier2"]}]
}
```
The response also carries `skills`, `skill_suggestions`, `evidence_skill`, `evidence_suggestions`, and for launch-readiness
stages `playbooks` (`{stage_id: skill}` — the built-in skill each loads without being attached) and `stage_tools`
(`{stage_id: [{"id", "name", "why", "required", "installed"}]}`). `GET /api/v1/tasks/{id}/routing` returns the same
`playbooks` and `stage_tools` for the task's stages. See [`LAUNCH_READINESS.md`](LAUNCH_READINESS.md).

Best-practice tiers by complexity: LOW runs on Tier 2 (ReAct; Superpower for verification); MEDIUM uses Tier 1 for
planning (BMAD/Supervisor) and Tier 2 for implementation and verification; HIGH uses Tier 1 throughout; SYSTEM uses
Tier 1 with Superpower.

---

#### `GET|PUT /api/v1/tasks/{id}/routing`
Read or replace a task's routing plan. `PUT {"complexity"?, "routing_plan": [...]}` validates stages, methods
(`react`, `bmad`, `supervisor`, `superpower`) and models (must be enabled in the router chain), marks the plan
confirmed and applies from the next stage run.

---

#### `GET /api/v1/tasks/{id}`
Retrieve detailed status and metadata for a specific task.

* **Response (`200 OK`):** Task details object.
* **Error (`404 Not Found`):** `{"error": "Task TASK-9999 not found"}`

---

### 2.2 Human-In-The-Loop (HITL) Controls

#### `POST /api/v1/tasks/{id}/inject`
Inject steering instructions directly into the active agent's prompt context. If the task was halted in `BLOCKED_FRUSTRATION`, this immediately clears the block and resumes execution.

* **Request Body:**
```json
{
  "instruction": "Focus on the Stripe idempotency header key. Ensure the database transaction commits before returning HTTP 200."
}
```

* **Response (`200 OK`):**
```json
{
  "status": "injected",
  "task_id": "TASK-8940",
  "task": { ... }
}
```

---

#### `POST /api/v1/tasks/{id}/reset`
Purge ephemeral Docker container volumes, execute `git reset --hard` to revert uncommitted modifications, reset error counters, and resume the current stage from a pristine checkpoint.

* **Response (`200 OK`):**
```json
{
  "status": "reset_completed",
  "task_id": "TASK-8940",
  "task": { ... }
}
```

---

#### `POST /api/v1/tasks/{id}/gate`
Submit human operator approval or rejection at a gated SDLC boundary (e.g. `techdoc_rfc` or `signoff_merge`).

* **Request Body:**
```json
{
  "approved": true,
  "feedback": "Architecture RFC approved for code generation."
}
```

* **Response (`200 OK`):**
```json
{
  "status": "gate_updated",
  "approved": true,
  "task": { ... }
}
```

---

### 2.3 Tool Hub: Package Manager

The orchestrator's own tools from `configs/packages.json` (see README §10 for what each kind does). State lives in `$MO_HOME` (default `~/.meta-orchestrator`).

* `GET /api/v1/packages` → `{"home", "bin_dir", "on_path", "packages": [Package]}`. Never waits for the network: `latest` is the cached value (`""` until a background lookup finishes; refreshed hourly).
  * `Package` = `{"id","name","category","description","docs","kind","package","required","requires":[id],"status":"installed"|"not_installed"|"error"|"checking","version","latest","update_available","managed","location","system_copy","installed_versions":[{"version","installed_at","active","kind":"install"|"snapshot"}],"actions":{"install","update","rollback","uninstall","pick_version"},"action_notes":{"install"?,"update"?,"rollback"?,"uninstall"?},"manual":{"commands":[string],"note"}|null,"job":Job|null,"history":[{"at","action","from","to","ok","message"}],"notes","problem"?}`.
  * `version` is `"unknown"` when the package is present but its version cannot be read. `managed` is true for an npm-cli/go-cli version linked from `bin_dir`, and for installed npx-installer and claude-plugin packages (which the orchestrator snapshots and restores in place); `system_copy` is an unmanaged copy of an npm-cli/go-cli binary found on `PATH`. `history` is newest first (at most 20). `problem` (additive) explains `status: "error"`.
* `GET /api/v1/packages/{id}/versions[?prerelease=1]` → `{"versions": [string], "latest"}`, newest first, at most 50 (npm registry, `go list -m -versions`, or Homebrew's stable version). `502 {"error", "versions": [], "latest"}` when the registry cannot be reached.
* `POST /api/v1/packages/{id}/install` `{"version"?}` → `202 Job`; `400` for an invalid version, a version on a kind that cannot be pinned (claude-plugin, system) or a missing prerequisite (npm, npx, go, claude); `501 {"error", "manual": {"commands", "note"}}` when it needs the operator's terminal; `409 {"error", "job"}` while a job for the package is queued or running.
* `POST /api/v1/packages/{id}/update` → `202 Job` (to the latest release).
* `POST /api/v1/packages/{id}/rollback` `{"version"?}` → `202 Job`. Without a version: the previously active kept version (npm-cli, go-cli) or the newest snapshot of another version (npx-installer, claude-plugin). A version that is not kept is installed (npm-cli, go-cli, npx-installer). `400` when there is nothing to roll back to; `501` for system packages.
* `POST /api/v1/packages/{id}/uninstall` → `202 Job`; `400` when nothing (managed) is installed.
* `POST /api/v1/packages/setup` → `202 {"jobs": [Job], "skipped": [{"id", "reason"}]}`: installs every required package that is missing, dependencies (`requires`) first, one job (`action: "setup"`) per package run one after another; a failure fails the remaining jobs without running them.
* `POST /api/v1/packages/setup?category=<name or slug>` (e.g. `launch-readiness`): installs every missing package of that category the same way, except that one failing does not stop the others. `400` for an unknown category. CLI: `packages setup [category]`.
* `POST /api/v1/packages/recheck` → same as `GET /api/v1/packages`, after clearing the cached version checks.
* `GET /api/v1/packages/jobs/{id}` → `Job` = `{"id","package","action":"install"|"update"|"rollback"|"uninstall"|"setup","version","from_version","state":"queued"|"running"|"succeeded"|"failed","log":[string],"message","started","finished"}` (`finished` is `""` until done).
* WebSocket event `package.job` carries a `Job` on every state change and, while it logs, at most every 500 ms.
* CLI: `go run ./cmd/daemon packages <list|versions|install|update|rollback|uninstall|setup> [id] [version]` (`make setup` runs `setup`); exits non-zero on failure.

---

### 2.4 AI Providers, Rules & Benchmarks

* `GET /api/v1/providers`: Lists active providers (`Claude`, `Antigravity`, `ChatGPT`, `OpenCode`), masked API keys, base URLs, and active project override status.
* `POST /api/v1/providers/test`: Ping connection and measure latency in milliseconds (`{"provider_id": "claude"}`).
* `GET /api/v1/benchmarks`: Shadow-benchmark measurements per pipeline stage (8) and complexity (4). `matrix` lists **measured cells only** — winning method and model, FPVR %, samples, cost — each with `applied` (routing uses it) or `not_applied_reason` (e.g. `"2 samples (needs 3)"`, `"first-pass 40% (needs 50%)"`). `benchmark_stage` is the stage that was measured (UAT verification shares E2E validation). Also returns `total_cells`, `measured_cells`, `applied_cells`, `min_samples`, `min_fpvr` and `shadow_enabled`. The routing policy for unmeasured cells is the router preview `matrix`, not this endpoint.
* `GET|POST /api/v1/router/settings`, `POST /api/v1/router/preview`: Routing mode and allowed-model chain. Responses include `matrix`: the decision for every pipeline stage × complexity (`stage_id`, `complexity`, `decision`), where `decision.method_source` (`policy`, `benchmark`, `rule`, `user`) and `decision.tier_source` (`policy`, `benchmark`, `calibrated`, `mode`, `rule`, `user`) say where the choice came from, and `decision.suggestion` names a recommended model the chain could not run. Built-in models always carry the catalog's name, price and latency; saved chain values never override them. Modes report `complexity_aware`; chain modes (priority, cost, latency, round-robin) order the models while the method still follows stage and complexity.

---

### 2.5 Workflows, Hooks & Registries

* `GET /api/v1/workflows`: Built-in workflows (`general_ai_sdlc`, hotfix, microservice, `launch_readiness`, `new_service_sdlc`; read-only) and custom ones. A task's `workflow_id` decides its stages. A stage's `gate_criteria` (items separated by `;`) is the checklist the operator confirms; it is shown in the console when the stage finishes.
* `POST /api/v1/workflows`, `PUT /api/v1/workflows/{id}`: Create or update a custom workflow. Every stage must be runnable (`400` otherwise); built-ins return `409`. Saved to `.sdlc/workflows.json`; `.sdlc/workflow.yaml` is loaded at startup.
* `GET /api/v1/workflows/stages`: Stage IDs a workflow may use.
* `GET /api/v1/hooks`: Configured lifecycle hooks (`.sdlc/hooks.json` and `.sdlc/hooks/<event>.sh`); webhook URLs show scheme and host only.
* `GET /api/v1/registries`: The skills, hooks, methods (`configs/methods.json`) and stage roles (`configs/profiles.json`) actually loaded, with unreadable files under `problems`.
* `POST /api/v1/skills/{name}/run` `{"role"}`: Runs a command-based skill once in the project root (60 s, 64 KiB of output) as a stage role; refused (`403`) when the role is not allowed or denies the skill.

---

### 2.6 Artifacts, Evidence & Outline

* `GET /api/v1/artifacts/{taskId}/{path}`: A task artifact — stage documents, `atdd/acceptance.spec.ts`, and evidence files (videos, screenshots, `evidence.json`, `EVIDENCE.md`, `evidence-manifest.json`). Paths and symlinks must stay inside the task's artifact directory.
* `GET|POST /api/v1/tasks/{id}/evidence`: Evidence setting and recorded files; POST records evidence for the implementation already in the worktree.
* `GET /api/v1/tasks/{id}/evidence/verify`: Recomputes every file's SHA-256 and the manifest's HMAC-SHA256; lists changed, missing and added files.
* `GET /api/v1/tasks/{id}/outline?repo={repo}&path={file}`: Declarations in one file of the task worktree (regex outline for Go, TS/JS/Vue, PHP, Python; `415` for other types).

---

### 2.7 Projects & Multi-Repo Workspaces

* `GET /api/v1/projects`: List all registered multi-repo projects.
* `POST /api/v1/projects`: Register a new project, provision its unified project root on disk, and synthesize atomic filesystem symlinks to all tagged repositories.
* `GET /api/v1/projects/{id}`: Retrieve project topology, mapped repositories, manifest types, and symlink statuses.
* `POST /api/v1/projects/{id}/resync`: Re-evaluate repository source paths and re-create symlinks.
* `DELETE /api/v1/projects/{id}`: Delete a project and remove its ephemeral workspace root.
* `POST /api/v1/projects/{id}/suggest-repos` `{"title","description"}`: Ranks the project's repositories by whole-word evidence in the draft task; nothing is selected automatically.
* `POST /api/v1/projects/scan`: Auto-scan a directory path, discover child repositories, identify language manifests (`package.json`, `go.mod`, `playwright.config.ts`, `openapi.yaml`), and suggest roles (`frontend`, `backend`, `automation-test`, `contracts`, `artifact`).

#### Repository dependencies

Project dependencies (React, Go modules, Flutter packages, …) of each registered repository, per ecosystem. Paths always come from the projects manager (or the task's own worktree), never from the request.

* `GET /api/v1/projects/{id}/dependencies[?task={taskId}][&latest=1]` → `{"project_id","repos":[RepoDeps]}`. With `task`, only the repositories assigned to that task are listed, read from the task's worktree (a repository whose worktree does not exist yet is left out; a task with worktrees disabled uses the source folder). `latest=1` adds `latest` to Node dependencies via `npm view <name> version` (at most 4 at a time, cached 1 h); the default GET never contacts a registry.
  * `RepoDeps = {"repo","path","ecosystems":[Ecosystem],"job":Job|null}` — `job` is the repository's most recent install.
  * `Ecosystem = {"ecosystem","manager","manifest","lockfile","installed","drift","install":{"command":[…],"auto","note"},"missing_tool","dependencies":[{"name","wanted","installed","latest","dev","key"}]}`. One entry per ecosystem (the shallowest manifest). `installed`: node — `node_modules` exists and its `.package-lock.json`/`.modules.yaml`/`.yarn-integrity`/`.yarn-state.yml` is not older than the lockfile and no installed version differs from it; go — every required module is in `GOMODCACHE`; flutter/dart — `.dart_tool/package_config.json` is not older than `pubspec.lock`; python — a project `.venv` exists; ruby/jvm — not checked (`false`, said in `install.note`). `drift` is a sentence when what is installed disagrees with the lockfile, else `""`. `missing_tool` is the toolchain id (`node`, `pnpm`, `yarn`, `bun`, `go`, `flutter`, `dart`, `java`, `maven`, `gradle`) to install when the manager binary is missing. `key` flags frameworks (react, vue, next, @angular/core, vite, typescript, jest, vitest, express, gin, echo, fiber, grpc, gorm, pgx, flutter_bloc, riverpod, dio, …).

| Lockfile / manifest | `install.command` | `auto` |
|---|---|---|
| `package-lock.json`, `npm-shrinkwrap.json` | `npm ci` | yes |
| `pnpm-lock.yaml` | `pnpm install --frozen-lockfile` | yes |
| `yarn.lock` | `yarn install --frozen-lockfile` (Yarn 1) / `yarn install --immutable` (Berry: `.yarnrc.yml`, `packageManager: yarn@2+` or a Berry lockfile) | yes |
| `bun.lock`, `bun.lockb` | `bun install --frozen-lockfile` | yes |
| `package.json` only | `npm install` (or the declared manager), note "No lockfile — versions may drift." | yes |
| `go.mod` | `go mod download` | yes |
| `pubspec.yaml` | `flutter pub get` / `dart pub get` | yes |
| `Gemfile(.lock)` | `bundle install` | yes |
| `uv.lock` / `poetry.lock` / `Pipfile.lock` | `uv sync --frozen` / `poetry install --no-root` / `pipenv sync` | yes |
| `requirements.txt` | `.venv/bin/python -m pip install -r requirements.txt` when `.venv` exists, else `python3 -m pip …` to copy | only with `.venv` |
| `pom.xml` / `build.gradle(.kts)` | `./mvnw -q dependency:resolve` / `./gradlew dependencies` with the wrapper, else `mvn …` / `gradle …` to copy | only with the wrapper |

* `POST /api/v1/projects/{id}/dependencies/install` `{"repo","ecosystem","task"?}` → `202 Job`. Runs the frozen install in the repository's registered folder (or the task's worktree; the repository must be assigned to the task and its worktree must exist, else `400`). Running it again is the rollback of local drift: it restores `node_modules` & co. to the lockfile. No shell (argv only), `CI=true`, 20-minute timeout that kills the whole process group, one install per repository at a time, output streamed into `log` with credentials masked. `501 {"error","install","missing_tool"}` when the command is not run automatically or its binary is missing; `409 {"error","job"}` while the repository already has an install running; `404` for an unknown project, repository, task or ecosystem.
* `POST /api/v1/projects/{id}/dependencies/upgrade` `{"repo","ecosystem","name","target","note"?}` → `201 {"task": Task}`. A version change is a code change, so it becomes a task created through `POST /api/v1/tasks` itself: title `Upgrade <name> to <target> in <repo>`, the repository assigned, starting at `task_implementation` (when the default workflow has it), and a description with the exact steps — update manifest and lockfile together with the repository's manager (e.g. `npm install react@19.1.0`, `pnpm add`, `go get …@… && go mod tidy`, `flutter pub add`), move companion packages, fix breaking changes, run the repository's checks. The agent works in the task worktree, the change is reviewed in a pull request, and rollback is reverting it. `name` must be a dependency of the manifest (`404` otherwise); `name`/`target` are validated (`400`).
* `GET /api/v1/dependencies/jobs/{id}` → `Job = {"id","package":"repo:<repo>","action":"install","version":"","from_version":"","state":"queued"|"running"|"succeeded"|"failed","log":[…],"message","started","finished"}`.
* WebSocket `dependency.job` carries the `Job` on every state change and at most every 500 ms while it logs.

---

## 3. WebSocket Streaming Protocol

* **URL:** `ws://127.0.0.1:5181/ws`
* **Filter by Task:** `ws://127.0.0.1:5181/ws?task_id=TASK-8942`

### Message Schema
```json
{
  "type": "event.type",
  "task_id": "TASK-8942",
  "stage_id": "task_implementation",
  "timestamp": "2026-09-25T13:42:01.123Z",
  "payload": { ... }
}
```

### Event Types
| Event Type | Description | Payload Shape |
|---|---|---|
| `agent.terminal` | Real-time stdout/stderr stream from container sandboxes | `{"stream": "stdout", "chunk": "..."}` |
| `agent.thought` | Agent reasoning vectors, decisions, and tool calls | `{"profile": "Lead Architect", "model": "claude-3-5-sonnet", "thought": "...", "tool_call": { ... }}` |
| `task.status` | State machine transition update | Serialized `Task` object |
| `dependency.job` | Repository dependency install progress (state changes, then at most every 500 ms while logging) | Serialized dependency `Job` |
| `frustration.halt` | Circuit breaker tripped on $\ge 3$ consecutive errors | `{"consecutive_fails": 3, "failing_trace": "...", "error_hash": "..."}` |
| `evidence.video.ready` | Playwright `.mp4` test execution recording finalized | `{"video_path": ".sdlc/artifacts/TASK-8942/run_final.mp4", "url": "..."}` |
