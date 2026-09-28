package router

import (
	"fmt"
	"strings"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/config"
)

// RoutingDecision captures the model, method, and token budget chosen by the router.
type RoutingDecision struct {
	Strategy       string   `json:"strategy"` // "best_practice" or "custom"
	Model          string   `json:"model"`
	FallbackChain  []string `json:"fallback_chain"` // 9router priority ordered fallback sequence
	Method         string   `json:"method"`
	TokenBudget    int64    `json:"token_budget"`
	Reasoning      string   `json:"reasoning"`
	RequiresDocker bool     `json:"requires_docker"`
}

// Router dispatches requests according to configured strategy.
type Router struct {
	cfg *config.OrchestratorConfig
}

// NewRouter creates a new router instance.
func NewRouter(cfg *config.OrchestratorConfig) *Router {
	if cfg == nil {
		cfg = config.GetDefaultConfig()
	}
	return &Router{cfg: cfg}
}

// Route decides the optimal model and execution method for a given task stage and complexity.
func (r *Router) Route(stageID string, complexity string, repoTypes []string) *RoutingDecision {
	strategy := r.cfg.Router.Strategy
	if strategy == "custom" && r.cfg.Router.CustomRules != "" {
		decision := r.routeCustom(stageID, complexity, repoTypes)
		if decision != nil {
			return decision
		}
	}

	return r.routeBestPractice(stageID, complexity, repoTypes)
}

func (r *Router) routeBestPractice(stageID string, complexity string, repoTypes []string) *RoutingDecision {
	normComplexity := strings.ToUpper(complexity)
	if normComplexity == "" {
		normComplexity = "MEDIUM"
	}

	normStage := strings.ToUpper(stageID)
	switch normStage {
	case "PRD_DISCOVERY":
		normStage = "INTAKE_PRD"
	case "ATDD_CREATION":
		normStage = "ATDD_RED_PHASE"
	case "TASK_IMPLEMENTATION":
		normStage = "IMPLEMENTATION_GREEN"
	case "E2E_VALIDATION", "UAT_VERIFICATION":
		normStage = "E2E_AUTOMATION"
	case "SIGNOFF_MERGE":
		normStage = "CONTRACT_VERIFY"
	}

	var model string
	var method string
	var budget int64
	var reasoning string
	requiresDocker := false

	switch normStage {
	case "INTAKE_PRD", "TECH_DOC_RFC", "CONTRACT_SPEC":
		model = r.cfg.ModelTiers.Tier1Reasoning
		method = "bmad"
		budget = 120000
		reasoning = "Best practice: Tier 1 high-reasoning model paired with BMAD product/architecture methodology"

	case "REPO_DISCOVERY", "TASK_BREAKDOWN":
		model = r.cfg.ModelTiers.Tier1Reasoning
		method = "supervisor"
		budget = 80000
		reasoning = "Best practice: Tier 1 model with Supervisor role for dependency mapping and atomic task decomposition"

	case "ATDD_RED_PHASE", "MOCK_ATDD":
		model = r.cfg.ModelTiers.Tier1Reasoning
		method = "bmad"
		budget = 100000
		requiresDocker = true
		reasoning = "Best practice: Tier 1 model + BMAD QA role for AST-grounded Red Phase test generation"

	case "IMPLEMENTATION_GREEN", "PATCH_IMPLEMENTATION", "CODEGEN_IMPLEMENT":
		if normComplexity == "HIGH" || normComplexity == "CRITICAL" {
			model = r.cfg.ModelTiers.Tier1Reasoning
			method = "bmad"
			budget = 200000
			reasoning = fmt.Sprintf("Best practice: Complex task (%s) routed to Tier 1 with BMAD developer/QA pairing", normComplexity)
		} else {
			model = r.cfg.ModelTiers.Tier2CodeGen
			method = "react"
			budget = 100000
			reasoning = fmt.Sprintf("Best practice: Standard task (%s) routed to Tier 2 with fast ReAct iteration", normComplexity)
		}
		requiresDocker = true

	case "E2E_AUTOMATION", "UAT_EVIDENCE", "VERIFY_REGRESSION", "CONTRACT_VERIFY":
		model = r.cfg.ModelTiers.Tier2CodeGen
		method = "superpower"
		budget = 90000
		requiresDocker = true
		reasoning = "Best practice: Browser/terminal verification routed to Superpower automation harness"

	default:
		// Default fallback
		model = r.cfg.ModelTiers.Tier2CodeGen
		method = "react"
		budget = 50000
		reasoning = "Default best-practice fallback for custom stage"
	}

	if r.cfg.Router.MaxTokenBudget > 0 && budget > r.cfg.Router.MaxTokenBudget {
		budget = r.cfg.Router.MaxTokenBudget
	}

	fallbackChain := r.buildFallbackChain(model)

	return &RoutingDecision{
		Strategy:       "best_practice",
		Model:          model,
		FallbackChain:  fallbackChain,
		Method:         method,
		TokenBudget:    budget,
		Reasoning:      reasoning,
		RequiresDocker: requiresDocker,
	}
}

// buildFallbackChain creates a prioritized sequence of fallback models (9router pattern).
func (r *Router) buildFallbackChain(primary string) []string {
	allCandidates := []string{
		primary,
		"claude/claude-3-5-sonnet-20241022",
		"antigravity/gemini-2.0-flash",
		"openai/gpt-4o",
		"opencode/deepseek-coder-v2",
	}

	seen := make(map[string]bool)
	var chain []string
	for _, c := range allCandidates {
		if c != "" && !seen[c] {
			seen[c] = true
			chain = append(chain, c)
		}
	}
	return chain
}

func (r *Router) routeCustom(stageID string, complexity string, repoTypes []string) *RoutingDecision {
	rules := r.cfg.Router.CustomRules

	// Simple declarative rule matching (e.g. "stage:ATDD_RED_PHASE -> model:openai/gpt-4o,method:bmad")
	lines := strings.Split(rules, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}

		parts := strings.Split(line, "->")
		if len(parts) != 2 {
			continue
		}

		condition := strings.TrimSpace(parts[0])
		action := strings.TrimSpace(parts[1])

		if strings.Contains(condition, "stage:"+stageID) || strings.Contains(condition, "complexity:"+strings.ToLower(complexity)) {
			decision := &RoutingDecision{
				Strategy:       "custom",
				Model:          r.cfg.ModelTiers.Tier1Reasoning,
				Method:         "react",
				TokenBudget:    r.cfg.Router.MaxTokenBudget,
				Reasoning:      fmt.Sprintf("Matched custom user rule: %s", line),
				RequiresDocker: true,
			}

			// Parse action key-values
			for _, item := range strings.Split(action, ",") {
				kv := strings.Split(strings.TrimSpace(item), ":")
				if len(kv) == 2 {
					switch kv[0] {
					case "model":
						decision.Model = kv[1]
					case "method":
						decision.Method = kv[1]
					}
				}
			}
			return decision
		}
	}

	return nil
}
