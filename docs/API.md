# Meta-Orchestrator REST API & WebSocket Protocol Reference

**Version:** 1.0.0  
**Base URL:** `http://localhost:8080/api/v1`  
**WebSocket Endpoint:** `ws://localhost:8080/ws`  

---

## 1. Overview & Architecture

The Meta-Orchestrator Daemon provides a high-throughput, low-latency REST and WebSocket interface:
* **REST API:** Handles CRUD operations for autonomous tasks, mid-process stage slicing, dependency lifecycle actions, AI provider credentials, benchmark matrices, and artifact inspection.
* **WebSocket Server:** Multiplexes streaming stdout/stderr terminal chunks, agent reasoning vectors, state transitions, and frustration circuit-breaker interrupts with sub-100ms latency.

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
  "selected_method": "Auto",
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

### 2.3 Tool Hub & Dependency Lifecycle

* `GET /api/v1/tools?category={category}&search={search}`: Catalog of tools (`BMAD`, `Superpower`, `Playwright`, `Tree-sitter`, `xterm.js`, `ffmpeg`) with active vs latest versions and health diagnostics.
* `POST /api/v1/tools/install`: Trigger step-by-step guided installation (`{"tool_id": "playwright", "version": "v1.49.0"}`).
* `POST /api/v1/tools/rollback`: Instant atomic rollback via symlink swap (`{"tool_id": "bmad", "target_version": "v1.0.0"}`).
* `POST /api/v1/tools/resolve-matrix`: Compute and apply the optimal conflict-free toolchain matrix for the host OS and CPU architecture.

---

### 2.4 AI Providers, Rules & Benchmarks

* `GET /api/v1/providers`: Lists active providers (`Claude`, `Antigravity`, `ChatGPT`, `OpenCode`), masked API keys, base URLs, and active project override status.
* `POST /api/v1/providers/test`: Ping connection and measure latency in milliseconds (`{"provider_id": "claude"}`).
* `GET /api/v1/benchmarks`: Returns the 36-cell benchmark matrix (9 SDLC Stages $\times$ 4 Complexities) with winning methods, models, and First-Pass Verification Rates (FPVR %). Each cell carries `source`: `shadow_benchmark` (measured) or `default` (best-practice policy filling cells no sweep has measured). `measured_cells` counts the measured ones.

---

### 2.5 Workflows & Multi-Source Registries

* `GET /api/v1/workflows`: Returns installed SDLC workflows (General AI SDLC, Hotfix Fast-Track, Microservice API).
* `POST /api/v1/workflows`: Ingests or updates a bespoke SDLC workflow definition matching `.sdlc/workflow.yaml`.
* `GET /api/v1/registries`: Returns multi-source discovery results:
  * **Skills:** Remote Git repos, Project Local (`.sdlc/skills/`), System (`~/.config/meta-orchestrator/skills/`), and Built-in tools.
  * **Prompts:** Cascading templates with slot variable schemas and source precedence flags.
  * **Hooks:** Event interceptors (`pre-stage`, `on-gate`, `pre-commit`) with policies (`BLOCK` vs `WARN`).

---

### 2.6 Artifacts Vault

* `GET /api/v1/artifacts/{taskId}/{filename}`: Retrieve parsed markdown schema artifact or media asset (`PRD.md`, `ATDD_SUITE.md`, `TECH_DOC_RFC.md`, `TASK_PLAN.md`, `UAT_PREPARATION.md`, `EVIDENCE.md`, `run_final.mp4`).

---

### 2.7 Projects & Multi-Repo Workspaces

* `GET /api/v1/projects`: List all registered multi-repo projects.
* `POST /api/v1/projects`: Register a new project, provision its unified project root on disk, and synthesize atomic filesystem symlinks to all tagged repositories.
* `GET /api/v1/projects/{id}`: Retrieve project topology, mapped repositories, manifest types, and symlink statuses.
* `POST /api/v1/projects/{id}/resync`: Re-evaluate repository source paths and re-create symlinks.
* `DELETE /api/v1/projects/{id}`: Delete a project and remove its ephemeral workspace root.
* `POST /api/v1/projects/scan`: Auto-scan a directory path, discover child repositories, identify language manifests (`package.json`, `go.mod`, `playwright.config.ts`, `openapi.yaml`), and suggest roles (`frontend`, `backend`, `automation-test`, `contracts`, `artifact`).

---

## 3. WebSocket Streaming Protocol

* **URL:** `ws://localhost:8080/ws`
* **Filter by Task:** `ws://localhost:8080/ws?task_id=TASK-8942`

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
| `frustration.halt` | Circuit breaker tripped on $\ge 3$ consecutive errors | `{"consecutive_fails": 3, "failing_trace": "...", "error_hash": "..."}` |
| `evidence.video.ready` | Playwright `.mp4` test execution recording finalized | `{"video_path": ".sdlc/artifacts/TASK-8942/run_final.mp4", "url": "..."}` |
