package router

import (
	"strings"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/config"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// TaskProfiler analyzes user requests, resolves target workflows, and generates TaskProfiles.
type TaskProfiler struct {
	router      *Router
	discovery   *RepoDiscoveryEngine
	config      *config.OrchestratorConfig
}

// NewTaskProfiler creates a new profiler.
func NewTaskProfiler(cfg *config.OrchestratorConfig, workspaceRoot string) *TaskProfiler {
	if cfg == nil {
		cfg = config.GetDefaultConfig()
	}
	return &TaskProfiler{
		router:    NewRouter(cfg),
		discovery: NewRepoDiscoveryEngine(workspaceRoot),
		config:    cfg,
	}
}

// ProfileTask classifies request complexity, discovers coupled repositories, and resolves routing.
func (p *TaskProfiler) ProfileTask(title string, description string, explicitWorkflowID string) (*types.TaskProfile, error) {
	// 1. Discover repositories
	repos, _ := p.discovery.Discover()
	impactedRepos := p.discovery.FindImpactedRepos(repos, title+" "+description)

	// 2. Assess complexity
	complexity := p.AssessComplexity(title, description, impactedRepos)

	// 3. Resolve Workflow ID
	workflowID := explicitWorkflowID
	if workflowID == "" {
		if strings.Contains(strings.ToLower(title), "hotfix") || strings.Contains(strings.ToLower(description), "emergency patch") {
			workflowID = "hotfix-fast-track"
		} else if strings.Contains(strings.ToLower(title), "grpc") || strings.Contains(strings.ToLower(description), "openapi contract") {
			workflowID = "microservice-api"
		} else if p.config.ActiveSDLC != "" {
			workflowID = p.config.ActiveSDLC
		} else {
			workflowID = "general-ai-sdlc"
		}
	}

	// 4. Determine initial routing decision (task type lets calibrated weights adjust tiering)
	taskType := ClassifyTaskType(title, description)
	routing := p.router.RouteForTask("INTAKE_PRD", complexity, taskType, impactedRepos)

	return &types.TaskProfile{
		WorkflowID:       workflowID,
		Complexity:       complexity,
		TaskType:         taskType,
		IdentifiedRepos:  impactedRepos,
		RecommendedModel: routing.Model,
		Method:           routing.Method,
		TokenBudget:      routing.TokenBudget,
		RequiresDocker:   routing.RequiresDocker,
	}, nil
}

// Router exposes the profiler's strategy router (e.g. to install a TierAdvisor).
func (p *TaskProfiler) Router() *Router { return p.router }

// taskTypeRules are checked in order; the first category with a matching keyword wins.
var taskTypeRules = []struct {
	category string
	keywords []string
}{
	{"architecture", []string{"architecture", "architectural", "system design", "design new", "epic", "event-sourcing", "rearchitect"}},
	{"migration", []string{"migration", "migrate", "schema change", "upgrade to"}},
	{"docs", []string{"readme", "typo", "documentation", "docs", "changelog", "comment"}},
	{"bugfix", []string{"bug", "fix", "crash", "regression", "hotfix", "error", "null pointer"}},
	{"crud", []string{"crud", "endpoint", "rest api", "create ", "update ", "delete ", "list ", "form"}},
	{"refactor", []string{"refactor", "cleanup", "clean up", "rename"}},
	{"test", []string{"test", "coverage", "atdd", "e2e"}},
	{"ui", []string{"ui", "css", "layout", "component", "style"}},
}

// ClassifyTaskType maps a request to a recurring task category used by the feedback loop.
func ClassifyTaskType(title string, description string) string {
	combined := " " + strings.ToLower(title+" "+description) + " "
	for _, rule := range taskTypeRules {
		for _, kw := range rule.keywords {
			if strings.Contains(combined, kw) {
				return rule.category
			}
		}
	}
	return "general"
}

// AssessComplexity grades a task from keywords and repository count (see HeuristicComplexity).
func (p *TaskProfiler) AssessComplexity(title string, description string, impactedRepos []string) string {
	return HeuristicComplexity(title, description, impactedRepos).Complexity
}
