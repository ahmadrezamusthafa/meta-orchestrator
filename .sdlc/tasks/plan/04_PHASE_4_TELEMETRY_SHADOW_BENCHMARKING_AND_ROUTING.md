# Phase 4 Execution Plan: Telemetry, Shadow Benchmarking & Smart Routing

**Document ID:** PLAN-004  
**Phase:** 4 of 5 (Weeks 13–16)  
**Status:** Approved for Implementation  
**Primary Focus:** TPF/TTR Telemetry, Dynamic Router Weights, Shadow Benchmarks, Analytics Dashboard  
**PRD References:** Pillar 7, Section 9, Section 11 (Phase 4)  

---

## 1. Phase Overview & Objectives

Phase 4 operationalizes continuous self-optimizing intelligence across the Meta-Orchestrator:
1. **Granular Telemetry Accounting:** Capture real-time Token-Per-Feature (TPF) cost (prompt vs completion), Time-to-Resolution (TTR), container CPU/Memory spikes, and test iterations.
2. **Dynamic Router Feedback Loop:** Compute empirical pass-rate heuristics to autonomously adjust model tiering and method weights for recurring task categories.
3. **Shadow Benchmarking Engine:** Replay completed production tickets in background ephemeral workspaces across alternative models and methods (e.g., Claude 3.5 Sonnet vs. DeepSeek vs. GPT-4o-mini, or BMAD vs. ReAct) to identify cost-saving optimizations.
4. **Executive Analytics Dashboard:** Deliver full Vue 3 visualization of LLM leaderboards, method ROI, burn rates, and MTTR trends.

---

## 2. Work Breakdown Structure (WBS) & Tasks

```
Phase 4: Telemetry, Shadow Benchmarking & Smart Routing
├── Epic 4.1: Cost & Performance Telemetry Engine
│   ├── TASK-4.1.1: Granular Token-Per-Feature (TPF) Accounting Collector
│   ├── TASK-4.1.2: Time-to-Resolution (TTR) & Resource Metrics Monitor
│   └── TASK-4.1.3: Telemetry Time-Series Datastore & Aggregator
├── Epic 4.2: Dynamic Router Feedback Loop
│   ├── TASK-4.2.1: Historical Pass-Rate & Heuristic Scoring Engine
│   └── TASK-4.2.2: Dynamic Weight Calibrator for Model & Method Selection
├── Epic 4.3: Shadow Benchmarking Engine (Background A/B Testing)
│   ├── TASK-4.3.1: Ticket Replay Sandbox Daemon
│   ├── TASK-4.3.2: Multi-Model / Multi-Method Matrix Execution Harness
│   └── TASK-4.3.3: Empirical Cost-Performance Diff Generator & Best Methods Matrix Builder
└── Epic 4.4: Executive Analytics & Leaderboard Dashboard
    ├── TASK-4.4.1: Vue 3 Analytics View & Model/Method Benchmark Matrix
    ├── TASK-4.4.2: Token Burn Rate & Methodology ROI Chart Components
    └── TASK-4.4.3: MTTR & Test Stability Index Visualizer
```

---

## 3. Detailed Task Specifications

### Epic 4.1: Cost & Performance Telemetry Engine

#### TASK-4.1.1: Granular Token-Per-Feature (TPF) Accounting Collector
* **Objective:** Capture exact token consumption (prompt tokens, completion tokens, cached tokens) broken down by task, profile, model tier, and repository.
* **Technical Scope:**
  - Instrument LLM client wrappers to extract usage headers on every completion call.
  - Calculate dollar costs using real-time pricing table per provider.
  - Publish metric events `telemetry.tokens.consumed` to event bus.
* **Deliverable Files:**
  - `internal/telemetry/token_tracker.go`
  - `internal/telemetry/pricing.go`
  - `internal/telemetry/token_tracker_test.go`
* **Verification Criteria:**
  - Execute 10 mock LLM completions; verify recorded token count and dollar calculation precisely match provider billing formulas.

---

#### TASK-4.1.2: Time-to-Resolution (TTR) & Resource Metrics Monitor
* **Objective:** Measure total task duration, per-phase latency, Docker CPU/Memory consumption, and test execution cycles.
* **Technical Scope:**
  - Record microsecond-level timestamps for all state machine transitions.
  - Sample Docker stats API (`docker stats --no-stream`) during container builds and Playwright runs.
  - Detect resource spikes and test cycle iterations.
* **Deliverable Files:**
  - `internal/telemetry/ttr_collector.go`
  - `internal/telemetry/container_stats.go`
* **Verification Criteria:**
  - Run containerized test suite; assert CPU/memory peak metrics and phase duration are logged and attached to task metadata.

---

#### TASK-4.1.3: Telemetry Time-Series Datastore & Aggregator
* **Objective:** Store historical metrics in an optimized relational time-series store and expose aggregated query endpoints.
* **Technical Scope:**
  - Database schema: `telemetry_tokens`, `telemetry_runs`, `telemetry_methods`.
  - Provide aggregation queries: average TPF per task category, 7-day token burn rate, MTTR rolling averages.
  - REST endpoints under `/api/v1/telemetry/summary` and `/api/v1/telemetry/trends`.
* **Deliverable Files:**
  - `internal/telemetry/datastore.go`
  - `internal/telemetry/queries.go`
  - `internal/telemetry/api.go`
* **Verification Criteria:**
  - Ingest 1,000 synthetic metric rows; assert query endpoint returns aggregated metrics in `< 25ms`.

---

### Epic 4.2: Dynamic Router Feedback Loop

#### TASK-4.2.1: Historical Pass-Rate & Heuristic Scoring Engine
* **Objective:** Analyze task completion history to identify correlation between task scope characteristics, routing strategy (Best Practice vs. Custom Router), and model/method success rates.
* **Technical Scope:**
  - Compute First-Pass Verification Rate (FPVR) and Token-Per-Feature (TPF) per task category and routing strategy.
  - Calculate success probability matrix: `P(success | task_type, method, model_tier, router_strategy)`.
  - Perform comparative analysis: detect cases where user Custom Rules outperform Best Practice Heuristics (or vice-versa).
  - Flag task types where lower-cost models achieve $\ge 90\%$ pass rate without hitting frustration loops.
* **Deliverable Files:**
  - `internal/router/feedback/heuristics.go`
  - `internal/router/feedback/scoring.go`
  - `internal/router/feedback/heuristics_test.go`
* **Verification Criteria:**
  - Run heuristic evaluator against historical test dataset; assert engine correctly classifies routine tasks for tier downgrading and highlights high-performing custom rules.

---

#### TASK-4.2.2: Dynamic Weight Calibrator for Model & Method Selection
* **Objective:** Feed heuristic scores back into the Task Profiler to dynamically alter model selection weights and recommend promoting verified Custom Rules to system-wide Best Practices.
* **Technical Scope:**
  - Maintain dynamic routing weight table in Redis.
  - Automatically tune Best Practice routing policy: route routine tickets to Tier 2/Tier 3 models while reserving Tier 1 for novel or architectural epics.
  - Suggest "Promote Custom Rule to Best Practice" when a project-specific custom rule demonstrates superior FPVR and lower cost across 20+ runs.
  - Admin override capability to lock routing rules manually.
* **Deliverable Files:**
  - `internal/router/feedback/calibrator.go`
  - `internal/router/feedback/weights.go`
* **Verification Criteria:**
  - Verify router shifts subsequent identical CRUD ticket requests from Tier 1 to Tier 2 when weight threshold is met; generates rule promotion recommendation when criteria are satisfied.

---

### Epic 4.3: Shadow Benchmarking Engine (Background A/B Testing)

#### TASK-4.3.1: Ticket Replay Sandbox Daemon
* **Objective:** Replay completed production tasks in background ephemeral environments without interfering with production queues.
* **Technical Scope:**
  - Listen for task completion events; queue non-production shadow benchmark jobs.
  - Clone original ticket inputs, repository commits at intake time, and ATDD test assertions.
  - Provision background isolated Docker sandbox with lowest CPU priority (nice level).
* **Deliverable Files:**
  - `internal/shadow/daemon.go`
  - `internal/shadow/replay.go`
* **Verification Criteria:**
  - Complete a production task; verify shadow runner launches in background without raising host CPU above configured safety ceiling.

---

#### TASK-4.3.2: Multi-Model / Multi-Method Matrix Execution Harness
* **Objective:** Autonomously benchmark all execution methods (`BMAD`, `Supervisor`, `ReAct`, `Superpower`) and AI models across all 9 SDLC stages and 4 task complexity levels (`Low`, `Medium`, `High`, `System`) to determine the optimal execution method for every scenario.
* **Technical Scope:**
  - Multi-Dimensional Benchmark Configuration in `configs/shadow_matrix.json`:
    - SDLC Stages: Stages 1 through 9.
    - Complexity Tiers: `Low` (single-file / minor patch), `Medium` (modular service feature), `High` (multi-repo contract change), `System` (Docker/CI/database migration).
    - Methods: `BMAD` (multi-agent role specialization), `Supervisor` (cross-repo orchestrator), `ReAct` (fast iterative loops), `Superpower` (bare-metal environment control).
    - Models: Multi-provider pool (Claude 3.5 Sonnet/Haiku, GPT-4o/4o-mini, Antigravity, OpenCode/Local).
  - Background Matrix Execution Harness:
    - Replays representative task fixtures and real past tickets in ephemeral Docker sandboxes.
    - Captures First-Pass Verification Rate (FPVR), Token-Per-Feature (TPF) cost, Time-to-Resolution (TTR), and tool call efficiency.
    - Runs in headless low-priority background workers without starving interactive tasks.
* **Deliverable Files:**
  - `internal/shadow/matrix_runner.go`
  - `internal/shadow/stage_benchmarker.go`
  - `configs/shadow_matrix.json`
  - `internal/shadow/matrix_runner_test.go`
* **Verification Criteria:**
  - Execute matrix sweep across 2 stages and 2 complexity levels; assert runner completes all permutations, isolates run metrics per cell, and records comparative traces.

---

#### TASK-4.3.3: Empirical Cost-Performance Diff Generator & Best Methods Matrix Builder
* **Objective:** Analyze benchmark results using Pareto-efficiency scoring, generate comparative diff reports, and autonomously compile the active `configs/best_methods_matrix.json` utilized by the router.
* **Technical Scope:**
  - Scoring Heuristic: Evaluate winning method per `(stage, complexity)` tuple:
    $$\text{Score} = (w_1 \cdot \text{FPVR}) - (w_2 \cdot \text{NormalizedCost}) - (w_3 \cdot \text{NormalizedTTR})$$
  - Matrix Generator: Automatically output `configs/best_methods_matrix.json` specifying the optimal method (e.g. Planning $\to$ BMAD, Implementation Low $\to$ ReAct, Implementation High $\to$ Supervisor, System $\to$ Superpower) and target model tier.
  - Markdown Benchmark Diff Report: Generate `.sdlc/benchmarks/{timestamp}_stage_method_benchmark.md` containing formatted markdown tables, cost differentials, and FPVR confidence intervals.
  - Hot-Reload Event: Emit `router.matrix.updated` on event bus so the Router and Task Profiler immediately adopt updated optimal methods without daemon restart.
* **Deliverable Files:**
  - `internal/shadow/reporter.go`
  - `internal/shadow/matrix_builder.go`
  - `configs/best_methods_matrix.json`
  - `internal/shadow/templates/benchmark_report.md.tmpl`
  - `internal/shadow/matrix_builder_test.go`
* **Verification Criteria:**
  - Ingest synthetic multi-method benchmark telemetry; verify engine selects correct winning methods per cell, formats Markdown report, and writes valid `configs/best_methods_matrix.json`.

---

### Epic 4.4: Executive Analytics & Leaderboard Dashboard

#### TASK-4.4.1: Vue 3 Analytics View & Model/Method Benchmark Matrix
* **Objective:** Build dedicated Analytics page in the Vue 3 frontend displaying comparative model leaderboards and the empirical Stage $\times$ Complexity Method Benchmark Matrix.
* **Technical Scope:**
  - Implement `/analytics` route with Navigation Rail integration.
  - Model Leaderboard table: Model Name, Tier, Tasks Completed, First-Pass Pass Rate (%), Avg TPF, Avg TTR.
  - Stage $\times$ Complexity Method Matrix integration: displays interactive 36-cell grid showing winning method per SDLC stage and complexity strata with live telemetry updates.
  - Filterable by timeframe (24h, 7d, 30d, all-time) and target repository.
  - Real-time refresh triggered by WebSocket event `router.matrix.updated`.
* **Deliverable Files:**
  - `web/src/views/AnalyticsView.vue`
  - `web/src/components/analytics/ModelLeaderboard.vue`
  - `web/src/components/analytics/MethodBenchmarkMatrix.vue`
* **Verification Criteria:**
  - Renders both the Model Leaderboard and the Method Benchmark Matrix with sorting and timeframe filtering; values formatted according to UI Spec typography tokens (`text-sky-400 font-mono text-xs`).

---

#### TASK-4.4.2: Token Burn Rate & Methodology ROI Chart Components
* **Objective:** Render responsive time-series charts illustrating daily token expenditures and methodology ROI.
* **Technical Scope:**
  - Integrate SVG/Canvas charting (Chart.js or lightweight SVG paths).
  - Visual display of Token Burn over time (Prompt vs Completion tokens stacked).
  - Methodology comparison bar chart: BMAD vs Supervisor vs ReAct vs Superpower costs.
* **Deliverable Files:**
  - `web/src/components/analytics/TokenBurnChart.vue`
  - `web/src/components/analytics/MethodRoiChart.vue`
* **Verification Criteria:**
  - Charts resize responsively; tooltips display exact token numbers and dollar equivalencies on hover.

---

#### TASK-4.4.3: MTTR & Test Stability Index Visualizer
* **Objective:** Display Mean Time to Resolution and ATDD test suite stability metrics.
* **Technical Scope:**
  - Gauge component for First-Pass Verification Rate (target `> 80%`).
  - Average MTTR indicator card (`< 15 minutes` target).
  - Flakiness indicator highlighting repositories with high failure loop counts.
* **Deliverable Files:**
  - `web/src/components/analytics/MttrGauge.vue`
  - `web/src/components/analytics/StabilityIndex.vue`
* **Verification Criteria:**
  - Visual gauges animate cleanly on mount; colors reflect target thresholds (`emerald` for pass, `amber` for warning, `rose` for target breach).
