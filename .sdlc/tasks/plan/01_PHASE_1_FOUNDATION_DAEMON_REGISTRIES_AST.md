# Phase 1 Execution Plan: Foundation Daemon, Registries & AST Core

**Document ID:** PLAN-001  
**Phase:** 1 of 5 (Weeks 1–4)  
**Status:** Completed & 100% Verified (All Tests Passing)  
**Primary Focus:** Concurrency Core, Redis FSM, Modular Registries, AST Sharding & Artifact Protocol  
**PRD References:** Pillar 1, Pillar 2, Pillar 5, Section 6, Section 11 (Phase 1)  

---

## 1. Phase Overview & Objectives

Phase 1 establishes the bedrock of the Meta-Orchestrator:
1. A resilient, persistent background execution daemon written in **Go** backed by **Redis** for state serialization and job queues.
2. A strict **Anti-Loop Breaker** that detects repeating failures and halts runaway execution into `BLOCKED_FRUSTRATION`.
3. The **Modular Registry System** managing Agent Profiles, Skills, Parameterized Prompts, and Workflow State Machines (`BMAD`, `Supervisor`, `ReAct`, `Superpower`).
4. An **AST-Based Context Sharding Engine** utilizing Tree-sitter parsers to slice codebases into pinpoint syntactic contexts, paired with the **Artifact-Driven Memory Protocol** (`.sdlc/artifacts/{task_id}/`).

---

## 2. Work Breakdown Structure (WBS) & Task Status

```
Phase 1: Foundation Daemon, Registries & AST Core [COMPLETED 100%]
├── Epic 1.1: Background Execution Daemon & Event Core (Go & Redis) [DONE]
│   ├── [x] TASK-1.1.1: Go Worker Pool & Distributed Task Queue
│   ├── [x] TASK-1.1.2: Persistent FSM, Slicing & Custom SDLC Workflow Engine (<5s RTO)
│   ├── [x] TASK-1.1.3: Anti-Loop Frustration Threshold Circuit Breaker
│   └── [x] TASK-1.1.4: Real-time WebSocket Streaming Server (Thought & Terminal)
├── Epic 1.2: Modular Schema Registries [DONE]
│   ├── [x] TASK-1.2.1: Profile Registry (profiles.json) & RBAC Enforcer
│   ├── [x] TASK-1.2.2: Modular Skill Registry & Polyglot Multi-Source Adapter Engine (BMAD, Claude, Superpower, MCP)
│   ├── [x] TASK-1.2.3: Parameterized Prompt Registry & Multi-Source Template Loader
│   ├── [x] TASK-1.2.4: Method Registry (methods.json) & FSM Engine
│   ├── [x] TASK-1.2.5: Tool Manifest Specification (tools.json) & Package Registry
│   ├── [x] TASK-1.2.6: Tool Lifecycle Engine (Step-by-Step Installer, Rollback & Best-Fit Resolver)
│   └── [x] TASK-1.2.7: Pluggable Lifecycle Hooks Engine & Execution Runner (Shell, Docker, Webhooks)
└── Epic 1.3: AI Routing Core & AST Context Sharding Engine [DONE]
    ├── [x] TASK-1.3.1: Dynamic Task Profiler & AST Impact Scoper
    ├── [x] TASK-1.3.2: Multi-Tiered LLM Router & Token Budget Dispatcher
    ├── [x] TASK-1.3.3: AST Tree-sitter Code Sharder (TS, PHP, Python, Go)
    ├── [x] TASK-1.3.4: Artifact-Driven Memory Store (.sdlc/artifacts/{task_id}/)
    ├── [x] TASK-1.3.5: Multi-Provider LLM Driver Interface (Claude, Antigravity, ChatGPT, OpenCode)
    ├── [x] TASK-1.3.6: Cascading Configuration Engine (System vs. Per-Project .sdlc/config.yaml)
    └── [x] TASK-1.3.7: Dual-Strategy Router Engine (Best Practice Heuristics & Custom Rule Matcher)
```

---

## 3. Detailed Task Specifications

### Epic 1.1: Background Execution Daemon & Event Core

#### TASK-1.1.1: Go Worker Pool & Distributed Task Queue
* **Objective:** Implement the asynchronous concurrency core capable of running up to 10 concurrent multi-repo tasks per worker node without blocking the control plane.
* **Technical Scope:**
  - Initialize Go project structure under `cmd/daemon/` and `internal/worker/`.
  - Implement Redis-backed BullMQ-compatible job queue with retry policies, exponential backoff, and concurrency semaphores.
  - Support worker process isolation and graceful shutdown signals (`SIGINT`, `SIGTERM`).
* **Deliverable Files:**
  - `cmd/daemon/main.go`
  - `internal/queue/redis_queue.go`
  - `internal/worker/pool.go`
  - `internal/worker/worker.go`
* **Verification Criteria:**
  - Automated concurrency test submitting 20 concurrent simulated jobs; verifies maximum 10 active concurrent workers with queueing and zero race conditions under `-race`.

---

#### TASK-1.1.2: Persistent FSM, Slicing & Custom SDLC Workflow Engine (<5s RTO)
* **Objective:** Build the core state machine execution engine supporting both the default 9-stage General AI SDLC and arbitrary user-defined Custom SDLC pipelines (via `.sdlc/workflow.yaml`), with partial stage slicing and <5s crash recovery RTO.
* **Technical Scope:**
  - Multi-SDLC FSM Driver:
    - Default State Machine: General AI SDLC (`INTAKE_PRD` → `REPO_DISCOVERY` → `ATDD_RED_PHASE` → `TECH_DOC_RFC` → `GATE_TECH_DOC_REVIEW` → `TASK_BREAKDOWN` → `IMPLEMENTATION_GREEN` → `E2E_AUTOMATION` → `UAT_EVIDENCE` → `READY_FOR_SIGNOFF`).
    - Custom SDLC Parser: Load declarative workflow from `.sdlc/workflow.yaml`, project config, or system templates (`hotfix-fast-track`, `microservice-api`). Dynamically instantiate states, role handoffs, write-lock rules, and gates.
  - Partial Stage Slicing (`slice_hydrator.go`): allow execution bounded by `StartStage` and `HaltStage` (e.g. Stage 7 Implementation $\to$ Stage 8 E2E Tests).
  - Snapshot & Serialization: persist full FSM state, active slice, and task memory to Redis (`orchestrator:task:{task_id}:state`) upon every transition.
  - Crash Recovery: automatically query uncompleted tasks on daemon startup and restore exact workflow state in `< 5s`.
* **Deliverable Files:**
  - `internal/fsm/state_machine.go`
  - `internal/fsm/custom_workflow.go`
  - `internal/fsm/slice_hydrator.go`
  - `internal/fsm/recovery.go`
  - `internal/fsm/custom_workflow_test.go`
* **Verification Criteria:**
  - Ingest a custom 3-stage `.sdlc/workflow.yaml`; verify FSM registers stages, enforces custom gate, survives `kill -9` process interruption, and recovers state in `< 3.5s`.

---

#### TASK-1.1.3: Anti-Loop Frustration Threshold Circuit Breaker
* **Objective:** Halt execution loops when agents repeat identical errors or fail test suites beyond a configurable threshold (default: 3 iterations).
* **Technical Scope:**
  - Track iteration history: tool call hashes, error stack traces, and exit codes per state.
  - If identical tool invocation or test failure occurs $\ge 3$ consecutive times:
    1. Set task state to `BLOCKED_FRUSTRATION`.
    2. Freeze token consumption and suspend container execution.
    3. Emit `event.frustration_halt` payload with error diff and stack trace to Redis Pub/Sub.
* **Deliverable Files:**
  - `internal/circuitbreaker/frustration.go`
  - `internal/circuitbreaker/frustration_test.go`
* **Verification Criteria:**
  - Unit test simulating 3 consecutive identical Playwright failure traces; asserts transition to `BLOCKED_FRUSTRATION` and emission of the halt event.

---

#### TASK-1.1.4: Real-time WebSocket Streaming Server (Thought & Terminal)
* **Objective:** Deliver streaming logs and agent thought reasoning to the frontend with sub-100ms latency.
* **Technical Scope:**
  - Implement WebSocket hub under `internal/ws/` supporting bidirectional messaging.
  - Event channels:
    - `agent.thought`: profile name, model ID, markdown thought chunks, tool invocation metadata.
    - `agent.terminal`: raw ANSI stdout/stderr chunks from Docker container bash executions.
    - `task.status`: FSM transition notifications and metric updates.
    - `hitl.inject`: inbound operator steering instructions.
* **Deliverable Files:**
  - `internal/ws/server.go`
  - `internal/ws/client.go`
  - `internal/ws/events.go`
* **Verification Criteria:**
  - Benchmark test measuring packet latency from Go emitter to connected WebSocket client; must maintain `< 40ms` average latency for 1000 events/sec.

---

### Epic 1.2: Modular Schema Registries

#### TASK-1.2.1: Profile Registry (`profiles.json`) & RBAC Enforcer
* **Objective:** Establish the agent persona configuration registry with strict role-based permission boundaries.
* **Technical Scope:**
  - Define JSON-Schema for profiles (`schemas/profile.schema.json`).
  - Implement `configs/profiles.json` covering `laravel_backend_architect`, `vue_frontend_engineer`, `atdd_qa_engineer`, and `devops_superpower`.
  - Implement Go RBAC validator preventing unprivileged profiles from invoking restricted skills (e.g. `vue_frontend_engineer` cannot run `docker_prune` or system-level host commands).
* **Deliverable Files:**
  - `schemas/profile.schema.json`
  - `configs/profiles.json`
  - `internal/registry/profiles.go`
  - `internal/registry/profiles_test.go`
* **Verification Criteria:**
  - Validation test rejecting malformed profiles; security unit test blocking unauthorized skill invocation for `vue_frontend_engineer`.

---

#### TASK-1.2.2: Modular Skill Registry & Polyglot Multi-Source Adapter Engine (BMAD, Claude, Superpower, MCP)
* **Objective:** Build an extensible, format-agnostic skill discovery and execution engine that ingests tools across disparate ecosystem formats (BMAD skills, Claude `SKILL.md`, Superpower shell tools, Model Context Protocol servers, and OpenAI functions) from remote Git repositories, local project paths (`.sdlc/skills/`), system directories, or built-in defaults, normalizing them into a `UniversalSkillContract`.
* **Technical Scope:**
  - Multi-Source Cascading Discovery:
    - `Project Local`: Scans `.sdlc/skills/{skill_name}/`.
    - `User System`: Scans `~/.config/meta-orchestrator/skills/`.
    - `Remote Repositories`: Clones and syncs Git repositories (e.g. `https://github.com/my-org/ai-skills.git`).
    - `Built-in`: Core tools in `configs/skills.json` (`resolve_symlinks`, `docker_compose_up`, `run_playwright_e2e`, `read_ast_node`, `git_atomic_commit`, `capture_viewport_video`).
  - Polyglot Skill Format Adapters:
    - **Claude Skill Adapter (`adapter_claude.go`):** Parses Anthropic `SKILL.md` bundles (extracts YAML frontmatter `name:`, `description:` + instructions body + execution scripts in `scripts/`). Translates Anthropic `tool_use` JSON schemas.
    - **BMAD Skill Adapter (`adapter_bmad.go`):** Ingests BMAD multi-agent skill packs, role bindings, and sequential handoff contracts.
    - **Superpower Skill Adapter (`adapter_superpower.go`):** Ingests bare-metal command tools, shell execution scripts, and environment configuration harnesses.
    - **Model Context Protocol Client (`adapter_mcp.go`):** Implements MCP client protocol over stdio and Server-Sent Events (SSE) transports, dynamically discovering tools from external MCP servers.
    - **OpenAI Tool Adapter (`adapter_openai.go`):** Ingests standard JSON-Schema function calling definitions.
  - Universal Skill Contract (`universal_contract.go`):
    - Normalizes diverse tool formats into a unified execution contract: argument validation, isolation level enforcement, timeout handlers, and streaming stdout/stderr buffers.
  - Sandbox Invocation Guard: enforces that skills only run inside designated Docker containers or restricted subprocesses.
* **Deliverable Files:**
  - `schemas/skill.schema.json`
  - `configs/skills.json`
  - `internal/registry/skills.go`
  - `internal/skills/universal_contract.go`
  - `internal/skills/adapter_claude.go`
  - `internal/skills/adapter_bmad.go`
  - `internal/skills/adapter_superpower.go`
  - `internal/skills/adapter_mcp.go`
  - `internal/skills/adapter_openai.go`
  - `internal/skills/multi_source_resolver.go`
  - `internal/skills/executor.go`
  - `internal/skills/universal_adapter_test.go`
* **Verification Criteria:**
  - Ingest 4 test tools in different formats: a Claude `SKILL.md` folder, a BMAD skill bundle, a Superpower shell tool, and a mock stdio MCP server; verify all 4 normalize to `UniversalSkillContract`, pass schema validation, and execute in container sandboxes.

---

#### TASK-1.2.3: Parameterized Prompt Registry & Multi-Source Template Loader
* **Objective:** Implement a version-controlled prompt template engine that discovers templates from remote repositories, local project directories (`.sdlc/prompts/`), or system directories, with cascading precedence, hot-reloading, and dynamic slot-filling.
* **Technical Scope:**
  - Multi-Source Discovery:
    - Precedence: `.sdlc/prompts/` (Project Local) > `~/.config/meta-orchestrator/prompts/` (System) > Remote Prompt Repositories > Built-in defaults.
  - Template Engine with Semantic Slot-Filling:
    - Supports placeholders: `{{system_architecture}}`, `{{failing_test_traces}}`, `{{target_ast_slice}}`, `{{task_spec}}`, `{{custom_context}}`.
  - Live Hot-Reloading:
    - Filesystem watcher (`fsnotify`) triggers instantaneous template re-compilation on disk changes without daemon restarts.
* **Deliverable Files:**
  - `prompts/base_system.md`
  - `prompts/qa_atdd_v1.md`
  - `prompts/developer_v1.md`
  - `internal/registry/prompts.go`
  - `internal/registry/prompt_resolver.go`
  - `internal/registry/prompts_test.go`
* **Verification Criteria:**
  - Place a local override in `.sdlc/prompts/developer_v1.md`; assert orchestrator resolves the local override instead of system/built-in; modify template and verify hot-reload takes effect in `< 50ms`.

---

#### TASK-1.2.4: Method Registry (`methods.json`) & FSM Engine
* **Objective:** Define and execute the multi-agent finite state machines for BMAD, Supervisor, ReAct, and Superpower workflows.
* **Technical Scope:**
  - Define `schemas/method.schema.json` and `configs/methods.json`.
  - Configure **BMAD**: Sequential `Product Manager` → `QA Architect (ATDD)` → `System Architect` → `Developer` → `QA Verifier`.
  - Configure **Supervisor**: Parallel execution coordinating frontend and backend subagents.
  - Configure **ReAct**: Rapid single-agent tool iteration loop.
  - Configure **Superpower**: Unrestricted plan-and-execute infrastructure loop.
* **Deliverable Files:**
  - `schemas/method.schema.json`
  - `configs/methods.json`
  - `internal/registry/methods.go`
* **Verification Criteria:**
  - Integration test asserting FSM transitions correctly through all roles for BMAD and flags illegal transitions.

---

#### TASK-1.2.5: Tool Manifest Specification (`tools.json`) & Package Registry
* **Objective:** Formalize declarative metadata, runtime prerequisites, pre-flight test commands, version histories, and rollback strategies for all orchestrator tools (BMAD, Superpower, Playwright, Tree-sitter, xterm.js, ffmpeg).
* **Technical Scope:**
  - Define `schemas/tool.schema.json` specifying: tool ID, category (`methodology`, `runtime`, `parser`, `ui`), current version, supported versions array, dependency matrix (Node, Go, Python, Docker versions), installation scripts, and health-check diagnostics.
  - Implement `configs/tools.json` registering default packages:
    - `bmad-methodology`: Agile BMAD multi-agent engine.
    - `superpower-runtime`: Autonomous infrastructure refactoring framework.
    - `playwright-engine`: Headless/headful browser automation and video capture.
    - `treesitter-parsers`: Multi-language AST sharding libraries.
    - `xterm-terminal-pack`: Terminal emulation and addon bindings.
* **Deliverable Files:**
  - `schemas/tool.schema.json`
  - `configs/tools.json`
  - `internal/registry/tools.go`
  - `internal/registry/tools_test.go`
* **Verification Criteria:**
  - JSON-Schema validation passes for all tool definitions; rejects incomplete packages missing health-check commands or rollback strategies.

---

#### TASK-1.2.6: Tool Lifecycle Engine (Step-by-Step Installer, Rollback & Best-Fit Resolver)
* **Objective:** Build backend engine executing step-by-step guided installations, atomic version upgrades, 1-click rollbacks, and environment-aware "best-fit" version calculations.
* **Technical Scope:**
  - Implement 4-stage pipeline: `1. Pre-flight env check` → `2. Fetch & Build` → `3. Self-Test / Verification` → `4. Register & Activate`.
  - Stream install/update progress output in real-time over WebSocket event `tool.install.progress`.
  - Implement Version Rollback Store: maintains isolated version directories (`/tools/versions/{tool_id}/{version}`) and manages atomic active symlinks (`/tools/active/{tool_id}`). Swapping active version completes in `< 1 second`.
  - Implement Best-Fit Heuristic Resolver: inspects host OS, architecture (Apple Silicon ARM64, Linux x86_64), Node/Go/Docker versions, and glibc/musl compatibility to resolve the optimal, conflict-free tool version matrix.
* **Deliverable Files:**
  - `internal/tools/installer.go`
  - `internal/tools/rollback.go`
  - `internal/tools/bestfit.go`
  - `internal/tools/installer_test.go`
* **Verification Criteria:**
  - Install a mock tool through all 4 steps; upgrade to v2; execute 1-click rollback to v1; assert active version reverts in `< 1s` and passes self-test diagnostics.

---

#### TASK-1.2.7: Pluggable Lifecycle Hooks Engine & Execution Runner (Shell, Docker, Webhooks)
* **Objective:** Implement an event-driven lifecycle hooks engine that executes custom scripts, containers, and webhooks at key SDLC transition points across project-local and system scopes.
* **Technical Scope:**
  - Interception Points: `pre-stage`, `post-stage`, `on-failure`, `on-gate`, `pre-commit`, `post-commit`.
  - Multi-Source Cascading Hook Configuration:
    - Project Local: `.sdlc/hooks/{event_name}.sh` or `.sdlc/hooks.yaml`.
    - User System: `~/.config/meta-orchestrator/hooks/`.
    - Remote Hook Packs: downloaded git repos with hook manifests.
  - Execution Drivers:
    - `ShellHook`: runs local executables with injected environment variables (`META_TASK_ID`, `META_STAGE`, `META_WORKSPACE_PATH`).
    - `DockerHook`: spins up ephemeral container (e.g. running Semgrep, Trivy, SonarQube) against the workspace.
    - `WebhookHook`: dispatches JSON payloads to HTTP endpoints (Slack, Discord, MS Teams, PagerDuty, CI systems).
  - Error Handling & Policy: support `blocking` (aborts pipeline on non-zero exit) vs `warning` (logs and continues).
* **Deliverable Files:**
  - `schemas/hooks.schema.json`
  - `internal/hooks/engine.go`
  - `internal/hooks/shell_runner.go`
  - `internal/hooks/docker_runner.go`
  - `internal/hooks/webhook_runner.go`
  - `internal/hooks/engine_test.go`
* **Verification Criteria:**
  - Configure a `pre-commit` shell hook and `post-stage` webhook; verify shell hook executes before commit and blocks on simulated error; verify webhook sends payload to mock HTTP server.

---

### Epic 1.3: AI Routing Core & AST Context Sharding Engine

#### TASK-1.3.1: Dynamic Task Profiler & Multi-Repository Discovery Engine
* **Objective:** Parse incoming feature requests, synthesize `PRD.md`, dynamically discover all affected repositories across the organization workspace via AST/manifest traversal, and resolve the active SDLC workflow (General AI SDLC or Custom SDLC) into a structured `TaskProfile`.
* **Technical Scope:**
  - Ingest prompts, user stories, or tickets; resolve target workflow (`General AI SDLC` or Custom SDLC from `.sdlc/workflow.yaml` / user selection); invoke PM Agent to generate initial `PRD.md` or custom intake artifact.
  - Implement Dynamic Repository Discovery Engine:
    - Traverses registered organizational git repositories.
    - Inspects package manifests (`package.json`, `composer.json`, `go.mod`, `.gitmodules`) and AST cross-repo route definitions / API contracts.
    - Automatically computes dependency impact graph and flags coupled repositories (e.g. `frontend-app`, `core-backend-api`, `api-contracts`).
  - Query AST engine to compute impacted classes, methods, and files within identified repositories.
  - Generate structured `TaskProfile` object with active `WorkflowID`, maximum token budget, recommended method, list of dynamically identified repos, and required container services.
* **Deliverable Files:**
  - `internal/router/profiler.go`
  - `internal/router/repo_discovery.go`
  - `internal/router/profiler_test.go`
  - `pkg/types/task_profile.go`
* **Verification Criteria:**
  - Benchmark against cross-repo feature ticket; verify engine dynamically identifies both frontend and backend repositories without explicit user declaration, and sets correct `WorkflowID` in `TaskProfile`.

---

#### TASK-1.3.2: Multi-Tiered LLM Router & Token Budget Dispatcher
* **Objective:** Implement cost-optimized model routing across Tier 1 (Reasoning), Tier 2 (Code Gen), and Tier 3 (Log Parsing).
* **Technical Scope:**
  - Configure model tiers:
    - **Tier 1:** Claude 3.5 Sonnet / GPT-4o for Architecture, Profiling, ATDD design.
    - **Tier 2:** Claude 3.5 Sonnet / GPT-4o / DeepSeek-Coder for code generation.
    - **Tier 3:** Claude 3.5 Haiku / GPT-4o-mini for log parsing and git diff summaries.
  - Implement token tracking counter that enforces `max_token_budget` per profile and task.
  - Support automatic fallback to secondary model on rate limit or API 5xx errors.
* **Deliverable Files:**
  - `internal/router/model_router.go`
  - `internal/router/budget_tracker.go`
  - `internal/llm/client_factory.go`
* **Verification Criteria:**
  - Mock test verifying Tier 1 is routed for architecture tasks, Tier 3 for compiler log analysis, and fallback triggers upon primary model HTTP 429.

---

#### TASK-1.3.3: AST Tree-sitter Code Sharder (TS, PHP, Python, Go)
* **Objective:** Prevent LLM context bloat by indexing repositories with Tree-sitter and extracting only relevant symbols, class signatures, and interfaces.
* **Technical Scope:**
  - Integrate Tree-sitter parsers for TypeScript, PHP, Python, and Go via CGO/pure bindings.
  - Extract class signatures, method declarations, route definitions, and interface contracts.
  - Construct localized context slices with bi-directional reference links instead of full source files.
* **Deliverable Files:**
  - `internal/ast/parser.go`
  - `internal/ast/treesitter_ts.go`
  - `internal/ast/treesitter_php.go`
  - `internal/ast/treesitter_go.go`
  - `internal/ast/sharder.go`
  - `internal/ast/sharder_test.go`
* **Verification Criteria:**
  - Parse a 1,000-line source file; verify AST sharder returns targeted 45-line context slice with >95% reduction in token count while preserving all required type signatures.

---

#### TASK-1.3.4: Artifact-Driven Memory Store (`.sdlc/artifacts/{task_id}/`)
* **Objective:** Strictly decouple agent communication from chat history through standardized, version-controlled markdown artifacts matching the General AI SDLC.
* **Technical Scope:**
  - Implement artifact manager creating `.sdlc/artifacts/{task_id}/`:
    - `PRD.md`: Formal Product Requirements Document synthesized from feature request.
    - `ATDD_SUITE.md`: Executable acceptance test suite grounded in codebase AST and PRD.
    - `TECH_DOC_RFC.md`: Technical architecture, API contracts, DB migrations, and sequence diagrams.
    - `TASK_PLAN.md`: Sequenced atomic task breakdown with dependency ordering.
    - `UAT_PREPARATION.md`: Plain-language human verification manual with test credentials and click paths.
    - `EVIDENCE.md`: Cryptographic audit log linking videos, screenshots, commit hashes, and SHA-256 HMAC.
  - Implement schema validators for each artifact markdown structure.
  - Provide versioning and diffing utilities to track inter-agent updates and state transitions.
* **Deliverable Files:**
  - `internal/artifacts/manager.go`
  - `internal/artifacts/validator.go`
  - `internal/artifacts/manager_test.go`
* **Verification Criteria:**
  - Automated test verifying artifact creation, schema validation, and disk persistence under `.sdlc/artifacts/{task_id}/`.

---

#### TASK-1.3.5: Multi-Provider LLM Driver Interface (Claude, Antigravity, ChatGPT, OpenCode)
* **Objective:** Implement pluggable provider adapters normalizing chat completions, streaming chunks, tool/function call conversions, and token usage accounting across Anthropic, Google/Antigravity, OpenAI, and OpenCode/Local inference engines.
* **Technical Scope:**
  - Define `ProviderClient` Go interface: `Complete(ctx, req)`, `Stream(ctx, req, ch)`, `CountTokens(req)`.
  - Implement adapters:
    - `internal/llm/anthropic_driver.go` (Claude 3.5 Sonnet / 3.5 Haiku / Opus via Anthropic API).
    - `internal/llm/antigravity_driver.go` (Antigravity engine & Gemini 1.5 Pro / Flash).
    - `internal/llm/openai_driver.go` (GPT-4o, GPT-4o-mini, o1, o3 via OpenAI API).
    - `internal/llm/opencode_driver.go` (Ollama, vLLM, DeepSeek-Coder, LocalAI with configurable base URLs and OpenAI-compatible endpoint wrappers).
  - Implement token normalization layer mapping provider-specific usage headers to standard internal telemetry formats.
* **Deliverable Files:**
  - `internal/llm/driver.go`
  - `internal/llm/anthropic_driver.go`
  - `internal/llm/antigravity_driver.go`
  - `internal/llm/openai_driver.go`
  - `internal/llm/opencode_driver.go`
  - `internal/llm/driver_test.go`
* **Verification Criteria:**
  - Mock server integration test sending identical structured prompt to all 4 drivers; verify all drivers return normalized responses and correctly formatted tool calls.

---

#### TASK-1.3.6: Cascading Configuration Engine (System vs. Per-Project `.sdlc/config.yaml`)
* **Objective:** Build a hierarchical configuration resolver that automatically loads global system defaults and overlays per-project custom settings.
* **Technical Scope:**
  - Implement strict 4-tier precedence resolver:
    $$\text{Project Config } (\texttt{.sdlc/config.yaml} \text{ or } \texttt{.meta-orchestrator.yaml}) \succ \text{User DB Settings} \succ \text{System Config } (\texttt{~/.meta-orchestrator/config.yaml}) \succ \text{Env Vars}$$
  - Deep-merge configuration parser: allows projects to override specific tiers (e.g., locking Tier 2 to local `opencode/deepseek-coder-v2` at `http://localhost:11434`) while seamlessly inheriting global system defaults for Tier 1 and Tier 3.
  - Implement filesystem watcher (`fsnotify`) to dynamically reload project-level configuration upon file changes without restarting the daemon process.
* **Deliverable Files:**
  - `internal/config/cascading_resolver.go`
  - `internal/config/project_config.go`
  - `internal/config/cascading_resolver_test.go`
* **Verification Criteria:**
  - Unit test loading a workspace with `.sdlc/config.yaml`; assert project-level Tier 2 model override takes precedence over global default, while un-overridden Tier 1 falls back to global system settings.
