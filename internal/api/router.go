package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/artifacts"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/config"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/connectors"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/fsm"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/projects"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/registry"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/skills"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/tools"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/ws"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// RouterConfig holds dependencies required by the REST API router.
type RouterConfig struct {
	TaskStore         fsm.TaskStateStore
	WorkflowReg       *fsm.WorkflowRegistry
	ToolManager       *tools.ToolManager
	ArtifactManager   *artifacts.ArtifactManager
	ConfigResolver    *config.CascadingConfigResolver
	ProjectManager    *projects.ProjectManager
	ConnectorsManager *connectors.Manager
	SkillResolver     *skills.MultiSourceSkillResolver
	PromptRegistry    *registry.PromptRegistry
	WSHub             *ws.Hub
	RootDir           string
}

// Router provides the HTTP REST API handler for the Mission Control frontend.
type Router struct {
	cfg           RouterConfig
	mux           *http.ServeMux
	mu            sync.RWMutex
	tasks         map[string]*types.Task
	taskProcesses map[string]*types.TaskProcessInfo
}

// NewRouter constructs a new REST API router.
func NewRouter(cfg RouterConfig) *Router {
	if cfg.ConnectorsManager == nil {
		cfg.ConnectorsManager = connectors.NewManager(cfg.RootDir)
	}
	if cfg.SkillResolver == nil {
		cfg.SkillResolver = skills.NewMultiSourceSkillResolver(cfg.RootDir)
	}
	if cfg.PromptRegistry == nil {
		cfg.PromptRegistry = registry.NewPromptRegistry(cfg.RootDir)
	}
	if cfg.SkillResolver != nil && cfg.ConnectorsManager != nil {
		cfg.SkillResolver.SetMCPProvider(cfg.ConnectorsManager)
		cfg.ConnectorsManager.SetOnMCPUpdateFunc(func(mcpExport map[string]interface{}) {
			cfg.SkillResolver.SetMCPProvider(cfg.ConnectorsManager)
		})
	}
	if cfg.WSHub != nil && cfg.ConnectorsManager != nil {
		cfg.ConnectorsManager.SetBroadcastFunc(cfg.WSHub.BroadcastEvent)
	}
	r := &Router{
		cfg:           cfg,
		mux:           http.NewServeMux(),
		tasks:         make(map[string]*types.Task),
		taskProcesses: make(map[string]*types.TaskProcessInfo),
	}
	r.seedDefaultTasks()
	r.registerRoutes()
	return r
}

func (r *Router) seedDefaultTasks() {
	now := time.Now()
	// Seed demo tasks for Mission Control visualization with JIRA and Confluence links
	r.tasks["TASK-8942"] = &types.Task{
		ID:                "TASK-8942",
		WorkflowID:        "general_ai_sdlc",
		Title:             "[PAY-1042] Implement Stripe payment gateway & webhook idempotency",
		Description:       "Add Stripe billing integration across frontend-portal and backend-core with replay protection.",
		CurrentStageID:    "task_implementation",
		CurrentStageIndex: 4,
		State:             types.TaskStateRunning,
		AssignedRepos:     []string{"frontend-portal", "backend-core", "api-contracts"},
		ProfileName:       "senior_fullstack_dev",
		SelectedMethod:    "BMAD",
		TokenUsage: types.TokenUsage{
			PromptTokens:     18420,
			CompletionTokens: 6210,
			TotalTokens:      24630,
			EstimatedCostUSD: 0.142,
		},
		MaxTokenBudget: 50000,
		ArtifactDir:    ".sdlc/artifacts/TASK-8942",
		Dependencies:   []string{"TASK-8938"},
		Metadata: map[string]string{
			"complexity":          "HIGH",
			"router_strategy":     "BEST_PRACTICE",
			"router_source":       "BP",
			"router_rationale":    "High complexity fullstack task routed to BMAD methodology using Claude 3.5 Sonnet",
			"jira_key":            "PAY-1042",
			"jira_url":            "https://jira.atlassian.net/browse/PAY-1042",
			"jira_status":         "In Progress",
			"jira_priority":       "High",
			"confluence_page_url": "https://wiki.atlassian.net/wiki/spaces/ARCH/pages/8942001/RFC-PAY-1042+Stripe+Payment+Gateway",
			"confluence_page_id":  "8942001",
			"confluence_space":    "ARCH",
			"worktree_enabled":    "true",
			"worktree_branch":     "feat/task-8942-stripe-billing",
		},
		CreatedAt: now.Add(-45 * time.Minute),
		UpdatedAt: now,
	}

	r.tasks["TASK-8943"] = &types.Task{
		ID:                "TASK-8943",
		WorkflowID:        "general_ai_sdlc",
		Title:             "[AUTH-409] OAuth2 Refresh Token Expiry Handling & UI Redirection",
		Description:       "Handle silent token refresh and route users to /login when refresh token expires.",
		CurrentStageID:    "atdd_creation",
		CurrentStageIndex: 1,
		State:             types.TaskStateRunning,
		AssignedRepos:     []string{"frontend-portal", "backend-core"},
		ProfileName:       "qa_automation_specialist",
		SelectedMethod:    "Supervisor",
		TokenUsage: types.TokenUsage{
			PromptTokens:     7520,
			CompletionTokens: 2100,
			TotalTokens:      9620,
			EstimatedCostUSD: 0.058,
		},
		MaxTokenBudget: 30000,
		ArtifactDir:    ".sdlc/artifacts/TASK-8943",
		Metadata: map[string]string{
			"complexity":       "MEDIUM",
			"write_lock":       "ACTIVE",
			"router_strategy":  "RULE_BASED",
			"router_source":    "RULE",
			"router_rationale": "Rule #2 matched: Stage=ATDD & Complexity=Medium -> Assigned Supervisor method",
			"jira_key":         "AUTH-409",
			"jira_url":         "https://jira.atlassian.net/browse/AUTH-409",
			"jira_status":      "To Do",
			"jira_priority":    "Medium",
			"worktree_enabled": "true",
			"worktree_branch":  "feat/task-8943-oauth2-refresh",
		},
		CreatedAt: now.Add(-20 * time.Minute),
		UpdatedAt: now,
	}

	r.tasks["TASK-8940"] = &types.Task{
		ID:                "TASK-8940",
		WorkflowID:        "general_ai_sdlc",
		Title:             "[GQL-330] GraphQL Schema Validation Bug in Subscription Resolver",
		Description:       "Repeated websocket timeout when querying active subscription state.",
		CurrentStageID:    "task_implementation",
		CurrentStageIndex: 4,
		State:             types.TaskStateBlockedFrustration,
		AssignedRepos:     []string{"backend-core"},
		ProfileName:       "backend_engineer",
		SelectedMethod:    "ReAct",
		TokenUsage: types.TokenUsage{
			PromptTokens:     34200,
			CompletionTokens: 11200,
			TotalTokens:      45400,
			EstimatedCostUSD: 0.285,
		},
		MaxTokenBudget: 40000,
		ArtifactDir:    ".sdlc/artifacts/TASK-8940",
		Metadata: map[string]string{
			"complexity":        "HIGH",
			"frustration_count": "3",
			"failing_trace":     "Playwright timeout: expected status 200 within 5000ms, received 504 Gateway Timeout",
			"router_source":     "BP",
			"jira_key":          "GQL-330",
			"jira_url":          "https://jira.atlassian.net/browse/GQL-330",
			"jira_status":       "Blocked",
			"jira_priority":     "Highest",
			"worktree_enabled":  "true",
			"worktree_branch":   "feat/task-8940-gql-resolver",
		},
		CreatedAt: now.Add(-120 * time.Minute),
		UpdatedAt: now,
	}

	r.tasks["TASK-8944"] = &types.Task{
		ID:                "TASK-8944",
		WorkflowID:        "general_ai_sdlc",
		Title:             "[PAY-1045] Checkout UI Payment Flow & Confirmation Receipts",
		Description:       "Frontend checkout modal and payment receipt components consuming Stripe webhook backend.",
		CurrentStageID:    "task_breakdown",
		CurrentStageIndex: 3,
		State:             types.TaskStateWaitingDependency,
		AssignedRepos:     []string{"frontend-portal", "api-contracts"},
		Dependencies:      []string{"TASK-8942"},
		ProfileName:       "frontend_engineer",
		SelectedMethod:    "BMAD",
		TokenUsage: types.TokenUsage{
			PromptTokens:     4200,
			CompletionTokens: 1100,
			TotalTokens:      5300,
			EstimatedCostUSD: 0.031,
		},
		MaxTokenBudget: 35000,
		ArtifactDir:    ".sdlc/artifacts/TASK-8944",
		Metadata: map[string]string{
			"complexity":         "MEDIUM",
			"router_source":      "BP",
			"jira_key":           "PAY-1045",
			"jira_url":           "https://jira.atlassian.net/browse/PAY-1045",
			"jira_status":        "Waiting for Dependency",
			"jira_priority":      "High",
			"worktree_enabled":   "true",
			"worktree_branch":    "feat/task-8944-checkout-ui",
			"unmet_dependencies": "TASK-8942",
		},
		CreatedAt: now.Add(-10 * time.Minute),
		UpdatedAt: now,
	}

	r.tasks["TASK-8938"] = &types.Task{
		ID:                "TASK-8938",
		WorkflowID:        "general_ai_sdlc",
		Title:             "User Profile Avatar Upload & S3 Bucket Presigned URLs",
		Description:       "End to end verified upload of user avatars with direct S3 multipart uploading.",
		CurrentStageID:    "signoff_merge",
		CurrentStageIndex: 7,
		State:             types.TaskStateCompleted,
		AssignedRepos:     []string{"frontend-portal", "backend-core"},
		ProfileName:       "lead_architect",
		SelectedMethod:    "BMAD",
		TokenUsage: types.TokenUsage{
			PromptTokens:     28400,
			CompletionTokens: 9100,
			TotalTokens:      37500,
			EstimatedCostUSD: 0.210,
		},
		MaxTokenBudget: 60000,
		ArtifactDir:    ".sdlc/artifacts/TASK-8938",
		Metadata: map[string]string{
			"complexity":       "MEDIUM",
			"router_source":    "BP",
			"video_url":        "/api/v1/artifacts/TASK-8938/run_final.mp4",
			"worktree_enabled": "true",
			"worktree_branch":  "feat/task-8938-avatar-upload",
		},
		CreatedAt: now.Add(-240 * time.Minute),
		UpdatedAt: now,
	}

	// Seed realistic background processes & console logs for Mission Control tasks
	r.taskProcesses["TASK-8942"] = &types.TaskProcessInfo{
		TaskID:          "TASK-8942",
		ProcessID:       4812,
		Command:         "go test -v ./internal/billing/... -run TestStripeWebhookIdempotency",
		WorkingDir:      "/workspaces/TASK-8942/backend-core",
		ContainerID:     "orch-sandbox-8942",
		Status:          "RUNNING",
		StartedAt:       now.Add(-45 * time.Minute),
		DurationSeconds: 2700,
		CPUPercent:      18.4,
		MemoryMB:        142.5,
		CurrentStep:     "Step 4 of 7: Task Implementation (BMAD Method)",
		ActiveAgent:     "senior_fullstack_dev (Claude 3.5 Sonnet)",
		SubProcesses: []types.TaskSubProcess{
			{PID: 4813, Command: "npm --prefix frontend-portal run test:unit -- PaymentModal", Status: "RUNNING"},
		},
		Logs: []string{
			fmt.Sprintf("[%s] \x1b[36m[Orchestrator]\x1b[0m Container sandbox initialized on isolated docker network (orch-sandbox-8942).", now.Add(-45*time.Minute).Format("15:04:05")),
			fmt.Sprintf("[%s] \x1b[32m[Git Coordinator]\x1b[0m Synchronized feature branches across target repositories: frontend-portal, backend-core, api-contracts.", now.Add(-44*time.Minute).Format("15:04:05")),
			fmt.Sprintf("[%s] \x1b[33m[Agent Loop]\x1b[0m Selected BMAD Method. Executing implementation phase for Stripe billing & webhook idempotency.", now.Add(-43*time.Minute).Format("15:04:05")),
			fmt.Sprintf("[%s] \x1b[34m[Process Spawn]\x1b[0m Spawned PID 4812: `go test -v ./internal/billing/... -run TestStripeWebhookIdempotency` in /workspaces/TASK-8942/backend-core", now.Add(-40*time.Minute).Format("15:04:05")),
			fmt.Sprintf("[%s] === RUN   TestStripeWebhookIdempotency", now.Add(-39*time.Minute).Format("15:04:05")),
			fmt.Sprintf("[%s] === RUN   TestStripeWebhookIdempotency/Replay_Protection_With_Redis_Lock", now.Add(-38*time.Minute).Format("15:04:05")),
			fmt.Sprintf("[%s] --- PASS: TestStripeWebhookIdempotency/Replay_Protection_With_Redis_Lock (0.12s)", now.Add(-38*time.Minute).Format("15:04:05")),
			fmt.Sprintf("[%s] === RUN   TestStripeWebhookIdempotency/Signature_Validation_Header_Verification", now.Add(-37*time.Minute).Format("15:04:05")),
			fmt.Sprintf("[%s] --- PASS: TestStripeWebhookIdempotency/Signature_Validation_Header_Verification (0.04s)", now.Add(-37*time.Minute).Format("15:04:05")),
			fmt.Sprintf("[%s] \x1b[32m[Coverage]\x1b[0m Backend test suite passed (89.2%% line coverage).", now.Add(-35*time.Minute).Format("15:04:05")),
			fmt.Sprintf("[%s] \x1b[35m[Sub-Process]\x1b[0m Spawned PID 4813: `npm --prefix frontend-portal run test:unit -- PaymentModal`", now.Add(-20*time.Minute).Format("15:04:05")),
			fmt.Sprintf("[%s] PASS  frontend-portal/src/components/PaymentModal.test.tsx (12 tests passed)", now.Add(-18*time.Minute).Format("15:04:05")),
			fmt.Sprintf("[%s] \x1b[32m[ATDD Check]\x1b[0m 12/12 ATDD specifications passing green.", now.Add(-10*time.Minute).Format("15:04:05")),
			fmt.Sprintf("[%s] \x1b[36m[Orchestrator]\x1b[0m Background process active: running continuous integration verification & watching file changes.", now.Add(-2*time.Minute).Format("15:04:05")),
		},
	}

	r.taskProcesses["TASK-8944"] = &types.TaskProcessInfo{
		TaskID:          "TASK-8944",
		ProcessID:       4920,
		Command:         "git worktree add ../worktrees/task-8944 -b feat/task-8944-checkout-ui feat/task-8942-stripe-billing",
		WorkingDir:      "/workspaces/TASK-8944",
		ContainerID:     "orch-sandbox-8944",
		Status:          "PAUSED",
		StartedAt:       now.Add(-10 * time.Minute),
		DurationSeconds: 600,
		CPUPercent:      0.0,
		MemoryMB:        24.0,
		CurrentStep:     "Step 3 of 7: Task Breakdown (Held in WAITING_DEPENDENCY)",
		ActiveAgent:     "frontend_engineer (Awaiting TASK-8942)",
		Logs: []string{
			fmt.Sprintf("[%s] \x1b[33m[Dependency DAG]\x1b[0m Evaluated task dependencies: Requires [TASK-8942] to be COMPLETED.", now.Add(-10*time.Minute).Format("15:04:05")),
			fmt.Sprintf("[%s] \x1b[33m[Dependency DAG]\x1b[0m Prerequisite TASK-8942 is currently in `task_implementation` (RUNNING). Holding task execution.", now.Add(-9*time.Minute).Format("15:04:05")),
			fmt.Sprintf("[%s] \x1b[36m[Git Worktree]\x1b[0m Parallel worktree prepared. Once TASK-8942 completes, worktree will automatically branch from feat/task-8942-stripe-billing.", now.Add(-8*time.Minute).Format("15:04:05")),
			fmt.Sprintf("[%s] \x1b[32m[Scheduler]\x1b[0m Subscribed to completion signal for TASK-8942.", now.Add(-7*time.Minute).Format("15:04:05")),
		},
	}

	r.taskProcesses["TASK-8943"] = &types.TaskProcessInfo{
		TaskID:          "TASK-8943",
		ProcessID:       5120,
		Command:         "playwright codegen --save-trace=atdd/auth-expiry.spec.ts",
		WorkingDir:      "/workspaces/TASK-8943/frontend-portal",
		ContainerID:     "orch-sandbox-8943",
		Status:          "RUNNING",
		StartedAt:       now.Add(-20 * time.Minute),
		DurationSeconds: 1200,
		CPUPercent:      9.2,
		MemoryMB:        88.0,
		CurrentStep:     "Step 2 of 7: ATDD Specification & Test Recording (Supervisor Method)",
		ActiveAgent:     "qa_automation_specialist (Supervisor)",
		Logs: []string{
			fmt.Sprintf("[%s] \x1b[36m[Orchestrator]\x1b[0m Container sandbox initialized on isolated docker network (orch-sandbox-8943).", now.Add(-20*time.Minute).Format("15:04:05")),
			fmt.Sprintf("[%s] \x1b[33m[Write-Lock Guard]\x1b[0m Read-only locks verified on application source tree (Red Phase ATDD).", now.Add(-19*time.Minute).Format("15:04:05")),
			fmt.Sprintf("[%s] \x1b[34m[Process Spawn]\x1b[0m Spawned PID 5120: `playwright codegen --save-trace=atdd/auth-expiry.spec.ts`", now.Add(-18*time.Minute).Format("15:04:05")),
			fmt.Sprintf("[%s] \x1b[32m[ATDD Synthesizer]\x1b[0m Generating failing acceptance tests for silent token refresh & session timeout.", now.Add(-10*time.Minute).Format("15:04:05")),
			fmt.Sprintf("[%s] \x1b[36m[Orchestrator]\x1b[0m Synthesized ATDD suite in atdd/auth-expiry.spec.ts (Expected RED state).", now.Add(-2*time.Minute).Format("15:04:05")),
		},
	}

	r.taskProcesses["TASK-8940"] = &types.TaskProcessInfo{
		TaskID:          "TASK-8940",
		ProcessID:       4380,
		Command:         "npx playwright test e2e/subscription.spec.ts --timeout=5000",
		WorkingDir:      "/workspaces/TASK-8940/backend-core",
		ContainerID:     "orch-sandbox-8940",
		Status:          "BLOCKED",
		StartedAt:       now.Add(-120 * time.Minute),
		DurationSeconds: 7200,
		CPUPercent:      0.0,
		MemoryMB:        45.2,
		CurrentStep:     "Step 5 of 7: E2E Validation (Halted by Frustration Circuit Breaker)",
		ActiveAgent:     "backend_engineer (ReAct Method)",
		ExitCode:        1,
		Logs: []string{
			fmt.Sprintf("[%s] \x1b[36m[Orchestrator]\x1b[0m Container sandbox initialized on isolated docker network (orch-sandbox-8940).", now.Add(-120*time.Minute).Format("15:04:05")),
			fmt.Sprintf("[%s] \x1b[34m[Process Spawn]\x1b[0m Spawned PID 4380: `npx playwright test e2e/subscription.spec.ts --timeout=5000`", now.Add(-115*time.Minute).Format("15:04:05")),
			fmt.Sprintf("[%s] Running 1 test using 1 worker", now.Add(-114*time.Minute).Format("15:04:05")),
			fmt.Sprintf("[%s] \x1b[31m[ERROR]\x1b[0m Playwright timeout: expected status 200 within 5000ms, received 504 Gateway Timeout", now.Add(-110*time.Minute).Format("15:04:05")),
			fmt.Sprintf("[%s] \x1b[31m[FAIL]\x1b[0m e2e/subscription.spec.ts:42:5 › WebSocket GraphQL Resolver Subscription Timeout", now.Add(-110*time.Minute).Format("15:04:05")),
			fmt.Sprintf("[%s] \x1b[33m[Loop Attempt 2/3]\x1b[0m Retrying with adjusted keep-alive interval...", now.Add(-80*time.Minute).Format("15:04:05")),
			fmt.Sprintf("[%s] \x1b[31m[FAIL]\x1b[0m Repeated timeout: 504 Gateway Timeout from GraphQL subscription handler.", now.Add(-75*time.Minute).Format("15:04:05")),
			fmt.Sprintf("[%s] \x1b[33m[Loop Attempt 3/3]\x1b[0m Retrying query payload with exponential backoff...", now.Add(-40*time.Minute).Format("15:04:05")),
			fmt.Sprintf("[%s] \x1b[31m[FAIL]\x1b[0m 504 Gateway Timeout encountered on 3 consecutive iterations.", now.Add(-35*time.Minute).Format("15:04:05")),
			fmt.Sprintf("[%s] \x1b[31;1m[CIRCUIT BREAKER]\x1b[0m Frustration threshold exceeded (3 consecutive failures). Process suspended for Human-In-The-Loop guidance.", now.Add(-30*time.Minute).Format("15:04:05")),
		},
	}

	r.taskProcesses["TASK-8938"] = &types.TaskProcessInfo{
		TaskID:          "TASK-8938",
		ProcessID:       3920,
		Command:         "git push origin feature/avatar-upload && gh pr create --fill",
		WorkingDir:      "/workspaces/TASK-8938/frontend-portal",
		ContainerID:     "orch-sandbox-8938",
		Status:          "COMPLETED",
		StartedAt:       now.Add(-240 * time.Minute),
		DurationSeconds: 14400,
		CPUPercent:      0.0,
		MemoryMB:        12.0,
		CurrentStep:     "Step 7 of 7: Signoff & Merge Complete",
		ActiveAgent:     "lead_architect (BMAD Method)",
		ExitCode:        0,
		Logs: []string{
			fmt.Sprintf("[%s] \x1b[36m[Orchestrator]\x1b[0m All ATDD specifications and UAT verification passed (100%% green).", now.Add(-60*time.Minute).Format("15:04:05")),
			fmt.Sprintf("[%s] \x1b[32m[Git Signoff]\x1b[0m Pushed commits to origin/feature/avatar-upload.", now.Add(-55*time.Minute).Format("15:04:05")),
			fmt.Sprintf("[%s] \x1b[32m[Pull Request]\x1b[0m Created Pull Request #48: 'User Profile Avatar Upload & S3 Bucket Presigned URLs'.", now.Add(-50*time.Minute).Format("15:04:05")),
			fmt.Sprintf("[%s] \x1b[32m[SUCCESS]\x1b[0m Task execution completed successfully. Container sandbox preserved in cold storage.", now.Add(-45*time.Minute).Format("15:04:05")),
		},
	}
}

func (r *Router) getOrCreateTaskProcessLocked(taskID string) *types.TaskProcessInfo {
	if proc, ok := r.taskProcesses[taskID]; ok {
		return proc
	}

	task := r.tasks[taskID]
	title := taskID
	method := "BMAD"
	step := "Step 1 of 7: Discovery"
	if task != nil {
		title = task.Title
		method = task.SelectedMethod
		step = fmt.Sprintf("Step %d of 7: %s (%s Method)", task.CurrentStageIndex+1, task.CurrentStageID, method)
	}

	now := time.Now()
	proc := &types.TaskProcessInfo{
		TaskID:          taskID,
		ProcessID:       4900 + len(r.taskProcesses),
		Command:         fmt.Sprintf("orchestrator run-stage --task=%s --method=%s", taskID, method),
		WorkingDir:      fmt.Sprintf("/workspaces/%s", taskID),
		ContainerID:     fmt.Sprintf("orch-sandbox-%s", strings.ToLower(taskID)),
		Status:          "RUNNING",
		StartedAt:       now,
		DurationSeconds: 120,
		CPUPercent:      12.5,
		MemoryMB:        96.0,
		CurrentStep:     step,
		ActiveAgent:     "orchestrator_agent",
		Logs: []string{
			fmt.Sprintf("[%s] \x1b[36m[Orchestrator]\x1b[0m Container sandbox initialized on isolated docker network.", now.Format("15:04:05")),
			fmt.Sprintf("[%s] \x1b[32m[Task Coordinator]\x1b[0m Active background process started for %s.", now.Format("15:04:05"), title),
			fmt.Sprintf("[%s] \x1b[34m[Process Spawn]\x1b[0m Spawned PID %d: `orchestrator run-stage --task=%s --method=%s`", now.Format("15:04:05"), 4900+len(r.taskProcesses), taskID, method),
			fmt.Sprintf("[%s] \x1b[36m[Console Stream]\x1b[0m STDOUT / STDERR live streaming active.", now.Format("15:04:05")),
		},
	}
	r.taskProcesses[taskID] = proc
	return proc
}

func (r *Router) registerRoutes() {
	r.mux.HandleFunc("/api/v1/tasks", r.handleTasks)
	r.mux.HandleFunc("/api/v1/tasks/", r.handleTaskItem)
	r.mux.HandleFunc("/api/v1/tools", r.handleTools)
	r.mux.HandleFunc("/api/v1/tools/", r.handleToolAction)
	r.mux.HandleFunc("/api/v1/providers", r.handleProviders)
	r.mux.HandleFunc("/api/v1/providers/test", r.handleProviderTest)
	r.mux.HandleFunc("/api/v1/providers/oauth/initiate", r.handleOAuthInitiate)
	r.mux.HandleFunc("/api/v1/providers/oauth/callback", r.handleOAuthCallback)
	r.mux.HandleFunc("/api/v1/providers/oauth/status", r.handleOAuthStatus)
	r.mux.HandleFunc("/api/v1/providers/oauth/disconnect", r.handleOAuthDisconnect)
	r.mux.HandleFunc("/api/v1/providers/oauth/providers", r.handleOAuthProviders)
	r.mux.HandleFunc("/api/v1/workflows", r.handleWorkflows)
	r.mux.HandleFunc("/api/v1/registries", r.handleRegistries)
	r.mux.HandleFunc("/api/v1/benchmarks", r.handleBenchmarks)
	r.mux.HandleFunc("/api/v1/artifacts/", r.handleArtifacts)
	r.mux.HandleFunc("/api/v1/projects", r.handleProjects)
	r.mux.HandleFunc("/api/v1/projects/scan", r.handleProjectScan)
	r.mux.HandleFunc("/api/v1/projects/", r.handleProjectItem)
	r.mux.HandleFunc("/api/v1/fs/browse", r.handleFSBrowse)
	r.mux.HandleFunc("/api/v1/fs/mkdir", r.handleFSMkdir)

	// Connector endpoints (Modular Catalog, Items, JIRA & Confluence, MCP)
	r.mux.HandleFunc("/api/v1/connectors", r.handleConnectors)
	r.mux.HandleFunc("/api/v1/connectors/catalog", r.handleConnectorCatalog)
	r.mux.HandleFunc("/api/v1/connectors/mcp-config", r.handleConnectorMCPConfig)
	r.mux.HandleFunc("/api/v1/connectors/items/", r.handleConnectorItemAction)
	r.mux.HandleFunc("/api/v1/connectors/jira", r.handleConnectorJira)
	r.mux.HandleFunc("/api/v1/connectors/confluence", r.handleConnectorConfluence)
	r.mux.HandleFunc("/api/v1/connectors/test", r.handleConnectorTest)
	r.mux.HandleFunc("/api/v1/connectors/jira/issues", r.handleJiraIssues)
	r.mux.HandleFunc("/api/v1/connectors/jira/import", r.handleJiraImport)
	r.mux.HandleFunc("/api/v1/connectors/confluence/publish", r.handleConfluencePublish)
	r.mux.HandleFunc("/api/v1/connectors/ping", r.handleConnectorPing)
	r.mux.HandleFunc("/api/v1/connectors/ping-config", r.handleConnectorPingConfig)
	r.mux.HandleFunc("/api/v1/skills", r.handleSkills)
	r.mux.HandleFunc("/api/v1/skills/sources", r.handleSkillSources)
	r.mux.HandleFunc("/api/v1/skills/sources/", r.handleSkillSourceAction)
	r.mux.HandleFunc("/api/v1/skills/check-compatibility", r.handleSkillCheckCompatibility)
	r.mux.HandleFunc("/api/v1/skills/rescan", r.handleSkillRescan)
	r.mux.HandleFunc("/api/v1/skills/", r.handleSkillAction)

	// Prompt template endpoints (Multi-Tier: Local, GitHub, Custom, System, Builtin)
	r.mux.HandleFunc("/api/v1/prompts", r.handlePrompts)
	r.mux.HandleFunc("/api/v1/prompts/sources", r.handlePromptSources)
	r.mux.HandleFunc("/api/v1/prompts/sources/", r.handlePromptSourceAction)
	r.mux.HandleFunc("/api/v1/prompts/check-compatibility", r.handlePromptCheckCompatibility)
	r.mux.HandleFunc("/api/v1/prompts/render", r.handlePromptRender)
}

func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// Enable CORS for development frontend
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With")

	if req.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	r.mux.ServeHTTP(w, req)
}

func (r *Router) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func (r *Router) writeError(w http.ResponseWriter, status int, msg string) {
	r.writeJSON(w, status, map[string]string{"error": msg})
}
