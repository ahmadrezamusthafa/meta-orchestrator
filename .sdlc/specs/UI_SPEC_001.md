# UI/UX Implementation Specification: Meta-Orchestrator

**Target Tech Stack:** Vue 3 (Composition API), Tailwind CSS v3.4+, Pinia, `@xterm/xterm`, Lucide Vue Next, Radix Vue.  
**Grid Base:** 8pt strict grid.  
**Theme:** Dark-mode primary (`slate-950` base).

---

## 1. LAYOUT ARCHITECTURE

### 1.1 Global App Shell
* **Global Navigation Rail (Fixed Left):**
  * Width: `64px` collapsed (icon-only), expanding to `240px` on manual toggle.
  * Z-index: `40`.
  * Top-to-bottom layout: Brand glyph (`32x32px`), Primary Nav items (`40x40px` touch targets), System Health indicator, User Profile / Registry settings at bottom.
* **Global Status Header (Fixed Top):**
  * Height: `48px` (`h-12`).
  * Left: `64px` (matching nav rail), Width: `calc(100vw - 64px)`.
  * Z-index: `30`.
  * Contents: Active Workspace selector dropdown (`w-60`), Daemon Connection Status pill (`h-6`, with green pulse indicator), Global Token Burn Counter (`font-mono text-xs`), HITL Blocked Task indicator badge (conditional red alert).
* **Main Viewport Canvas:**
  * Position: `top: 48px`, `left: 64px`, `width: calc(100vw - 64px)`, `height: calc(100vh - 48px)`.
  * Overflow: Hidden on canvas root; internal scroll per pane.

### 1.2 View 1: Mission Control (Kanban Dashboard)
* **Structure:** Single horizontal scrolling container with dynamically projected columns.
* **Layout:**
  * Top Sub-header (`h-14`, `px-6`, `py-3`): Search input (`w-72`), Workflow Selector (`w-48`: `General AI SDLC`, `Hotfix Fast-Track`, `Custom Project Pipeline`), Method Filter (`w-36`), Repo Filter (`w-48`), "New Task" primary action button (`right-aligned`).
  * Board Container: Flex row (`gap-4`, `p-6`, `h-[calc(100%-56px)]`, `overflow-x-auto`).
  * **Dynamic Column Projection Engine:**
    - Columns dynamically render based on the active workflow definition:
      - *Default General AI SDLC (8 Columns, each `w-[320px]`, `min-w-[320px]`, `h-full`, `flex flex-col`):*
        1. `PRD & Dynamic Repo Discovery`
        2. `ATDD Creation (Red Phase)`
        3. `Tech Doc / RFC Review (Gate)`
        4. `Task Breakdown & Planning`
        5. `Implementation (Write-Unlocked)`
        6. `Automation & E2E Validation`
        7. `Manual & UAT Verification (Evidence)`
        8. `Ready for Sign-Off & Merge`
      - *Custom SDLC Pipelines:* If the project or filter selects a custom workflow (e.g., 3-stage Hotfix), columns automatically project matching custom stages (`Triage & Reproduction`, `Implementation & Regression`, `Verification & Video`).
* **"New Task" Modal & Pipeline Scope Selector (`NewTaskModal`):**
  * Dialog: `max-w-2xl bg-slate-900 border border-slate-800 rounded-xl shadow-2xl p-6 space-y-4`.
  * **Workflow Template Selector:**
    - Dropdown: `General AI SDLC (9-Stage Factory)` (Default), `Hotfix Fast-Track (3-Stage)`, `Microservice API (4-Stage)`, or `Custom Workflow from .sdlc/workflow.yaml`.
  * **Execution Scope Toggle:**
    - Option 1: `Full Pipeline Execution` (Execute all stages in selected workflow).
    - Option 2: `Partial / Custom Stage Slice` (Arbitrary entry & exit points).
  * **Stage Range Selector (When Partial Slice Selected):**
    - `Start From Stage`: Dropdown (lists stages of selected workflow).
    - `Halt At Stage`: Dropdown (lists downstream stages of selected workflow).
  * **Prerequisite Artifact Ingestion:**
    - File upload / text paste area for existing `TASK_PLAN.md`, `TECH_DOC_RFC.md`, or `PRD.md` when starting mid-process.
    - Git Branch Selector / Input: links directly to an existing feature branch.
  * **Video Evidence Toggle:** Checkbox `Record Playwright .mp4 test execution video` (default: checked).
  * **Method Selection Strategy:**
    - `Auto Best-Practice Matrix` (Dynamic per-stage/task assignment: BMAD for Planning, ReAct/Supervisor for Implementation).
    - `Custom Router Rules` / `Custom Method Override` (Pin entire execution or specific stages to a selected method).

### 1.3 View 2: Active Task Detail / Execution Workspace
* **Structure:** Two-column split-pane layout with resizable horizontal divider.
* **Pane Dimensions:**
  * **Left Execution Pane:** `flex-1` (default `60%` width, `min-w-[500px]`), full height.
    * Tab Bar (`h-10`, `border-b`): `Live Terminal (xterm.js)`, `Agent Thought Stream`, `Workspace Dependency Graph`.
    * Viewport (`h-[calc(100%-40px)]`): Houses either the canvas-based terminal, virtualized message list, or SVG topology graph.
  * **Right Control & Artifact Pane:** Fixed `40%` width (`min-w-[420px]`, `max-w-[640px]`), full height, vertical scroll.
    * Section 1: Task Metadata, Dynamically Identified Repos & Method Status Card (`h-auto`, `p-4`).
    * Section 2: Write-Lock Status Indicator & Gate Controls (`p-4`, sticky header: `Approve Tech Doc`, `Authorize Merge`).
    * Section 3: Artifact Viewer / Evidence Media Vault (`flex-1`, tabs: `PRD.md`, `ATDD_SUITE.md`, `TECH_DOC_RFC.md`, `TASK_PLAN.md`, `UAT_PREPARATION.md`, `EVIDENCE.md`, `Video (.mp4)`).
    * Section 4 (Bottom Dock): Live Context Injection Bar (`h-24`, sticky bottom).

### 1.4 View 3: Tool Hub & Dependency Lifecycle Manager
* **Structure:** High-density catalog and version management grid with sticky sub-header.
* **Layout:**
  * Top Sub-header (`h-14`, `px-6`, `py-3`): Search tool filter (`w-72`), Category Filter (Methodologies, Parsers/AST, Runtimes, UI Kits), "Auto-Resolve Best Fit Matrix" action button (`bg-sky-600 hover:bg-sky-500 text-white font-semibold text-xs px-3 py-1.5 rounded`).
  * Tool Grid (`p-6 grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4 overflow-y-auto`):
    * Houses interactive `ToolLifecycleCard` items for BMAD, Superpower, Playwright, Tree-sitter, xterm.js, ffmpeg, etc.
  * Installation & Rollback Stepper Drawer/Modal:
    * 4-step interactive guided installer (`Pre-flight Checks` → `Package Fetch & Link` → `Diagnostic Health Test` → `Ready`).
    * Version selector dropdown: `Latest`, `Best Fit (Recommended)`, previous versions for instant 1-click rollback.

### 1.5 View 4: AI Providers & Model Settings (`/settings/providers`)
* **Structure:** Two-tier settings layout: Global System Providers vs. Per-Project Cascading Overrides.
* **Layout:**
  * Top Sub-header (`h-14`, `px-6`, `py-3`): Context switcher toggle (`Global System Defaults` vs `Current Project: [Repo Name]`), "Test All Connections" action button (`bg-slate-800 hover:bg-slate-700 text-slate-200 text-xs px-3 py-1.5 rounded`).
  * Content Area (`p-6 space-y-6 max-w-5xl overflow-y-auto`):
    * Section A: Active Providers Grid (Claude, Antigravity, ChatGPT, OpenCode / Local AI, Custom Endpoint).
    * Section B: Tier Model Matrix Mapper (Assign provider + model ID to Tier 1 Reasoning, Tier 2 Code Gen, Tier 3 Log Parsing).
    * Section C: Cascading Inheritance Inspector: Displays configuration origin per tier (`Inherited from ~/.meta-orchestrator/config.yaml` or `Overridden by .sdlc/config.yaml`), with a 1-click "Create Project Override" button.
    * Section D: Routing Strategy Configurator (Best Practice vs. Custom Router).
    * Section E: Stage & Task Complexity Method Benchmark Matrix:
      - Interactive matrix table cross-referencing:
        `SDLC Stage` (PRD, ATDD, Tech Doc, Breakdown, Implementation, E2E) $\times$ `Task Complexity` (Low, Med, High, System) $\times$ `Method` (BMAD, Supervisor, ReAct, Superpower).
      - Metrics displayed per cell: First-Pass Verification Rate (FPVR %), Mean Token Cost ($), and MTTR (min).
      - "Proven Best Method" badges highlighting the optimal framework recommended by the shadow benchmarking engine.

### 1.6 View 5: Custom SDLC Workflow Builder & Manager (`/settings/workflows`)
* **Structure:** Visual pipeline editor and workflow catalog for authoring and assigning custom SDLC lifecycles.
* **Layout:**
  * Top Sub-header (`h-14`, `px-6`, `py-3`): "Workflow Catalog" selector, "New Custom Workflow" action button (`bg-emerald-600 text-white text-xs px-3 py-1.5 rounded`), "Export to .sdlc/workflow.yaml" button.
  * Workflow List & Card Grid:
    * Displays active workflows (`General AI SDLC (Built-in)`, `Hotfix Fast-Track`, `Microservice API`, `Spike / RFC Only`).
    * Badges: Active project default badge, stage count badge, gate count badge.
  * Visual Stage Sequence Builder:
    * Interactive node-based drag-and-drop linear/branching stage editor.
    * Stage Configuration Drawer:
      - Stage Name & Unique ID.
      - Assigned Method (`BMAD`, `Supervisor`, `ReAct`, `Superpower`, `Custom`).
      - Allowed Persona Roles (`Architect`, `Developer`, `QA`, `DevOps`).
      - Write-Locking Boundaries: toggle whether source code is locked until a previous stage red-phase passes.
      - Required Input & Output Artifacts (`*.md` schemas, `.mp4` video, test suites).
      - Gate Type: Automated Verification vs. Human Approval Gate (`AWAITING_APPROVAL`).
  * Live Preview: Visual simulation showing how the Kanban Board columns project for this custom workflow.

### 1.7 View 6: Multi-Source Registries & Hooks Manager (`/settings/registries`)
* **Structure:** Tabbed registry management interface governing Skills, Prompt Templates, and Lifecycle Hooks across Remote, System, and Local sources.
* **Tabs:**
  * **Tab 1: Skills Hub & Universal Adapter:**
    * Multi-Source Explorer:
      - `Remote Repositories`: Lists connected Git URLs (e.g. `github.com/org/ai-skills`) with "Add Remote Repo" modal, auto-sync status, and branch tags.
      - `Project Local Skills`: Scans `.sdlc/skills/` with hot-reload status indicators.
      - `System / Global Skills`: Scans `~/.config/meta-orchestrator/skills/`.
      - `Built-in Skills`: Core daemon tools (`run_playwright`, `docker_compose`, `read_ast`).
    * **Polyglot Skill Format Badges & Importers:**
      - Skill Format Indicators: `[Claude SKILL.md]`, `[BMAD Pack]`, `[Superpower Tool]`, `[MCP Server]`, `[Native Tool]`.
      - "Import / Connect Skill" Action Modal:
        - Mode 1: Connect Anthropic Model Context Protocol (MCP) server (stdio command or SSE URL).
        - Mode 2: Ingest Claude Code / Antigravity `SKILL.md` directory or git repository.
        - Mode 3: Ingest BMAD multi-agent skill bundle or Superpower shell tool.
    * Skill Detail Card: Displays JSON-Schema inputs, adapter normalization status, container execution constraints, and test execution button with live output drawer.
  * **Tab 2: Prompt Templates:**
    * Multi-Source Prompt Browser (Remote vs. System vs. Local `.sdlc/prompts/`).
    * Cascading Override Indicator: shows which template takes precedence at runtime.
    * Live Slot-Filling Preview: test prompt rendering with mock data (`{{system_architecture}}`, `{{failing_test_traces}}`).
  * **Tab 3: Lifecycle Hooks Engine:**
    * Interactive Hook Matrix mapping lifecycle events (`pre-stage`, `post-stage`, `on-failure`, `on-gate`, `pre-commit`, `post-commit`) to hook scripts or webhooks.
    * Hook Configuration: Type (`Shell Script`, `Docker Container`, `Webhook POST`), path/URL, timeout, retry count, and failure behavior (`Block Pipeline` vs. `Warn & Continue`).

### 1.8 Visual Hierarchy
* **Primary Focal Point:** The active execution state.
  * On Mission Control: Active cards in `Write-Locked` and `Implementation` states featuring high-contrast colored left borders (4px solid: `emerald-500` for running, `amber-500` for ATDD lock, `rose-500` for Frustrated/Blocked).
  * On Task Detail: The Live Terminal stdout stream and Frustration Warning Banner (when triggered, full-width `rose-950` background with `rose-400` border and pulsing alert icon).
* **Secondary Elements:** Collapsed historical logs, token counters (`text-slate-400 font-mono text-xs`), metadata key-value tables (`text-slate-300`).
* **Tertiary Elements:** Navigation rail icons, panel dividers (`border-slate-800`), file path tags.

---

## 2. COMPONENT INVENTORY

### 2.1 Primary Action Button (`BtnPrimary`)
* **Description:** Initiates task generation, runs manual gate approvals, triggers PR creation.
* **Data Density:** `h-9` (`36px`), `px-4`, `py-2`. Text: `text-xs font-semibold uppercase tracking-wider`.
* **States:**
  * *Default:* `bg-emerald-600 text-white shadow-sm`.
  * *Hover:* `bg-emerald-500 cursor-pointer`.
  * *Active:* `bg-emerald-700 ring-2 ring-emerald-400 ring-offset-2 ring-offset-slate-900`.
  * *Disabled:* `bg-slate-800 text-slate-500 cursor-not-allowed border border-slate-700/50`.
  * *Focused:* `outline-none ring-2 ring-emerald-400 ring-offset-2 ring-offset-slate-900`.

### 2.2 Destructive Action Button (`BtnDestructive`)
* **Description:** Triggers "Reset Workspace" and Docker volume purges.
* **Data Density:** `h-9` (`36px`), `px-4`, `py-2`. Text: `text-xs font-semibold`.
* **States:**
  * *Default:* `bg-rose-950/80 text-rose-300 border border-rose-800/80`.
  * *Hover:* `bg-rose-900 text-rose-100 border-rose-700`.
  * *Active:* `bg-rose-800 ring-2 ring-rose-500`.
  * *Disabled:* `bg-slate-900 text-slate-600 border-slate-800 cursor-not-allowed`.
  * *Focused:* `outline-none ring-2 ring-rose-500 ring-offset-2 ring-offset-slate-900`.

### 2.3 Kanban Task Card (`KanbanCard`)
* **Description:** Interactive card item representing a single task in the workflow.
* **Data Density:** `p-3.5` (`14px`), `rounded-lg`, `border border-slate-800`, `bg-slate-900/90`. Card height: self-contained (`~140px-180px`). Contains: Task ID (`font-mono text-xs text-slate-400`), Task Title (`text-sm font-medium line-clamp-2`), Method Badge (`h-5 text-[10px] px-1.5`), Target Repositories count tag (`text-[10px] font-mono`), Token Burn counter, Elapsed Time timer.
* **States:**
  * *Default:* `border-slate-800 bg-slate-900/90 shadow-sm`.
  * *Hover:* `border-slate-700 bg-slate-850 -translate-y-0.5 transition-transform duration-150`.
  * *Active / Dragging:* `border-emerald-500/80 shadow-lg shadow-emerald-950/40 rotate-1 scale-[1.02] cursor-grabbing`.
  * *Blocked / Frustrated:* `border-rose-600/80 bg-rose-950/30 animate-pulse-subtle`.
  * *Focused:* `ring-2 ring-emerald-500 border-transparent`.

### 2.4 Phase Status Badge (`PhaseStatusBadge`)
* **Description:** Displays the exact lifecycle gate.
* **Data Density:** Height: `20px` (`h-5`), `px-2`, `rounded-full`, `text-[10px] font-bold tracking-tight uppercase`.
* **States by Phase:**
  * *Planning / Profiling:* `bg-sky-950 text-sky-400 border border-sky-800`.
  * *ATDD Writing:* `bg-amber-950 text-amber-300 border border-amber-800`.
  * *Write-Locked (Red):* `bg-purple-950 text-purple-300 border border-purple-800`.
  * *Implementation (Green):* `bg-emerald-950 text-emerald-300 border border-emerald-800`.
  * *Blocked / Error:* `bg-rose-950 text-rose-300 border border-rose-800`.

### 2.5 Live Agent Terminal (`XtermTerminal`)
* **Description:** Container for `@xterm/xterm` canvas rendering raw Docker & test execution logs.
* **Data Density:** Maximum information density. Font: `JetBrains Mono` or `Fira Code`, `12px`, line-height `1.4`. Black background (`bg-black`), zero internal padding on canvas container, outer wrapper has `p-2` with `border-t border-slate-800`.
* **Controls:** Fixed top utility overlay (`h-8`, `px-2`, `bg-slate-900/90`): "Autoscroll" toggle switch, "Clear" button, "Download Log" icon button, Connection state pill (`Connected` / `Buffering` / `Terminated`).
* **States:**
  * *Streaming:* Bottom autoscroll locked, cursor blink enabled.
  * *Paused / Manual Scroll:* Top warning badge appears (`Scrollback Active - Autoscroll Paused`).
  * *Disconnected:* Semi-transparent overlay with `Disconnected from daemon socket` banner.

### 2.6 Streaming Thought Stream (`ThoughtFeed`)
* **Description:** Virtualized message timeline rendering agent reasoning and tool invocations.
* **Data Density:** Row-based card layout (`gap-3`, `p-4`). Each thought bubble: `bg-slate-900`, `border border-slate-800`, `rounded-md`, `p-3`.
  * Header line (`h-5`): Profile Glyph (16x16px), Profile Name (`font-semibold text-xs text-slate-200`), Model tag (`text-[10px] font-mono text-slate-400`), Timestamp (`text-[10px] text-slate-500`).
  * Body: Collapsible markdown stream (`text-xs text-slate-300 leading-relaxed font-sans`).
  * Tool Invocation Block: Embedded code block (`bg-slate-950 p-2 rounded text-[11px] font-mono text-emerald-400 border border-slate-800/80`).

### 2.7 HITL Live Context Injection Bar (`ContextInput`)
* **Description:** Text input dock allowing operators to inject steering instructions directly into the active LLM context.
* **Data Density:** Height: `80px`. `p-2.5`, `bg-slate-900 border-t border-slate-800`.
* **Elements:** Multi-line auto-expanding textarea (`h-12 max-h-24 text-xs font-sans placeholder:text-slate-500 bg-slate-950 border border-slate-700 rounded p-2 text-slate-200`), Right-aligned Action cluster (`Send Instruction` button, `Reset Volume` icon button).
* **States:**
  * *Default:* `border-slate-700 bg-slate-950`.
  * *Focused:* `border-emerald-500 ring-1 ring-emerald-500`.
  * *Disabled (Agent not running):* `opacity-50 cursor-not-allowed bg-slate-900`.
  * *Submitting:* Input disabled, button replaced with mini inline spinner.

### 2.8 Live Workspace Visualizer Graph (`WorkspaceGraphNode`)
* **Description:** Canvas/SVG rendering dynamically linked repositories and Docker mounts.
* **Data Density:** Node size: `180px x 64px`.
* **Contents:** Repo icon (`16x16`), Repo name (`text-xs font-bold`), Branch name (`text-[10px] font-mono text-slate-400`), Symlink indicator badge (`LNK` green dot or `ERR` red dot).
* **States:**
  * *Healthy Symlink:* Edge connector line is solid `emerald-500/70` with animated SVG dash offset indicating active IO.
  * *Broken / In-Conflict:* Edge connector line is dashed `rose-500` with pulsing alert icon on midpoint.
  * *Selected:* Node gets `ring-2 ring-sky-400 shadow-md`.

### 2.9 Evidence Media Vault (`VideoPlayerVault`)
* **Description:** Embedded HTML5 player with frame-stepping for Playwright `.mp4` recordings.
* **Data Density:** 16:9 aspect ratio container (`aspect-video w-full rounded-lg bg-black border border-slate-800`).
* **Controls:** Compact custom bar overlay (`h-8`, `px-3`, `bg-slate-950/80 backdrop-blur`): Play/Pause button (`16x16`), Time scrubber (`h-1.5 bg-slate-700 rounded flex-1`), Speed Selector (`1x`, `1.5x`, `2x`), Timestamp indicator (`00:14 / 00:42`), Fullscreen toggle.
* **Fail State:** Displays black box with centered message: `No MP4 recorded for this test execution run`.

### 2.10 Tool & Dependency Lifecycle Card (`ToolLifecycleCard`) & Wizard (`InstallWizardModal`)
* **Description:** Manages lifecycle, health diagnostics, step-by-step installation, updates, and rollback for tools (BMAD, Superpower, Playwright, xterm.js, Tree-sitter).
* **Card Data Density:** `p-4`, `rounded-xl`, `border border-slate-800`, `bg-slate-900`.
  * Top Row: Tool icon/glyph (`28x28px`), Tool Name (`font-semibold text-sm text-slate-100`), Version Badge (`text-[10px] font-mono px-2 py-0.5 rounded-full`), Health Dot (`emerald-500` = healthy, `amber-500` = update available, `rose-500` = incompatible/uninstalled).
  * Middle: Short description (`text-xs text-slate-400 line-clamp-2`), Compatibility Tag (`text-[10px] font-mono text-sky-400` e.g. `Best Fit: v2.4.1 (Node 20, Darwin arm64)`).
  * Bottom Action Bar:
    * If Uninstalled: `BtnPrimary` (`h-8 text-xs`): "Guided Install".
    * If Installed & Outdated: `BtnPrimary` ("Update to vX.Y") + dropdown menu ("Rollback to Previous", "Change Version").
    * If Installed & Healthy: Version indicator pill, "Re-run Diagnostics" icon button, "Rollback / Pin Version" button.
* **InstallWizardModal Structure:**
  * Fixed-width dialog (`max-w-xl`, `bg-slate-900 border border-slate-800 rounded-xl shadow-2xl p-6`).
  * Stepper Header: 4 steps with connector lines (`1. Pre-flight` → `2. Fetch & Build` → `3. Self-Test` → `4. Done`).
  * Embedded Mini-Console: 120px tall dark viewport streaming live bash download/installation logs.
  * Version Picker: Select between `Latest (vX.Y.Z)`, `Best Fit (Recommended)`, or manual version tag.

### 2.11 AI Provider Config Card (`ProviderConfigCard`) & Inheritance Badge (`ProjectOverrideBadge`)
* **Description:** Configuration card for managing credentials, base URLs, active models, and latency tests for Claude, Antigravity, ChatGPT, OpenCode, and Custom providers.
* **ProviderConfigCard Structure:**
  * Container: `p-4 rounded-xl border border-slate-800 bg-slate-900/90 space-y-3`.
  * Header: Provider Logo/Icon, Provider Name (`font-semibold text-sm text-slate-100`), Enable/Disable Switch, Latency Ping Badge (`142ms text-emerald-400 font-mono text-[10px]` or `Offline text-rose-400`).
  * Fields: API Key input (masked with eye reveal toggle), Base URL input (for OpenCode / local vLLM / custom proxies), Default Model dropdown.
  * Footer: "Test Connection" button (`h-7 text-xs px-2.5 bg-slate-800 hover:bg-slate-700 text-slate-200 rounded`), "Save" button.
* **ProjectOverrideBadge:**
  * **Inherited Mode:** `bg-slate-800 text-slate-400 border border-slate-700 text-[10px] font-mono px-2 py-0.5 rounded-full flex items-center gap-1`. Label: `System Inherited (Global)`.
  * **Custom Mode:** `bg-emerald-950 text-emerald-300 border border-emerald-800 text-[10px] font-mono px-2 py-0.5 rounded-full flex items-center gap-1`. Label: `Project Override (.sdlc/config.yaml)`.

---

## 3. TYPOGRAPHY & SPACING TOKENS

### 3.1 8pt Spacing Grid Tokens
All layout geometry, margins, and paddings conform strictly to 8-point increments (with explicit 4px half-grid exceptions for micro-badges and table cell padding):

| Token | Dimension | Applied Locations |
|---|---|---|
| `space-0.5` | `2px` | Border widths, indicator dot offsets |
| `space-1` | `4px` | Badge vertical padding, icon-to-text inline gaps |
| `space-2` | `8px` | Card internal gaps, button horizontal gaps, input padding |
| `space-3` | `12px` | Compact table cell vertical padding, sub-header horizontal padding |
| `space-4` | `16px` | Card interior padding, panel header padding, modal content padding |
| `space-5` | `20px` | Gap between card groups |
| `space-6` | `24px` | Section margins, Kanban column gaps, main dashboard outer margin |
| `space-8` | `32px` | Empty state vertical padding, drawer margins |
| `space-12` | `48px` | Global header height, major container margins |
| `space-16` | `64px` | Navigation rail width |

### 3.2 Typography Scale
Primary typeface: Inter (UI), JetBrains Mono / Fira Code (Code, Hashes, Logs, Metrics).

| Role | Font Family | Size | Weight | Line Height | Color Token |
|---|---|---|---|---|---|
| **Display / Header 1** | Inter | `20px` | SemiBold (600) | `28px` | `text-slate-100` |
| **Section Title / H2** | Inter | `16px` | SemiBold (600) | `24px` | `text-slate-200` |
| **Component Header / H3**| Inter | `14px` | Medium (500) | `20px` | `text-slate-200` |
| **Body (Default)** | Inter | `13px` | Regular (400) | `18px` | `text-slate-300` |
| **Body (Dense / Metadata)**| Inter | `12px` | Regular (400) | `16px` | `text-slate-400` |
| **Micro-Label / Badges** | Inter | `10px` | Bold (700) | `12px` | Uppercase, tracking-wider |
| **Terminal / Log Code** | JetBrains Mono| `12px` | Regular (400) | `16px` | `text-emerald-400` / ANSI |
| **Hash / Commit SHA** | JetBrains Mono| `11px` | Medium (500) | `14px` | `text-slate-400 font-mono` |
| **Token / Cost Metrics** | JetBrains Mono| `13px` | SemiBold (600) | `16px` | `text-sky-400 font-mono` |

---

## 4. EDGE CASES & FEEDBACK SPECIFICATIONS

### 4.1 Empty States
1. **Empty Kanban Column:**
   * Context: No tasks currently in the given lifecycle phase.
   * Container: `h-48 border border-dashed border-slate-800 rounded-lg flex flex-col items-center justify-center p-4`.
   * Icon: Lucide `Inbox` (`24x24px`, `text-slate-600`).
   * Copy: Primary text: `No active tasks` (`text-xs font-medium text-slate-400`). Secondary text: `Items routed to this phase will appear automatically` (`text-[11px] text-slate-500 text-center mt-1`).
   * CTA: None (passive listener).
2. **Empty Live Terminal (Task initialized, waiting for container):**
   * Background: Solid `slate-950`.
   * Center Loader: Lucide `Terminal` with a blinking underscore (`_`) in `text-emerald-500 font-mono text-sm`.
   * Copy: `Spawning ephemeral Docker Compose network and linking AST shards...`
3. **Empty Evidence Vault (Video / Screenshots):**
   * Container: `w-full h-64 border border-dashed border-slate-800 rounded-lg flex flex-col items-center justify-center`.
   * Icon: Lucide `VideoOff` (`28x28px`, `text-slate-600`).
   * Copy: `Evidence capture pending` (`text-xs font-semibold text-slate-300`). Subtext: `Visual recordings are captured during Phase 5 (Validation & Verification).`

### 4.2 Error States
1. **Agent Frustration Alert (Infinite Loop / 3+ Test Failures):**
   * Component: Top sticky banner inside Active Task Pane.
   * Style: `bg-rose-950 border-l-4 border-rose-600 p-4 shadow-xl flex items-start gap-3`.
   * Icon: Lucide `AlertOctagon` (`20x20px`, `text-rose-400 shrink-0 mt-0.5`).
   * Copy:
     * Header: `Execution Halted: Frustration Threshold Exceeded (Attempt 3/3 Failed)` (`text-xs font-bold text-rose-200 uppercase`).
     * Message: `Playwright test suite failed repeatedly with assertion: 'Expected HTTP 200, received 500'. Token burn paused to prevent resource exhaustion.` (`text-xs text-rose-300 mt-1`).
   * Actions: Two inline buttons (`gap-2 mt-3 flex`):
     * `BtnPrimary` (`text-xs h-7 px-3 bg-rose-600 hover:bg-rose-500`): `Inject Guidance & Retry`
     * `BtnDestructive` (`text-xs h-7 px-3`): `Purge Volume & Reset Workspace`
2. **WebSocket Disconnection (Daemon Failure):**
   * Component: Global floating toast at top-center (`top-4 left-1/2 -translate-x-1/2 z-50`).
   * Style: `bg-amber-950 border border-amber-800 text-amber-200 px-4 py-2 rounded-full shadow-2xl flex items-center gap-2`.
   * Indicator: Pulsing yellow dot (`w-2 h-2 rounded-full bg-amber-400 animate-ping`).
   * Copy: `Daemon WebSocket disconnected. Reconnecting in 3s... (State persisted in Redis)` (`text-xs font-mono`).
3. **Symlink Collision / Manifest Parse Failure:**
   * Inline code alert within Workspace Tab: `p-3 bg-red-950/60 border border-red-800 rounded text-xs font-mono text-red-300`.
   * Copy: `Error: Conflicting dependency '@company/api-contracts' detected between frontend package.json (v2.1) and backend composer.json (v3.0). Auto-symlink aborted.`

### 4.3 Loading States & Layout Shift Prevention
1. **Task Detail Shell Hydration:**
   * Never render empty layout spaces or resizing containers.
   * Left and right panes render fixed `60% / 40%` layout immediately.
   * Terminal viewport renders a dark placeholder canvas (`bg-black`) with an embedded pulsing skeleton bar at bottom (`w-32 h-4 bg-slate-800 rounded animate-pulse`).
2. **Kanban Card Loading Skeleton:**
   * Dimensions strictly match `KanbanCard` height (`156px`).
   * Style: `rounded-lg border border-slate-800/80 bg-slate-900/50 p-3.5 space-y-3`.
   * Elements:
     * Top row: Skeleton box `w-16 h-4 bg-slate-800 rounded` and `w-20 h-4 bg-slate-800 rounded`.
     * Middle: Two lines `w-full h-3.5 bg-slate-800 rounded` and `w-3/4 h-3.5 bg-slate-800 rounded`.
     * Bottom row: `w-12 h-3 bg-slate-800 rounded` and `w-16 h-3 bg-slate-800 rounded`.
3. **Artifact Markdown Viewer Loading:**
   * Document area displays a vertical stack of 8 skeleton lines of varying widths (`w-11/12`, `w-full`, `w-4/5`, `w-2/3`, each `h-3 bg-slate-800/60 rounded mb-2.5 animate-pulse`).
   * Prevents layout collapse when swapping between `UAT_PREPARATION.md` and `EVIDENCE.md`.
