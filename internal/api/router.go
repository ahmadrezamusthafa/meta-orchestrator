package api

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/artifacts"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/config"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/connectors"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/fsm"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/projects"
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
	WSHub             *ws.Hub
	RootDir           string
}

// Router provides the HTTP REST API handler for the Mission Control frontend.
type Router struct {
	cfg   RouterConfig
	mux   *http.ServeMux
	mu    sync.RWMutex
	tasks map[string]*types.Task
}

// NewRouter constructs a new REST API router.
func NewRouter(cfg RouterConfig) *Router {
	if cfg.ConnectorsManager == nil {
		cfg.ConnectorsManager = connectors.NewManager(cfg.RootDir)
	}
	r := &Router{
		cfg:   cfg,
		mux:   http.NewServeMux(),
		tasks: make(map[string]*types.Task),
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
		},
		CreatedAt: now.Add(-120 * time.Minute),
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
			"complexity":    "MEDIUM",
			"router_source": "BP",
			"video_url":     "/api/v1/artifacts/TASK-8938/run_final.mp4",
		},
		CreatedAt: now.Add(-240 * time.Minute),
		UpdatedAt: now,
	}
}

func (r *Router) registerRoutes() {
	r.mux.HandleFunc("/api/v1/tasks", r.handleTasks)
	r.mux.HandleFunc("/api/v1/tasks/", r.handleTaskItem)
	r.mux.HandleFunc("/api/v1/tools", r.handleTools)
	r.mux.HandleFunc("/api/v1/tools/", r.handleToolAction)
	r.mux.HandleFunc("/api/v1/providers", r.handleProviders)
	r.mux.HandleFunc("/api/v1/providers/test", r.handleProviderTest)
	r.mux.HandleFunc("/api/v1/workflows", r.handleWorkflows)
	r.mux.HandleFunc("/api/v1/registries", r.handleRegistries)
	r.mux.HandleFunc("/api/v1/benchmarks", r.handleBenchmarks)
	r.mux.HandleFunc("/api/v1/artifacts/", r.handleArtifacts)
	r.mux.HandleFunc("/api/v1/projects", r.handleProjects)
	r.mux.HandleFunc("/api/v1/projects/scan", r.handleProjectScan)
	r.mux.HandleFunc("/api/v1/projects/", r.handleProjectItem)
	r.mux.HandleFunc("/api/v1/fs/browse", r.handleFSBrowse)
	r.mux.HandleFunc("/api/v1/fs/mkdir", r.handleFSMkdir)

	// Connector endpoints (JIRA & Confluence)
	r.mux.HandleFunc("/api/v1/connectors", r.handleConnectors)
	r.mux.HandleFunc("/api/v1/connectors/jira", r.handleConnectorJira)
	r.mux.HandleFunc("/api/v1/connectors/confluence", r.handleConnectorConfluence)
	r.mux.HandleFunc("/api/v1/connectors/test", r.handleConnectorTest)
	r.mux.HandleFunc("/api/v1/connectors/jira/issues", r.handleJiraIssues)
	r.mux.HandleFunc("/api/v1/connectors/jira/import", r.handleJiraImport)
	r.mux.HandleFunc("/api/v1/connectors/confluence/publish", r.handleConfluencePublish)
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
