package api

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/artifacts"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/atdd"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/circuitbreaker"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/config"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/connectors"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/fsm"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/hooks"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/llm"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/packages"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/projects"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/pullrequest"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/quota"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/registry"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/router"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/security"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/skills"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/telemetry"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/toolchain"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/worker"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/ws"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// RouterConfig holds dependencies required by the REST API router.
type RouterConfig struct {
	TaskStore   fsm.TaskStateStore
	WorkflowReg *fsm.WorkflowRegistry
	// Packages installs and tracks the tools the orchestrator itself needs. Nil loads
	// RootDir/configs/packages.json with its state in packages.DefaultHome().
	Packages          *packages.Manager
	ArtifactManager   *artifacts.ArtifactManager
	ConfigResolver    *config.CascadingConfigResolver
	ProjectManager    *projects.ProjectManager
	ConnectorsManager *connectors.Manager
	SkillResolver     *skills.MultiSourceSkillResolver
	PromptRegistry    *registry.PromptRegistry
	WSHub             *ws.Hub
	RootDir           string
	Telemetry         TelemetryConfig
	// TaskStorePath is the JSON file holding the Kanban board. Empty keeps tasks in memory only.
	TaskStorePath string
	// ProviderStatePath is the JSON file holding provider credentials, default models and quotas.
	// Empty keeps them in memory only, so they are lost on restart.
	ProviderStatePath string
	// QuotaFetchers read provider-side plan limits per provider id. Nil uses the local Claude Code
	// and Antigravity logins (quota.DefaultFetchers); tests pass an empty map to stay offline.
	QuotaFetchers map[string]quota.Fetcher
	// Docker limits the Docker work agents run on this machine; zero values use the defaults.
	Docker DockerLimits
	// EnableJiraSync runs the background JIRA → board sync using the rules in connectors.json.
	EnableJiraSync bool
	// Toolchain checks and installs the tools the repositories' stacks need. Nil checks this
	// machine (toolchain.New).
	Toolchain *toolchain.Toolchain
	// ProfileRegistry holds the stage roles. Nil loads RootDir/configs/profiles.json.
	ProfileRegistry *registry.ProfileRegistry
	// MaxConcurrentStages bounds the stages (agent turns) running at once; 0 uses 10.
	MaxConcurrentStages int
	// StageQueueSize is how many claimed stages may wait for a free slot; 0 uses 100.
	StageQueueSize int
	// HookEngine runs lifecycle hooks; nil creates one for RootDir. Hooks in RootDir/.sdlc/hooks.json
	// are loaded into it.
	HookEngine *hooks.HookEngine
}

// Router provides the HTTP REST API handler for the Mission Control frontend.
type Router struct {
	cfg            RouterConfig
	mux            *http.ServeMux
	mu             sync.RWMutex
	tasks          map[string]*types.Task
	taskProcesses  map[string]*types.TaskProcessInfo
	strategyRouter *router.Router
	budgetTracker  *router.BudgetTracker
	clientFactory  *llm.ClientFactory
	telemetry      *telemetrySubsystem
	console        *consoleHub
	quota          *quota.Service
	connections    *connectionChecker

	taskSeq       int             // last allocated TASK-N; guarded by mu
	dismissedJira map[string]bool // JIRA keys the operator removed from the board; guarded by mu
	jira          *jiraSyncState
	persistMu     sync.Mutex
	worktreeMu    sync.Mutex // serializes git worktree creation/removal
	approvals     *approvalHub
	docker        *dockerGate          // limits concurrent Docker commands across all agents
	toolchain     *toolchain.Toolchain // checks and installs the tools the stacks need
	evidenceJobs  sync.WaitGroup       // background evidence finalization (transcode, signed manifest)
	busyOps       sync.Map             // "pr:<task>" / "uat:<task>" while a delivery job runs
	prClient      *pullrequest.Client  // opens pull requests; tests point it at a fake API
	uatRunner     uatScreenshotRunner  // captures UAT screenshots; tests stub it

	// Guardrails on the stage flow (guardrails.go).
	methods     *registry.MethodRegistry           // per-method limits (configs/methods.json)
	breaker     *circuitbreaker.CircuitBreaker     // halts a stage that keeps failing the same way
	writeLocks  *security.WriteLockManager         // ATDD red-phase write lock
	interceptor *security.PolyglotSkillInterceptor // enforces writeLocks on tool calls
	redPhase    *atdd.RedPhaseVerifier             // judges the ATDD test run

	lastPersisted []byte
	stages        *worker.Pool // bounded pool every stage turn runs on
	hookCtx       context.Context
	hookCancel    context.CancelFunc
	hookWG        sync.WaitGroup
	mirrored      map[string][]byte // task snapshots last written to cfg.TaskStore; guarded by persistMu
	mirrorFailing bool              // guarded by persistMu
	stop          chan struct{}
	closeOnce     sync.Once

	pkgOnce      sync.Once
	pkgMgr       *packages.Manager // the tools the orchestrator itself needs (packages_handler.go)
	profilesOnce sync.Once
	profiles     *registry.ProfileRegistry // stage roles (configs/profiles.json)
}

// NewRouter constructs a new REST API router.
func NewRouter(cfg RouterConfig) *Router {
	if cfg.ArtifactManager == nil {
		cfg.ArtifactManager = artifacts.NewArtifactManager(filepath.Join(cfg.RootDir, ".sdlc", "artifacts"))
	}
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
	if cfg.WorkflowReg == nil {
		cfg.WorkflowReg = fsm.NewWorkflowRegistry()
	}

	var orchCfg *config.OrchestratorConfig
	if cfg.ConfigResolver != nil {
		orchCfg, _ = cfg.ConfigResolver.Resolve()
	}
	if orchCfg == nil {
		orchCfg = config.GetDefaultConfig()
	}

	stratRouter := router.NewRouter(orchCfg)
	budgTracker := router.NewBudgetTracker()
	loadProviderState(cfg.ProviderStatePath)
	clFactory := llm.NewClientFactory(orchCfg)
	clFactory.SetCredentialResolver(func(providerID string) (apiKey string, sessionToken string, authMethod string) {
		providerAuthStore.mu.RLock()
		defer providerAuthStore.mu.RUnlock()
		if state, ok := providerAuthStore.state[providerID]; ok && state != nil {
			return state.APIKey, state.SessionToken, state.AuthMethod
		}
		return "", "", ""
	})

	r := &Router{
		cfg:            cfg,
		mux:            http.NewServeMux(),
		tasks:          make(map[string]*types.Task),
		taskProcesses:  make(map[string]*types.TaskProcessInfo),
		strategyRouter: stratRouter,
		budgetTracker:  budgTracker,
		clientFactory:  clFactory,
		console:        newConsoleHub(),
		quota:          quota.NewService(quotaFetchers(cfg.QuotaFetchers), time.Minute),
		dismissedJira:  make(map[string]bool),
		jira:           newJiraSyncState(),
		approvals:      newApprovalHub(),
		docker:         newDockerGate(cfg.Docker),
		toolchain:      cfg.Toolchain,
		prClient:       &pullrequest.Client{},
		stages:         newStagePool(cfg),
		mirrored:       map[string][]byte{},
		stop:           make(chan struct{}),
	}
	if r.toolchain == nil {
		r.toolchain = toolchain.New()
	}
	r.docker.onStats = r.reportContainerStats
	r.connections = newConnectionChecker(r.quota)
	stratRouter.SetAvailability(r.connections.Usable)
	if saved := savedRouterSettings(); saved != nil {
		saved.PriorityChain = withCatalogMeta(saved.PriorityChain) // heal stale prices saved by older builds
		stratRouter.SetSettings(*saved)
	}
	r.initTelemetry()
	r.initGuardrails()
	r.initHooks()
	r.loadWorkflows()
	r.loadBoard()
	r.loadConsoles()
	r.recoverFromMirror()
	r.reconcileIdleRunning()
	r.registerRoutes()
	r.startBackgroundProcessMonitor()
	r.startBoardPersister()
	if cfg.EnableJiraSync {
		r.startJiraSync()
	}
	return r
}

// getOrCreateTaskProcessLocked returns the task's run record. A new record reflects the task as it
// is — it never claims a process, container or resources that do not exist. Caller holds r.mu.
func (r *Router) getOrCreateTaskProcessLocked(taskID string) *types.TaskProcessInfo {
	if proc, ok := r.taskProcesses[taskID]; ok {
		return proc
	}
	proc := &types.TaskProcessInfo{TaskID: taskID, Status: "IDLE", CurrentStep: "Not running", Logs: []string{}}
	if task := r.tasks[taskID]; task != nil {
		proc.ActiveAgent = task.SelectedMethod
		if task.State == types.TaskStateRunning {
			proc.Status = "RUNNING"
			proc.StartedAt = task.UpdatedAt
			proc.CurrentStep = "Running " + stageLabel(task.CurrentStageID)
		}
	}
	r.taskProcesses[taskID] = proc
	return proc
}

func (r *Router) registerRoutes() {
	r.mux.HandleFunc("/api/v1/tasks", r.handleTasks)
	r.mux.HandleFunc("/api/v1/tasks/", r.handleTaskItem)
	r.mux.HandleFunc("/api/v1/tasks/analyze", r.handleTaskAnalyze)
	r.mux.HandleFunc("/api/v1/tasks/{id}/evidence/verify", r.handleEvidenceVerify)
	r.mux.HandleFunc("/api/v1/tasks/{id}/outline", r.handleTaskOutline)
	r.mux.HandleFunc("/api/v1/toolchain", r.handleToolchain)
	r.mux.HandleFunc("/api/v1/toolchain/", r.handleToolchainAction)
	r.mux.HandleFunc("/api/v1/packages", r.handlePackages)
	r.mux.HandleFunc("/api/v1/packages/", r.handlePackageAction)
	r.mux.HandleFunc("/api/v1/providers", r.handleProviders)
	r.mux.HandleFunc("/api/v1/providers/test", r.handleProviderTest)
	r.mux.HandleFunc("/api/v1/providers/usage", r.handleProviderUsage)
	r.mux.HandleFunc("/api/v1/providers/quota", r.handleProviderQuota)
	r.mux.HandleFunc("/api/v1/providers/connection", r.handleProviderConnections)
	r.mux.HandleFunc("/api/v1/router/settings", r.handleRouterSettings)
	r.mux.HandleFunc("/api/v1/router/models", r.handleRegisterModel)
	r.mux.HandleFunc("/api/v1/router/preview", r.handleRouterPreview)
	r.mux.HandleFunc("/api/v1/providers/oauth/initiate", r.handleOAuthInitiate)
	r.mux.HandleFunc("/api/v1/providers/oauth/callback", r.handleOAuthCallback)
	r.mux.HandleFunc("/api/v1/providers/oauth/status", r.handleOAuthStatus)
	r.mux.HandleFunc("/api/v1/providers/oauth/disconnect", r.handleOAuthDisconnect)
	r.mux.HandleFunc("/api/v1/providers/oauth/providers", r.handleOAuthProviders)
	r.mux.HandleFunc("/api/v1/workflows", r.handleWorkflows)
	r.mux.HandleFunc("/api/v1/workflows/", r.handleWorkflowItem)
	r.mux.HandleFunc("/api/v1/hooks", r.handleHooks)
	r.mux.HandleFunc("/api/v1/registries", r.handleRegistries)
	r.mux.HandleFunc("/api/v1/benchmarks", r.handleBenchmarks)
	r.mux.Handle("/api/v1/telemetry/", telemetry.NewHandler(r.telemetry.store))
	r.mux.HandleFunc("/api/v1/router/weights", r.handleRouterWeights)
	r.mux.HandleFunc("/api/v1/router/weights/lock", r.handleRouterWeightLock)
	r.mux.HandleFunc("/api/v1/shadow/status", r.handleShadowStatus)
	r.mux.HandleFunc("/api/v1/artifacts/", r.handleArtifacts)
	r.mux.HandleFunc("/api/v1/projects", r.handleProjects)
	r.mux.HandleFunc("/api/v1/projects/scan", r.handleProjectScan)
	r.mux.HandleFunc("/api/v1/projects/", r.handleProjectItem)
	r.mux.HandleFunc("/api/v1/dependencies/jobs/", r.handleDependencyJob)
	r.mux.HandleFunc("/api/v1/fs/browse", r.handleFSBrowse)
	r.mux.HandleFunc("/api/v1/fs/mkdir", r.handleFSMkdir)

	// Connector endpoints (Modular Catalog, Items, JIRA & Confluence, MCP)
	r.mux.HandleFunc("/api/v1/connectors", r.handleConnectors)
	r.mux.HandleFunc("/api/v1/connectors/catalog", r.handleConnectorCatalog)
	r.mux.HandleFunc("/api/v1/connectors/suggestions", r.handleConnectorSuggestions)
	r.mux.HandleFunc("/api/v1/connectors/mcp-config", r.handleConnectorMCPConfig)
	r.mux.HandleFunc("/api/v1/connectors/items/", r.handleConnectorItemAction)
	r.mux.HandleFunc("/api/v1/connectors/jira", r.handleConnectorJira)
	r.mux.HandleFunc("/api/v1/connectors/confluence", r.handleConnectorConfluence)
	r.mux.HandleFunc("/api/v1/connectors/test", r.handleConnectorTest)
	r.mux.HandleFunc("/api/v1/connectors/jira/issues", r.handleJiraIssues)
	r.mux.HandleFunc("/api/v1/connectors/jira/import", r.handleJiraImport)
	r.mux.HandleFunc("/api/v1/connectors/jira/sync", r.handleJiraSync)
	r.mux.HandleFunc("/api/v1/connectors/jira/sync/run", r.handleJiraSyncRun)
	r.mux.HandleFunc("/api/v1/connectors/jira/sync/dismissed", r.handleJiraSyncDismissed)
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
	// CORS and the loopback-only Host/Origin checks live in localaccess.Middleware, which wraps
	// the daemon's whole mux; no wildcard origin is ever sent from here.
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

// startBackgroundProcessMonitor keeps the runtime duration of RUNNING task processes current.
// It deliberately emits no synthetic log lines or resource figures: the task console shows only
// real activity (see console.go).
func (r *Router) startBackgroundProcessMonitor() {
	go func() {
		ticker := time.NewTicker(4 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			r.mu.Lock()
			for taskID, proc := range r.taskProcesses {
				task, hasTask := r.tasks[taskID]
				if proc.Status != "RUNNING" || (hasTask && task.State != types.TaskStateRunning) {
					continue
				}
				proc.DurationSeconds = int64(time.Since(proc.StartedAt).Seconds())
			}
			r.mu.Unlock()
		}
	}()
}

func quotaFetchers(f map[string]quota.Fetcher) map[string]quota.Fetcher {
	if f == nil {
		return quota.DefaultFetchers()
	}
	return f
}
