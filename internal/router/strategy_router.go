package router

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/config"
)

// RouterMode defines the operational mode for AI model routing.
type RouterMode string

const (
	ModePrioritySequence RouterMode = "priority_sequence" // Custom ordered waterfall priority chain
	ModeBestPractice     RouterMode = "best_practice"      // Heuristic tiered routing (Stage + Complexity)
	ModeCostOptimized    RouterMode = "cost_optimized"     // Lowest cost per token first
	ModeLatencyOptimized RouterMode = "latency_optimized"  // Lowest TTFT / latency first
	ModeRoundRobin       RouterMode = "round_robin"        // Balanced rotation across active providers
)

// PriorityModelItem represents a model entry in the customizable priority chain.
type PriorityModelItem struct {
	ID        string  `json:"id"`
	Provider  string  `json:"provider"`
	Model     string  `json:"model"`
	Name      string  `json:"name"`
	Enabled   bool    `json:"enabled"`
	CostPer1k float64 `json:"cost_per_1k"` // USD cost per 1k tokens
	LatencyMs int     `json:"latency_ms"`  // estimated latency
}

// RouterSettings captures current mode and customized priority chain.
type RouterSettings struct {
	Mode          RouterMode          `json:"mode"`
	PriorityChain []PriorityModelItem `json:"priority_chain"`
}

// RoutingDecision captures the model, method, and token budget chosen by the router.
type RoutingDecision struct {
	Strategy       string   `json:"strategy"` // "best_practice", "priority_sequence", "cost_optimized", "latency_optimized", "round_robin", "custom"
	Model          string   `json:"model"`
	FallbackChain  []string `json:"fallback_chain"` // 9router priority ordered fallback sequence
	Method         string   `json:"method"`
	TokenBudget    int64    `json:"token_budget"`
	Reasoning      string   `json:"reasoning"`
	RequiresDocker bool     `json:"requires_docker"`
}

// Router dispatches requests according to configured strategy.
type Router struct {
	mu            sync.RWMutex
	cfg           *config.OrchestratorConfig
	mode          RouterMode
	priorityChain []PriorityModelItem
	roundRobinIdx int
}

// NewRouter creates a new router instance.
func NewRouter(cfg *config.OrchestratorConfig) *Router {
	if cfg == nil {
		cfg = config.GetDefaultConfig()
	}

	defaultChain := []PriorityModelItem{
		{
			ID:        "item-1",
			Provider:  "claude",
			Model:     "claude-3-5-sonnet-20241022",
			Name:      "Claude 3.5 Sonnet",
			Enabled:   true,
			CostPer1k: 0.003,
			LatencyMs: 142,
		},
		{
			ID:        "item-2",
			Provider:  "antigravity",
			Model:     "gemini-2.0-flash",
			Name:      "Gemini 2.0 Flash",
			Enabled:   true,
			CostPer1k: 0.0001,
			LatencyMs: 98,
		},
		{
			ID:        "item-3",
			Provider:  "chatgpt",
			Model:     "gpt-4o",
			Name:      "OpenAI GPT-4o",
			Enabled:   true,
			CostPer1k: 0.0025,
			LatencyMs: 185,
		},
		{
			ID:        "item-4",
			Provider:  "opencode",
			Model:     "deepseek-coder-v2",
			Name:      "DeepSeek Coder V2 (Local)",
			Enabled:   true,
			CostPer1k: 0.0,
			LatencyMs: 250,
		},
	}

	mode := ModeBestPractice
	if cfg.Router.Strategy == "priority_sequence" {
		mode = ModePrioritySequence
	}

	return &Router{
		cfg:           cfg,
		mode:          mode,
		priorityChain: defaultChain,
	}
}

// SetSettings updates the active mode and custom priority chain.
func (r *Router) SetSettings(settings RouterSettings) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if settings.Mode != "" {
		r.mode = settings.Mode
	}
	if len(settings.PriorityChain) > 0 {
		r.priorityChain = make([]PriorityModelItem, len(settings.PriorityChain))
		copy(r.priorityChain, settings.PriorityChain)
	}
}

// GetSettings retrieves current mode and priority chain.
func (r *Router) GetSettings() RouterSettings {
	r.mu.RLock()
	defer r.mu.RUnlock()
	chainCopy := make([]PriorityModelItem, len(r.priorityChain))
	copy(chainCopy, r.priorityChain)
	return RouterSettings{
		Mode:          r.mode,
		PriorityChain: chainCopy,
	}
}

// Route decides the optimal model and execution method for a given task stage and complexity.
func (r *Router) Route(stageID string, complexity string, repoTypes []string) *RoutingDecision {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check custom rule override first if strategy is "custom"
	if r.cfg.Router.Strategy == "custom" && r.cfg.Router.CustomRules != "" {
		decision := r.routeCustom(stageID, complexity, repoTypes)
		if decision != nil {
			return decision
		}
		return r.routeBestPractice(stageID, complexity, repoTypes)
	}

	// Handle configured router mode
	switch r.mode {
	case ModePrioritySequence:
		return r.routePrioritySequence(stageID, complexity)
	case ModeCostOptimized:
		return r.routeCostOptimized(stageID, complexity)
	case ModeLatencyOptimized:
		return r.routeLatencyOptimized(stageID, complexity)
	case ModeRoundRobin:
		return r.routeRoundRobin(stageID, complexity)
	default:
		return r.routeBestPractice(stageID, complexity, repoTypes)
	}
}

func (r *Router) routePrioritySequence(stageID string, complexity string) *RoutingDecision {
	var enabled []PriorityModelItem
	for _, item := range r.priorityChain {
		if item.Enabled {
			enabled = append(enabled, item)
		}
	}

	if len(enabled) == 0 {
		return r.routeBestPractice(stageID, complexity, nil)
	}

	var fallbackChain []string
	for _, item := range enabled {
		fullModel := item.Model
		if !strings.Contains(fullModel, "/") {
			fullModel = fmt.Sprintf("%s/%s", item.Provider, item.Model)
		}
		fallbackChain = append(fallbackChain, fullModel)
	}

	primary := fallbackChain[0]
	budget := int64(100000)
	if r.cfg.Router.MaxTokenBudget > 0 {
		budget = r.cfg.Router.MaxTokenBudget
	}

	return &RoutingDecision{
		Strategy:       string(ModePrioritySequence),
		Model:          primary,
		FallbackChain:  fallbackChain,
		Method:         "react",
		TokenBudget:    budget,
		Reasoning:      fmt.Sprintf("Priority Sequence: Custom waterfall order starting with %s (%d models in chain)", primary, len(fallbackChain)),
		RequiresDocker: true,
	}
}

func (r *Router) routeCostOptimized(stageID string, complexity string) *RoutingDecision {
	var sortedItems []PriorityModelItem
	for _, item := range r.priorityChain {
		if item.Enabled {
			sortedItems = append(sortedItems, item)
		}
	}

	sort.Slice(sortedItems, func(i, j int) bool {
		return sortedItems[i].CostPer1k < sortedItems[j].CostPer1k
	})

	if len(sortedItems) == 0 {
		return r.routeBestPractice(stageID, complexity, nil)
	}

	var fallbackChain []string
	for _, item := range sortedItems {
		fullModel := item.Model
		if !strings.Contains(fullModel, "/") {
			fullModel = fmt.Sprintf("%s/%s", item.Provider, item.Model)
		}
		fallbackChain = append(fallbackChain, fullModel)
	}

	primary := fallbackChain[0]
	return &RoutingDecision{
		Strategy:       string(ModeCostOptimized),
		Model:          primary,
		FallbackChain:  fallbackChain,
		Method:         "react",
		TokenBudget:    80000,
		Reasoning:      fmt.Sprintf("Cost-Optimized: Lowest cost model %s ($%.4f/1k) prioritized first", primary, sortedItems[0].CostPer1k),
		RequiresDocker: true,
	}
}

func (r *Router) routeLatencyOptimized(stageID string, complexity string) *RoutingDecision {
	var sortedItems []PriorityModelItem
	for _, item := range r.priorityChain {
		if item.Enabled {
			sortedItems = append(sortedItems, item)
		}
	}

	sort.Slice(sortedItems, func(i, j int) bool {
		return sortedItems[i].LatencyMs < sortedItems[j].LatencyMs
	})

	if len(sortedItems) == 0 {
		return r.routeBestPractice(stageID, complexity, nil)
	}

	var fallbackChain []string
	for _, item := range sortedItems {
		fullModel := item.Model
		if !strings.Contains(fullModel, "/") {
			fullModel = fmt.Sprintf("%s/%s", item.Provider, item.Model)
		}
		fallbackChain = append(fallbackChain, fullModel)
	}

	primary := fallbackChain[0]
	return &RoutingDecision{
		Strategy:       string(ModeLatencyOptimized),
		Model:          primary,
		FallbackChain:  fallbackChain,
		Method:         "react",
		TokenBudget:    100000,
		Reasoning:      fmt.Sprintf("Latency-Optimized: Fastest responding model %s (%dms) prioritized first", primary, sortedItems[0].LatencyMs),
		RequiresDocker: true,
	}
}

func (r *Router) routeRoundRobin(stageID string, complexity string) *RoutingDecision {
	var enabled []PriorityModelItem
	for _, item := range r.priorityChain {
		if item.Enabled {
			enabled = append(enabled, item)
		}
	}

	if len(enabled) == 0 {
		return r.routeBestPractice(stageID, complexity, nil)
	}

	idx := r.roundRobinIdx % len(enabled)
	r.roundRobinIdx++

	// Put the chosen model first, then the remaining
	var fallbackChain []string
	chosen := enabled[idx]
	chosenModel := chosen.Model
	if !strings.Contains(chosenModel, "/") {
		chosenModel = fmt.Sprintf("%s/%s", chosen.Provider, chosen.Model)
	}
	fallbackChain = append(fallbackChain, chosenModel)

	for i, item := range enabled {
		if i != idx {
			m := item.Model
			if !strings.Contains(m, "/") {
				m = fmt.Sprintf("%s/%s", item.Provider, item.Model)
			}
			fallbackChain = append(fallbackChain, m)
		}
	}

	return &RoutingDecision{
		Strategy:       string(ModeRoundRobin),
		Model:          chosenModel,
		FallbackChain:  fallbackChain,
		Method:         "react",
		TokenBudget:    100000,
		Reasoning:      fmt.Sprintf("Round-Robin: Distributed request to slot #%d (%s) for load balancing", idx+1, chosenModel),
		RequiresDocker: true,
	}
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
