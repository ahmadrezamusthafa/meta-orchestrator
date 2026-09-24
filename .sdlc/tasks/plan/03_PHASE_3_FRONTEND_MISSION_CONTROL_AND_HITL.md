# Phase 3 Execution Plan: Frontend Mission Control & HITL Observability

**Document ID:** PLAN-003  
**Phase:** 3 of 5 (Weeks 9–12)  
**Status:** Approved for Implementation  
**Primary Focus:** Vue 3 Mission Control, Terminal Emulation, Virtualized Streams, Evidence Vault, HITL  
**PRD References:** Pillar 6, Section 5, Section 8, Section 11 (Phase 3)  
**UI Spec References:** Sections 1, 2, 3, 4 ([UI_SPEC_001.md](../../specs/UI_SPEC_001.md))  

---

## 1. Phase Overview & Objectives

Phase 3 delivers the complete developer observability and Human-in-the-Loop (HITL) frontend:
1. **Design System & App Shell:** Strict 8pt grid, dark-mode base (`slate-950`), responsive 64px/240px Navigation Rail, and Global Status Header with live token burn and daemon connection indicators.
2. **Mission Control (Kanban Dashboard):** 6-column workflow board (`Intake & Profiling`, `ATDD Test Writing`, `Write-Locked (Red Phase)`, `Implementation`, `Validation & Evidence Capture`, `Ready for Sign-Off`).
3. **Execution Workspace (Split-Pane):** 60/40 split with `@xterm/xterm` canvas terminal, virtualized thought stream with RAF debouncing, SVG workspace topology visualizer, and tabbed Evidence Media Vault.
4. **HITL Controls:** Sticky Frustration Alert banner, Live Context Injection dock, and One-Click Workspace Reset.

---

## 2. Work Breakdown Structure (WBS) & Tasks

```
Phase 3: Frontend Mission Control & HITL Observability
├── Epic 3.1: Frontend Shell, Design Tokens & State Architecture
│   ├── TASK-3.1.1: Vue 3 + Tailwind v3.4+ Setup & 8pt Token System
│   ├── TASK-3.1.2: Fixed Navigation Rail (64px Collapsed / 240px Expanded)
│   ├── TASK-3.1.3: Global Status Header & Real-time Daemon Telemetry Pill
│   └── TASK-3.1.4: Pinia State Stores & WebSocket Client with Reconnect Toast
├── Epic 3.2: Mission Control (Kanban Dashboard)
│   ├── TASK-3.2.1: Filter Toolbar (Search, Method, Repo, "New Task" Action)
│   ├── TASK-3.2.2: 8-Column General AI SDLC Kanban Board Layout & Column Containers
│   ├── TASK-3.2.3: `KanbanCard` Component & Visual State Indicators
│   ├── TASK-3.2.4: Kanban Column Empty States & Skeletons
│   └── TASK-3.2.5: `NewTaskModal` with Mid-Process Stage Range Selector & Artifact Ingestion
├── Epic 3.3: Active Task Detail & Execution Workspace View
│   ├── TASK-3.3.1: Resizable Split-Pane Workspace (60% Exec / 40% Control)
│   ├── TASK-3.3.2: `XtermTerminal` Component with RAF Streaming Buffer
│   ├── TASK-3.3.3: `ThoughtFeed` Virtualized Thought Stream Component
│   ├── TASK-3.3.4: `WorkspaceGraphNode` SVG Topology & Dependency Visualizer
│   └── TASK-3.3.5: Evidence Media Vault & Tabbed Markdown Viewer
├── Epic 3.4: Human-in-the-Loop (HITL) Controls & Recovery Matrix
│   ├── TASK-3.4.1: Sticky Frustration Alert Banner & Action Controls
│   ├── TASK-3.4.2: `ContextInput` Live Prompt Steering Dock
│   ├── TASK-3.4.3: One-Click Workspace Reset & Volume Purge Modal
│   └── TASK-3.4.4: Gate Approval Action Components (`BtnPrimary` / `BtnDestructive`)
├── Epic 3.5: Tool Hub & Dependency Lifecycle Manager View
│   ├── TASK-3.5.1: Tool Hub Catalog View (`/tools`) & `ToolLifecycleCard`
│   ├── TASK-3.5.2: Step-by-Step Guided `InstallWizardModal` with Live Mini-Console
│   ├── TASK-3.5.3: Version Management, 1-Click Update & Rollback Controller
│   └── TASK-3.5.4: "Auto-Resolve Best Fit Matrix" Heuristic Evaluator & Action
└── Epic 3.6: AI Provider, Router Strategy & Project Settings UI
    ├── TASK-3.6.1: Provider Management View (`/settings/providers`) & `ProviderConfigCard`
    ├── TASK-3.6.2: Cascading Project Settings Drawer & `ProjectOverrideBadge`
    ├── TASK-3.6.3: Router Strategy Configurator & `RoutingExplainerPill`
    ├── TASK-3.6.4: Stage & Complexity Benchmark Matrix Table View
    ├── TASK-3.6.5: Custom SDLC Workflow Builder & Manager View (`/settings/workflows`)
    └── TASK-3.6.6: Multi-Source Registries & Hooks Manager View (`/settings/registries`)
```

---

## 3. Detailed Task Specifications

### Epic 3.1: Frontend Shell, Design Tokens & State Architecture

#### TASK-3.1.1: Vue 3 + Tailwind v3.4+ Setup & 8pt Token System
* **Objective:** Scaffold Vue 3 (Vite, TypeScript, `<script setup>`) frontend with strict 8pt grid tokens, dark-mode base (`slate-950`), and typography scale.
* **Technical Scope:**
  - Configure `tailwind.config.ts` with custom 8pt spacing tokens (`space-0.5` through `space-16`), slate dark palette, and typography scale (Inter and JetBrains Mono).
  - Install dependencies: `pinia`, `vue-router`, `@xterm/xterm`, `@xterm/addon-fit`, `lucide-vue-next`, `radix-vue`.
  - Establish base CSS with dark background (`bg-slate-950`) and custom scrollbars.
* **Deliverable Files:**
  - `web/package.json`
  - `web/vite.config.ts`
  - `web/tailwind.config.ts`
  - `web/src/assets/main.css`
* **Verification Criteria:**
  - Vite dev server starts without error; typography scale and 8pt margin/padding tokens render accurately across demo elements.

---

#### TASK-3.1.2: Fixed Navigation Rail (64px Collapsed / 240px Expanded)
* **Objective:** Build the fixed left navigation rail supporting collapsed icon-only mode and expanded menu view.
* **Technical Scope:**
  - Implement fixed left rail: `w-16` (64px) expanding to `w-60` (240px) on toggle, `z-40`, `bg-slate-900 border-r border-slate-800`.
  - Top brand glyph (`32x32px`), nav icons (Mission Control, Workspaces, Registries, Telemetry, Settings), bottom system health indicator.
  - Smooth CSS width transition (`duration-200 ease-in-out`).
* **Deliverable Files:**
  - `web/src/components/layout/NavigationRail.vue`
  - `web/src/components/layout/NavRailItem.vue`
* **Verification Criteria:**
  - Clicking collapse toggle smoothly animates between 64px and 240px; active routes highlight with emerald border indicator.

---

#### TASK-3.1.3: Global Status Header & Real-time Daemon Telemetry Pill
* **Objective:** Implement the fixed top header (`h-12`, `z-30`) displaying active workspace selector, daemon status pill, token counter, and HITL alert.
* **Technical Scope:**
  - Position: `top-0 left-16 w-[calc(100vw-64px)] h-12 bg-slate-900/90 backdrop-blur border-b border-slate-800 px-4 flex items-center justify-between`.
  - Components:
    - Active Workspace selector dropdown (`w-60`).
    - Daemon Connection Status pill with pulsing green indicator dot.
    - Global Token Burn Counter (`font-mono text-xs text-sky-400`).
    - HITL Blocked Task indicator badge (conditional red pulsing badge when any task is `BLOCKED_FRUSTRATION`).
* **Deliverable Files:**
  - `web/src/components/layout/GlobalHeader.vue`
  - `web/src/components/common/DaemonStatusPill.vue`
* **Verification Criteria:**
  - Simulated disconnect updates pill to amber/red; setting task state to `BLOCKED_FRUSTRATION` renders pulsing red indicator badge in header.

---

#### TASK-3.1.4: Pinia State Stores & WebSocket Client with Reconnect Toast
* **Objective:** Implement reactive state management and high-frequency WebSocket client with auto-reconnection and floating notification toast.
* **Technical Scope:**
  - Create Pinia stores: `useTaskStore`, `useTerminalStore`, `useWorkspaceStore`, `useTelemetryStore`.
  - Implement WebSocket client handling reconnect with exponential backoff.
  - Implement top-center floating toast: `Daemon WebSocket disconnected. Reconnecting in 3s... (State persisted in Redis)` with pulsing yellow dot (`amber-950` style).
* **Deliverable Files:**
  - `web/src/stores/tasks.ts`
  - `web/src/stores/terminal.ts`
  - `web/src/services/websocket.ts`
  - `web/src/components/common/WsReconnectToast.vue`
* **Verification Criteria:**
  - Sever WebSocket connection; assert reconnection attempts fire every 3s and toast is visible; restore connection and assert toast dismisses automatically.

---

### Epic 3.2: Mission Control (Kanban Dashboard)

#### TASK-3.2.1: Filter Toolbar (Search, Method, Repo, "New Task" Action)
* **Objective:** Implement top sub-header (`h-14`, `px-6`, `py-3`) containing search input, method filter, repo filter, and "New Task" button.
* **Technical Scope:**
  - Search input (`w-72`) with debounced text filtering.
  - Method Filter dropdown (`w-36`): `All Methods`, `BMAD`, `Supervisor`, `ReAct`, `Superpower`.
  - Repo Filter dropdown (`w-48`): multi-repo selector.
  - Right-aligned "New Task" button using `BtnPrimary` (`bg-emerald-600 text-white font-semibold`).
* **Deliverable Files:**
  - `web/src/components/kanban/KanbanToolbar.vue`
  - `web/src/components/common/BtnPrimary.vue`
* **Verification Criteria:**
  - Filter selections filter the Kanban store reactive items in real time.

---

#### TASK-3.2.2: 8-Column General AI SDLC Kanban Board Layout & Column Containers
* **Objective:** Render the 8-column General AI SDLC lifecycle board with horizontal scrolling and fixed column widths.
* **Technical Scope:**
  - Board Container: flex row, `gap-4`, `p-6`, `h-[calc(100%-56px)]`, `overflow-x-auto`.
  - 8 fixed columns (each `w-[320px]`, `min-w-[320px]`, `h-full`, `bg-slate-900/60 rounded-xl border border-slate-800 flex flex-col`):
    1. `PRD & Dynamic Repo Discovery`
    2. `ATDD Creation (Red Phase)`
    3. `Tech Doc / RFC Review (Gate)`
    4. `Task Breakdown & Planning`
    5. `Implementation (Write-Unlocked)`
    6. `Automation & E2E Validation`
    7. `Manual & UAT Verification (Evidence)`
    8. `Ready for Sign-Off & Merge`
  - Column header: stage title, count badge, and phase icon.
* **Deliverable Files:**
  - `web/src/views/MissionControlView.vue`
  - `web/src/components/kanban/KanbanColumn.vue`
* **Verification Criteria:**
  - Renders 8 columns smoothly; handles horizontal scroll on smaller viewports without breaking global viewport bounds.

---

#### TASK-3.2.3: `KanbanCard` Component & Visual State Indicators
* **Objective:** Build interactive Kanban task card with colored left borders, token metrics, and drag/hover effects.
* **Technical Scope:**
  - Card layout: `p-3.5`, `rounded-lg`, `border border-slate-800`, `bg-slate-900/90`.
  - Left border indicator:
    - `emerald-500` (4px solid) for running implementation.
    - `amber-500` for ATDD write-lock active.
    - `rose-500` for Frustrated/Blocked.
  - Content: Task ID (`font-mono text-xs text-slate-400`), Title (`text-sm font-medium line-clamp-2`), Method badge, Repo count, Token Burn, Elapsed Time.
  - Hover elevation (`-translate-y-0.5`) and click navigation to Task Detail View.
* **Deliverable Files:**
  - `web/src/components/kanban/KanbanCard.vue`
  - `web/src/components/common/PhaseStatusBadge.vue`
* **Verification Criteria:**
  - Card displays all required metadata; clicking card triggers navigation to `/tasks/{task_id}`; blocked cards pulse subtly (`animate-pulse-subtle`).

---

#### TASK-3.2.4: Kanban Column Empty States & Skeletons
* **Objective:** Implement dashed empty column placeholders and 156px card loading skeletons.
* **Technical Scope:**
  - Empty state: `h-48 border border-dashed border-slate-800 rounded-lg flex flex-col items-center justify-center p-4` with Lucide `Inbox` icon (`24x24`, `text-slate-600`), primary text `No active tasks`, and subtext `Items routed to this phase will appear automatically`.
  - Loading skeleton: `h-[156px] rounded-lg border border-slate-800/80 bg-slate-900/50 p-3.5 space-y-3` with animated pulse bars matching card geometry.
* **Deliverable Files:**
  - `web/src/components/kanban/KanbanColumnEmpty.vue`
  - `web/src/components/kanban/KanbanCardSkeleton.vue`
* **Verification Criteria:**
  - Empty columns render placeholder cleanly; skeleton mode prevents layout shift during initial data hydration.

---

#### TASK-3.2.5: `NewTaskModal` with Mid-Process Stage Range Selector & Artifact Ingestion
* **Objective:** Build an interactive modal dialog for launching tasks with either full end-to-end SDLC pipeline execution or sliced mid-process execution (e.g. executing strictly from Stage 7 Task Implementation through Stage 8 E2E Validation with Playwright `.mp4` video output).
* **Technical Scope:**
  - Modal container: centered, dark backdrop blur (`bg-slate-950/80`), `max-w-2xl`, styled to 8pt design tokens (`p-6 bg-slate-900 border border-slate-800 rounded-xl`).
  - Inputs:
    - Task Title (`text-sm bg-slate-950 border-slate-700`) and Description / Ticket Body.
    - Multi-Repo Target Selector (auto-detect toggle or explicit repository checkboxes).
    - Execution Scope Radio: `Full SDLC Pipeline` (default) vs `Partial Stage Slice (Mid-Process)`.
    - Partial Stage Slice Controls (conditionally displayed):
      - `Start Stage` Dropdown: Stage 1 (PRD Creation) through Stage 8 (E2E Validation).
      - `Halt Stage` Dropdown: Stage 1 through Stage 9 (Sign-Off & Merge).
      - `Artifact & Branch Ingestion Dropzone`:
        - Input for existing Git Branch name (e.g. `feat/order-checkout-v2`).
        - Drag-and-drop / file selector for pre-existing `TASK_PLAN.md` or `TECH_DOC_RFC.md`.
      - Rich Media Toggle: Checkbox `Record Playwright .mp4 test execution video` (default: checked).
    - Method & Provider Selection:
      - Radio toggle: `Best Practice Heuristic Router` (Auto) vs `Custom Router Rules` vs `Manual Override` (BMAD, ReAct, Supervisor, Superpower).
  - API Submission: dispatches `POST /api/v1/tasks` with structured `execution_slice` payload.
* **Deliverable Files:**
  - `web/src/components/kanban/NewTaskModal.vue`
  - `web/src/components/kanban/StageRangeSelector.vue`
  - `web/src/components/kanban/ArtifactUploadDropzone.vue`
* **Verification Criteria:**
  - Select "Partial Stage Slice", select Start Stage = Stage 7 and Halt Stage = Stage 8, provide branch name, ensure video recording toggle is on; click "Launch Task"; assert POST payload accurately contains all slice parameters and task card appears directly in the Implementation column.

---

### Epic 3.3: Active Task Detail & Execution Workspace View

#### TASK-3.3.1: Resizable Split-Pane Workspace (60% Exec / 40% Control)
* **Objective:** Implement the two-column split-pane layout with resizable divider and zero layout shifts.
* **Technical Scope:**
  - Container: `h-full flex overflow-hidden`.
  - Left Execution Pane: `flex-1` (default 60% width, min-width `500px`), full height.
  - Resizable horizontal divider bar (`w-1.5 bg-slate-800 hover:bg-emerald-500 cursor-col-resize transition-colors`).
  - Right Control & Artifact Pane: fixed 40% width (min `420px`, max `640px`), full height, vertical scroll.
* **Deliverable Files:**
  - `web/src/views/TaskDetailView.vue`
  - `web/src/components/layout/SplitPane.vue`
* **Verification Criteria:**
  - Dragging divider resizes panes smoothly; respects min/max width constraints; persists split ratio to `localStorage`.

---

#### TASK-3.3.2: `XtermTerminal` Component with RAF Streaming Buffer
* **Objective:** Render raw Docker stdout/stderr container logs via `@xterm/xterm` with 60fps `requestAnimationFrame` debouncing.
* **Technical Scope:**
  - Initialize `Terminal` instance with `FitAddon` and JetBrains Mono font (`12px`, black background).
  - Buffer incoming WebSocket `agent.terminal` chunks in an internal ring-buffer and flush via `window.requestAnimationFrame`.
  - Utility toolbar: "Autoscroll" toggle switch, "Clear" button, "Download Log" button, and connection state pill (`Connected` / `Buffering` / `Terminated`).
  - Scrollback pause badge when user scrolls upward.
* **Deliverable Files:**
  - `web/src/components/terminal/XtermTerminal.vue`
  - `web/src/composables/useTerminalBuffer.ts`
* **Verification Criteria:**
  - Stream 5,000 log lines in 2 seconds; verify browser UI remains responsive at 60fps without freezing; autoscroll pauses when scrolled up.

---

#### TASK-3.3.3: `ThoughtFeed` Virtualized Thought Stream Component
* **Objective:** Render agent reasoning and tool invocations in a virtualized timeline without DOM overload.
* **Technical Scope:**
  - Row-based timeline with thought bubbles: profile glyph (`16x16px`), profile name (`font-semibold text-xs text-slate-200`), model tag (`text-[10px] font-mono text-slate-400`), timestamp.
  - Collapsible markdown reasoning body.
  - Embedded tool invocation blocks (`bg-slate-950 p-2 rounded text-[11px] font-mono text-emerald-400 border border-slate-800/80`) with expandable JSON arguments/outputs.
* **Deliverable Files:**
  - `web/src/components/thought/ThoughtFeed.vue`
  - `web/src/components/thought/ThoughtBubble.vue`
* **Verification Criteria:**
  - Render 500 thought nodes; virtualized list renders only visible elements; expanding tool blocks reveals full payload without layout distortion.

---

#### TASK-3.3.4: `WorkspaceGraphNode` SVG Topology & Dependency Visualizer
* **Objective:** Display dynamically cloned repositories, Docker mounts, and symlinks in an interactive SVG/canvas graph.
* **Technical Scope:**
  - SVG node size: `180px x 64px` with repo name, branch name, and symlink indicator badge (`LNK` green dot or `ERR` red dot).
  - Edge connectors: solid `emerald-500/70` with animated SVG dash offset indicating active IO.
  - Conflicted symlink edges: dashed `rose-500` with pulsing alert icon.
  - Interactive click selection of nodes to view container stats.
* **Deliverable Files:**
  - `web/src/components/workspace/WorkspaceGraph.vue`
  - `web/src/components/workspace/WorkspaceGraphNode.vue`
* **Verification Criteria:**
  - Renders 3 connected repo nodes; animated dashed lines show IO; clicking node displays mount details.

---

#### TASK-3.3.5: Evidence Media Vault & Tabbed Markdown Viewer
* **Objective:** Provide in-browser `.mp4` video player, screenshot diff gallery, and comprehensive markdown artifact viewer for all General AI SDLC documents.
* **Technical Scope:**
  - `VideoPlayerVault`: 16:9 HTML5 video player with custom controls (Play/Pause, time scrubber, `1x/1.5x/2x` speed selector, fullscreen). Fail state: centered message `No MP4 recorded for this test execution run`.
  - Side-by-side screenshot diff gallery with zoom modal for visual review.
  - Tabbed Markdown viewer with tabs:
    - `PRD.md` (Product Requirements Document)
    - `ATDD_SUITE.md` (Acceptance Test Suite grounded in AST)
    - `TECH_DOC_RFC.md` (Technical architecture, schemas, and migrations)
    - `TASK_PLAN.md` (Sequenced task breakdown)
    - `UAT_PREPARATION.md` (Human verification manual)
    - `EVIDENCE.md` (Cryptographically signed audit manifest)
  - 8-line skeleton loader preventing layout shifts during tab swaps.
* **Deliverable Files:**
  - `web/src/components/evidence/VideoPlayerVault.vue`
  - `web/src/components/evidence/ScreenshotDiff.vue`
  - `web/src/components/evidence/ArtifactMarkdownViewer.vue`
* **Verification Criteria:**
  - Video scrub and speed change operate smoothly; swapping artifact tabs preserves scroll position and displays skeleton loader during fetch.

---

### Epic 3.4: Human-in-the-Loop (HITL) Controls & Recovery Matrix

#### TASK-3.4.1: Sticky Frustration Alert Banner & Action Controls
* **Objective:** Render high-visibility alert banner when agent exceeds frustration threshold (3 failed attempts).
* **Technical Scope:**
  - Top sticky banner: `bg-rose-950 border-l-4 border-rose-600 p-4 shadow-xl flex items-start gap-3`.
  - Icon: Lucide `AlertOctagon` (`20x20px`, `text-rose-400`).
  - Header: `Execution Halted: Frustration Threshold Exceeded (Attempt 3/3 Failed)`.
  - Copy: failing Playwright assertion message.
  - Action buttons: `BtnPrimary` ("Inject Guidance & Retry") and `BtnDestructive` ("Purge Volume & Reset Workspace").
* **Deliverable Files:**
  - `web/src/components/hitl/FrustrationBanner.vue`
* **Verification Criteria:**
  - Triggering `BLOCKED_FRUSTRATION` displays sticky banner immediately; clicking "Inject Guidance" focuses the context dock.

---

#### TASK-3.4.2: `ContextInput` Live Prompt Steering Dock
* **Objective:** Implement sticky bottom dock allowing human developers to inject corrective instructions into active agent context.
* **Technical Scope:**
  - Height: `80px`, `p-2.5`, `bg-slate-900 border-t border-slate-800`.
  - Multi-line auto-expanding textarea (`h-12 max-h-24 text-xs font-sans bg-slate-950 border border-slate-700 rounded p-2 text-slate-200`).
  - Action buttons: "Send Instruction" with inline loading spinner when submitting, and "Reset Volume" icon button.
  - Send message to daemon via WebSocket `hitl.inject` event.
* **Deliverable Files:**
  - `web/src/components/hitl/ContextInput.vue`
* **Verification Criteria:**
  - Type message and hit Cmd+Enter; assert payload sends to daemon over WS and input clears; disabled state applies when task is not running.

---

#### TASK-3.4.3: One-Click Workspace Reset & Volume Purge Modal
* **Objective:** Implement instant workspace recovery modal to purge corrupted Docker volumes, restore git tree, and re-execute.
* **Technical Scope:**
  - Confirmation dialog built with Radix Vue.
  - Uses `BtnDestructive` (`bg-rose-950/80 text-rose-300 border border-rose-800/80`).
  - Invokes backend reset endpoint: purges task volumes, runs `git clean -fd && git reset --hard`, and restarts current phase.
* **Deliverable Files:**
  - `web/src/components/hitl/ResetWorkspaceModal.vue`
  - `web/src/components/common/BtnDestructive.vue`
* **Verification Criteria:**
  - Confirming reset dispatches API call, closes modal, displays toast, and resets terminal state.

---

#### TASK-3.4.4: Gate Approval Action Components (`BtnPrimary` / `BtnDestructive`)
* **Objective:** Implement standardized action buttons conforming to UI Spec 2.1 and 2.2 with all interactive states.
* **Technical Scope:**
  - Standard button sizes (`h-9`, `px-4`, `py-2`, `text-xs font-semibold uppercase tracking-wider`).
  - States: Default, Hover, Active, Disabled, Focused with custom ring offsets.
  - Integrated with SDLC lifecycle approval gates: `AWAITING_CODE_GEN` (Authorize code gen) and `READY_FOR_SIGNOFF` (Authorize multi-repo PR merge).
* **Deliverable Files:**
  - `web/src/components/common/BtnPrimary.vue`
  - `web/src/components/common/BtnDestructive.vue`
  - `web/src/components/hitl/GateApprovalBar.vue`
* **Verification Criteria:**
  - Buttons render exact Tailwind classes from UI spec 2.1 & 2.2; gate actions disable during pending requests.

---

### Epic 3.5: Tool Hub & Dependency Lifecycle Manager View

#### TASK-3.5.1: Tool Hub Catalog View (`/tools`) & `ToolLifecycleCard` Component
* **Objective:** Render the dedicated `/tools` management view with high-density catalog grid, category tabs, and interactive `ToolLifecycleCard` items.
* **Technical Scope:**
  - Route: `/tools` integrated into Navigation Rail.
  - Sub-header: Search input (`w-72`), category filter tabs (`All`, `Methodologies`, `Parsers`, `Runtimes`, `UI Kits`), and "Auto-Resolve Best Fit Matrix" action button.
  - `ToolLifecycleCard` layout matching UI Spec 2.10: tool glyph, name, version badge, health dot (`emerald-500` healthy, `amber-500` update available, `rose-500` uninstalled/error), best-fit tag, action cluster (`Guided Install`, `Update to vX.Y`, `Rollback`).
* **Deliverable Files:**
  - `web/src/views/ToolHubView.vue`
  - `web/src/components/tools/ToolLifecycleCard.vue`
* **Verification Criteria:**
  - Catalog renders all core tools (BMAD, Superpower, Playwright, Tree-sitter, xterm.js); category filtering and status badges update dynamically based on daemon store state.

---

#### TASK-3.5.2: Step-by-Step Guided `InstallWizardModal` with Live Mini-Console
* **Objective:** Build interactive 4-step modal wizard guiding users through installation with live streaming build logs.
* **Technical Scope:**
  - Dialog structure matching UI Spec 2.10: `max-w-xl`, dark background (`bg-slate-900 border border-slate-800 rounded-xl`).
  - 4-step visual stepper: `1. Pre-flight Checks` → `2. Fetch & Build` → `3. Self-Test / Diagnostics` → `4. Ready`.
  - Embedded 120px tall terminal console rendering live output from WebSocket event `tool.install.progress`.
  - Next/Cancel controls and error recovery suggestions on failed steps.
* **Deliverable Files:**
  - `web/src/components/tools/InstallWizardModal.vue`
  - `web/src/components/tools/InstallStepper.vue`
* **Verification Criteria:**
  - Launching guided install transitions through steps 1 to 4; mini-console renders streamed stdout logs without UI stutter.

---

#### TASK-3.5.3: Version Management, 1-Click Update & Rollback Controller
* **Objective:** Allow operators to update tools to latest releases, roll back to prior snapshots, or pin specific versions from a dropdown menu.
* **Technical Scope:**
  - Version selector dropdown showing `Latest (vX.Y.Z)`, `Best Fit (Recommended)`, and local rollback snapshots.
  - 1-Click Update button triggering atomic update with progress indicator.
  - Rollback action opening quick confirmation modal displaying version changelog diff and executing instant symlink swap via daemon API.
* **Deliverable Files:**
  - `web/src/components/tools/VersionSelectorDropdown.vue`
  - `web/src/components/tools/RollbackConfirmModal.vue`
* **Verification Criteria:**
  - Selecting rollback to v1 initiates API call, confirms instant version reversion, and updates card status badge to healthy.

---

#### TASK-3.5.4: "Auto-Resolve Best Fit Matrix" Heuristic Evaluator & Action
* **Objective:** Provide automated system evaluation that detects the optimal, conflict-free version matrix across all tools for the host machine.
* **Technical Scope:**
  - Invokes backend `/api/v1/tools/resolve-matrix` endpoint.
  - Evaluates host OS, architecture, installed Node/Go versions, and cross-tool lockfiles.
  - Displays comparison modal showing current vs recommended best-fit versions with 1-click "Apply Recommended Matrix" batch update.
* **Deliverable Files:**
  - `web/src/components/tools/BestFitMatrixModal.vue`
  - `web/src/components/tools/BestFitDiffTable.vue`
* **Verification Criteria:**
  - Clicking "Auto-Resolve Best Fit Matrix" opens recommendation modal; applying recommendations triggers batch background updates with progress toasts.

---

### Epic 3.6: AI Provider & Project Settings UI

#### TASK-3.6.1: Provider Management View (`/settings/providers`) & `ProviderConfigCard`
* **Objective:** Render comprehensive provider management view allowing users to configure, test, and map models for Anthropic (Claude), Google/Antigravity, OpenAI (ChatGPT), OpenCode/Local inference, and custom endpoints.
* **Technical Scope:**
  - Route: `/settings/providers` accessible from Navigation Rail settings.
  - Sub-header: Context switcher (Global System Defaults vs. Current Project), "Test All Connections" button.
  - `ProviderConfigCard` items matching UI Spec 2.11: logo, provider name, status toggle, latency ping badge (`142ms font-mono text-[10px] text-emerald-400`), masked API key input, base URL input (for OpenCode / local vLLM / custom proxies), default model dropdown, and "Test Connection" button.
  - Tier Model Matrix Mapper: interactive table mapping Tier 1 (Reasoning), Tier 2 (Code Gen), and Tier 3 (Log Parsing) to specific provider + model combinations.
* **Deliverable Files:**
  - `web/src/views/ProviderSettingsView.vue`
  - `web/src/components/providers/ProviderConfigCard.vue`
  - `web/src/components/providers/TierModelMatrix.vue`
* **Verification Criteria:**
  - Clicking "Test Connection" triggers API latency ping and updates badge; saving persists credentials and updates Tier mapping store.

---

#### TASK-3.6.2: Cascading Project Settings Drawer & `ProjectOverrideBadge`
* **Objective:** Visualize and manage per-project custom AI provider settings with clear inheritance indicators.
* **Technical Scope:**
  - Implement `ProjectOverrideBadge` matching UI Spec 2.11 (`System Inherited (Global)` in slate vs `Project Override (.sdlc/config.yaml)` in emerald).
  - Slide-over drawer accessible from Mission Control and Workspace view to view/edit active project's `.sdlc/config.yaml`.
  - Form/YAML dual-editor with 1-click "Eject to Custom Project Config" (creates `.sdlc/config.yaml` from global settings) and "Reset to System Defaults" (deletes local override) actions.
* **Deliverable Files:**
  - `web/src/components/providers/ProjectSettingsDrawer.vue`
  - `web/src/components/common/ProjectOverrideBadge.vue`
* **Verification Criteria:**
  - Switching workspaces updates badge to reflect whether project has `.sdlc/config.yaml`; editing and saving writes back to `.sdlc/config.yaml` on disk.

---

#### TASK-3.6.3: Router Strategy Configurator & `RoutingExplainerPill`
* **Objective:** Build the management interface for selecting between Best Practice Heuristic Routing and Custom Rule-Based Routing, authoring custom rules, and visualizing execution rationale on task cards.
* **Technical Scope:**
  - Route: Configuration card in `/settings/providers` (UI Spec 1.5 Section D).
  - Strategy Toggle: Radio button switching between `Best Practice Heuristics` (built-in task complexity & stage mapping) and `Custom Rule-Based Router`.
  - Custom Rule Builder Modal:
    - Rule priority ordering via drag-and-drop.
    - Match conditions: Repository regex, task label/tag, SDLC Stage (Stages 1–9), Task Complexity (`Low`, `Medium`, `High`, `System`).
    - Action assignment: Chosen Method (`BMAD`, `ReAct`, `Supervisor`, `Superpower`), AI Provider, Model ID, Max Token Budget.
  - `RoutingExplainerPill` Component (UI Spec 2.12):
    - Display on `KanbanCard` and `TaskDetailView` header showing routing source badge (`[BP]` or `[RULE]`).
    - Click/Hover popover detailing the exact evaluation path (e.g. "Rule #2 matched: Stage=Implementation & Complexity=High -> Assigned Supervisor method using Claude 3.5 Sonnet").
* **Deliverable Files:**
  - `web/src/components/router/RouterConfigurator.vue`
  - `web/src/components/router/RuleBuilderModal.vue`
  - `web/src/components/common/RoutingExplainerPill.vue`
* **Verification Criteria:**
  - Add a custom rule matching Stage 7; verify rule saves to daemon; verify `RoutingExplainerPill` renders on matching task cards with popover showing rule evaluation details.

---

#### TASK-3.6.4: Stage & Complexity Method Benchmark Matrix Table View
* **Objective:** Render an interactive matrix visualization of the empirical benchmark results pairing every SDLC Stage and Task Complexity level with its optimal method.
* **Technical Scope:**
  - Route: Section E in `/settings/providers` and integrated in `/analytics` view.
  - Table Matrix Layout:
    - 9 Rows: SDLC Stages (`1. PRD`, `2. Dynamic Repo Discovery`, `3. ATDD Red Phase`, `4. Tech Doc / RFC`, `5. Task Breakdown`, `6. Write-Locked Red Verification`, `7. Task Implementation`, `8. Automation & E2E Validation`, `9. Sign-Off & Evidence`).
    - 4 Columns: Complexity Strata (`Low`, `Medium`, `High`, `System`).
  - Cell Data Display:
    - Winning Method badge (`BMAD`, `ReAct`, `Supervisor`, `Superpower`) with method color code.
    - First-Pass Verification Rate (FPVR %) badge (e.g. `96%`).
    - Average Token Cost and Latency metrics.
  - Interactive Actions:
    - Click cell to view historical benchmark runs comparing all 4 methods.
    - "Run Shadow Benchmark Sweep" button to trigger background A/B testing across all combinations.
    - "Sync to configs/best_methods_matrix.json" 1-click update.
* **Deliverable Files:**
  - `web/src/components/benchmark/StageComplexityMatrix.vue`
  - `web/src/components/benchmark/BenchmarkCellDetailModal.vue`
* **Verification Criteria:**
  - Matrix table displays all 36 stage/complexity permutations; clicking a cell opens comparison modal displaying execution times and token costs; exporting writes back to configuration.

---

#### TASK-3.6.5: Custom SDLC Workflow Builder & Manager View (`/settings/workflows`)
* **Objective:** Build a visual pipeline designer and catalog manager empowering users to author, customize, test, and export bespoke SDLC lifecycle workflows (e.g. 3-stage Hotfix, 4-stage Microservice API).
* **Technical Scope:**
  - Route: `/settings/workflows` with Navigation Rail shortcut.
  - Workflow Catalog:
    - Lists active workflows with active project indicator, stage counts, and gate types.
    - 1-click "Clone as Custom", "Set as Project Default", and "Export to .sdlc/workflow.yaml" actions.
  - Visual Stage Sequence Builder:
    - Node-based drag-and-drop linear/branching stage editor.
    - Stage Properties Panel:
      - Stage Name and ID.
      - Execution Method Binding (`BMAD`, `Supervisor`, `ReAct`, `Superpower`, or `Custom`).
      - Allowed Persona Roles (`Architect`, `Developer`, `QA`, `DevOps`).
      - Write-Locking Rule: toggle whether source code remains write-locked until Red Phase passes.
      - Required Input and Output Artifacts (`*.md` schemas, `.mp4` video, test suites).
      - Gate Type: Automated Verification vs. Human Approval Gate (`AWAITING_APPROVAL`).
  - Live Kanban Preview: interactive widget rendering the exact Kanban columns that will be projected for the designed custom workflow.
* **Deliverable Files:**
  - `web/src/views/WorkflowSettingsView.vue`
  - `web/src/components/workflows/WorkflowCatalog.vue`
  - `web/src/components/workflows/WorkflowStageBuilder.vue`
  - `web/src/components/workflows/StagePropertiesDrawer.vue`
  - `web/src/components/workflows/KanbanProjectionPreview.vue`
* **Verification Criteria:**
  - Design a 3-stage hotfix workflow; verify stage properties persist to YAML; verify preview renders 3 matching Kanban columns; export to `.sdlc/workflow.yaml` writes clean valid YAML file.

---

#### TASK-3.6.6: Multi-Source Registries & Hooks Manager View (`/settings/registries`)
* **Objective:** Deliver a unified control center for discovering, managing, and inspecting Skills, Prompt Templates, and Lifecycle Hooks across Remote Git repositories, Local project paths, and System global directories.
* **Technical Scope:**
  - Route: `/settings/registries`.
  - Tab 1: Skills Hub:
    - Multi-Source Tree: Remote Repositories (with "Add Remote Git Repo" modal, branch tag, and auto-sync status), Project Local (`.sdlc/skills/`), and System (`~/.config/meta-orchestrator/skills/`).
    - Tool Card: Displays JSON-Schema parameters, container isolation level, and "Run Test Invocation" button with output drawer.
  - Tab 2: Prompt Templates:
    - Browser with cascading inheritance indicators (highlighting whether a prompt is inherited from System or overridden by `.sdlc/prompts/`).
    - Integrated template editor with live slot-filling preview (`{{system_architecture}}`, `{{failing_test_traces}}`).
  - Tab 3: Lifecycle Hooks Engine:
    - Visual Event Matrix: mapping hooks to SDLC lifecycle triggers (`pre-stage`, `post-stage`, `on-failure`, `on-gate`, `pre-commit`, `post-commit`).
    - Hook Editor Drawer: configure execution type (`Shell Script`, `Docker Container`, `Webhook POST`), path/URL, timeout, retry count, and failure policy (`Block Pipeline` vs. `Warn & Continue`).
* **Deliverable Files:**
  - `web/src/views/RegistriesSettingsView.vue`
  - `web/src/components/registries/SkillsHubTab.vue`
  - `web/src/components/registries/AddRemoteSkillModal.vue`
  - `web/src/components/registries/PromptTemplatesTab.vue`
  - `web/src/components/registries/LifecycleHooksTab.vue`
  - `web/src/components/registries/HookEditorDrawer.vue`
* **Verification Criteria:**
  - Add a remote Git skill repository; assert repository is listed and syncs; inspect a prompt template and verify override indicator reflects source precedence; configure a `pre-commit` hook and assert it saves to `.sdlc/hooks.yaml`.
