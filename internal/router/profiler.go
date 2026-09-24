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

	// 4. Determine initial routing decision
	routing := p.router.Route("INTAKE_PRD", complexity, impactedRepos)

	return &types.TaskProfile{
		WorkflowID:       workflowID,
		Complexity:       complexity,
		IdentifiedRepos:  impactedRepos,
		RecommendedModel: routing.Model,
		Method:           routing.Method,
		TokenBudget:      routing.TokenBudget,
		RequiresDocker:   routing.RequiresDocker,
	}, nil
}

// AssessComplexity evaluates title, scope, and repo impact to assign a complexity tier.
func (p *TaskProfiler) AssessComplexity(title string, description string, impactedRepos []string) string {
	combined := strings.ToLower(title + " " + description)

	if strings.Contains(combined, "refactor") ||
		strings.Contains(combined, "migration") ||
		strings.Contains(combined, "breaking change") ||
		len(impactedRepos) > 2 {
		return "HIGH"
	}

	if strings.Contains(combined, "typo") ||
		strings.Contains(combined, "readme") ||
		strings.Contains(combined, "css tweak") ||
		strings.Contains(combined, "color change") {
		return "LOW"
	}

	return "MEDIUM"
}
