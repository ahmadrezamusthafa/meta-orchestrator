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
	ModeBestPractice     RouterMode = "best_practice"     // Heuristic tiered routing (Stage + Complexity)
	ModeCostOptimized    RouterMode = "cost_optimized"    // Lowest cost per token first
	ModeLatencyOptimized RouterMode = "latency_optimized" // Lowest TTFT / latency first
	ModeRoundRobin       RouterMode = "round_robin"       // Balanced rotation across active providers
)

// PriorityModelItem represents a model entry in the customizable priority chain. The chain is the
// operator's allow-list: every routing mode picks from its enabled items (see bindToPool).
type PriorityModelItem struct {
	ID        string   `json:"id"`
	Provider  string   `json:"provider"`
	Model     string   `json:"model"`
	Name      string   `json:"name"`
	Enabled   bool     `json:"enabled"`
	CostPer1k float64  `json:"cost_per_1k"`     // USD cost per 1k tokens
	LatencyMs int      `json:"latency_ms"`      // estimated latency
	Tiers     []string `json:"tiers,omitempty"` // tier tags; empty infers them from the model family
}

// RouterSettings captures current mode and customized priority chain.
type RouterSettings struct {
	Mode          RouterMode          `json:"mode"`
	PriorityChain []PriorityModelItem `json:"priority_chain"`
}

// RoutingDecision captures the model, method, and token budget chosen by the router.
type RoutingDecision struct {
	Strategy       string           `json:"strategy"` // "best_practice", "priority_sequence", "cost_optimized", "latency_optimized", "round_robin", "custom"
	Model          string           `json:"model"`
	Tier           string           `json:"tier,omitempty"`    // "tier1".."tier3" when the decision came from tier mapping
	RuleID         string           `json:"rule_id,omitempty"` // matched custom rule identifier
	FallbackChain  []string         `json:"fallback_chain"`    // 9router priority ordered fallback sequence
	Method         string           `json:"method"`
	TokenBudget    int64            `json:"token_budget"`
	Reasoning      string           `json:"reasoning"`
	RequiresDocker bool             `json:"requires_docker"`
	Suggestion     *ModelSuggestion `json:"suggestion,omitempty"` // recommended model the chain did not allow
}

// TierAdvisor supplies calibrated tier overrides for best-practice routing (see router/feedback).
// currentTier is the tier best practice picked; ok=false keeps it.
type TierAdvisor interface {
	AdviseTier(taskType, complexity, stageID, currentTier string) (tier string, reason string, ok bool)
}

// MethodAdvisor supplies the empirically optimal method (and tier) per stage and complexity,
// e.g. the shadow benchmark's best_methods_matrix. ok=false keeps best practice.
type MethodAdvisor interface {
	// model is the measured winner ("provider/model"), or "" to use the tier's configured model.
	AdviseMethod(stageID, complexity string) (method, tier, model, reason string, ok bool)
}

// Router dispatches requests according to configured strategy.
type Router struct {
	mu            sync.RWMutex
	cfg           *config.OrchestratorConfig
	mode          RouterMode
	priorityChain []PriorityModelItem
	roundRobinIdx int
	advisor       TierAdvisor
	methodAdvisor MethodAdvisor
	available     ProviderAvailability
}

// DefaultPriorityChain is the chain a fresh install starts with (and "Reset Defaults" restores).
func DefaultPriorityChain() []PriorityModelItem {
	return []PriorityModelItem{
		{ID: "item-1", Provider: "claude", Model: "claude-3-5-sonnet-20241022", Name: "Claude 3.5 Sonnet",
			Enabled: true, CostPer1k: 0.003, LatencyMs: 142, Tiers: []string{TierReasoning, TierCodeGen}},
		{ID: "item-2", Provider: "antigravity", Model: "gemini-2.0-flash", Name: "Gemini 2.0 Flash",
			Enabled: true, CostPer1k: 0.0001, LatencyMs: 98, Tiers: []string{TierCodeGen, TierLogParse}},
		{ID: "item-3", Provider: "chatgpt", Model: "gpt-4o", Name: "OpenAI GPT-4o",
			Enabled: true, CostPer1k: 0.0025, LatencyMs: 185, Tiers: []string{TierReasoning, TierCodeGen}},
		{ID: "item-4", Provider: "opencode", Model: "deepseek-coder-v2", Name: "DeepSeek Coder V2 (Local)",
			Enabled: true, CostPer1k: 0.0, LatencyMs: 250, Tiers: []string{TierCodeGen}},
	}
}

// NewRouter creates a new router instance.
func NewRouter(cfg *config.OrchestratorConfig) *Router {
	if cfg == nil {
		cfg = config.GetDefaultConfig()
	}

	mode := ModeBestPractice
	if cfg.Router.Strategy == "priority_sequence" {
		mode = ModePrioritySequence
	}

	return &Router{
		cfg:           cfg,
		mode:          mode,
		priorityChain: DefaultPriorityChain(),
	}
}

// SetSettings updates the active mode and custom priority chain. A nil chain keeps the current
// one; an empty (non-nil) chain clears it, which leaves best practice unrestricted.
func (r *Router) SetSettings(settings RouterSettings) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if settings.Mode != "" {
		r.mode = settings.Mode
	}
	if settings.PriorityChain != nil {
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

// SetTierAdvisor installs the dynamic weight calibrator used by best-practice routing.
func (r *Router) SetTierAdvisor(a TierAdvisor) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.advisor = a
}

// SetMethodAdvisor installs the benchmark-driven method matrix used by best-practice routing.
func (r *Router) SetMethodAdvisor(a MethodAdvisor) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.methodAdvisor = a
}

// SetAvailability installs the provider health check used to skip disconnected providers.
func (r *Router) SetAvailability(a ProviderAvailability) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.available = a
}

// Route decides the optimal model and execution method for a given task stage and complexity.
func (r *Router) Route(stageID string, complexity string, repoTypes []string) *RoutingDecision {
	return r.RouteForTask(stageID, complexity, "", repoTypes)
}

// RouteForTask is Route with the task category, which lets calibrated weights adjust
// best-practice tiering for recurring task types.
func (r *Router) RouteForTask(stageID string, complexity string, taskType string, repoTypes []string) *RoutingDecision {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.route(r.mode, r.priorityChain, stageID, complexity, taskType, repoTypes, true)
}

// PreviewStage is one representative stage shown in the routing preview.
type PreviewStage struct {
	Label      string `json:"label"`
	Stage      string `json:"stage"`
	Complexity string `json:"complexity"`
}

// PreviewStages covers each distinct best-practice branch.
var PreviewStages = []PreviewStage{
	{Label: "PRD & RFC", Stage: "INTAKE_PRD", Complexity: "MEDIUM"},
	{Label: "Task breakdown", Stage: "TASK_BREAKDOWN", Complexity: "MEDIUM"},
	{Label: "ATDD red phase", Stage: "ATDD_RED_PHASE", Complexity: "MEDIUM"},
	{Label: "Implementation (standard)", Stage: "IMPLEMENTATION_GREEN", Complexity: "MEDIUM"},
	{Label: "Implementation (complex)", Stage: "IMPLEMENTATION_GREEN", Complexity: "HIGH"},
	{Label: "E2E / UAT verification", Stage: "E2E_AUTOMATION", Complexity: "MEDIUM"},
}

// RoutePreview is what the router would decide for one preview stage.
type RoutePreview struct {
	PreviewStage
	Decision *RoutingDecision `json:"decision"`
}

// TierAssignment is the model each tier resolves to under the current chain.
type TierAssignment struct {
	Tier        string           `json:"tier"`
	Recommended string           `json:"recommended"`
	Model       string           `json:"model"`
	Suggestion  *ModelSuggestion `json:"suggestion,omitempty"`
}

// Preview routes the preview stages under settings (or the live settings when nil) without
// changing router state, so the UI can show the effect of an unsaved chain.
func (r *Router) Preview(settings *RouterSettings) ([]RoutePreview, []TierAssignment) {
	r.mu.Lock()
	defer r.mu.Unlock()
	mode, chain := r.mode, r.priorityChain
	if settings != nil {
		if settings.Mode != "" {
			mode = settings.Mode
		}
		if settings.PriorityChain != nil {
			chain = settings.PriorityChain
		}
	}
	rows := make([]RoutePreview, 0, len(PreviewStages))
	for _, s := range PreviewStages {
		rows = append(rows, RoutePreview{PreviewStage: s, Decision: r.route(mode, chain, s.Stage, s.Complexity, "", nil, false)})
	}
	var tiers []TierAssignment
	for _, t := range []string{TierReasoning, TierCodeGen, TierLogParse} {
		d := &RoutingDecision{Model: r.modelForTier(t), Tier: t}
		r.bindToPool(d, chain)
		tiers = append(tiers, TierAssignment{Tier: t, Recommended: r.modelForTier(t), Model: d.Model, Suggestion: d.Suggestion})
	}
	return rows, tiers
}

// route is the decision core. Caller holds r.mu. advance=false leaves round-robin state untouched.
func (r *Router) route(mode RouterMode, chain []PriorityModelItem, stageID, complexity, taskType string, repoTypes []string, advance bool) *RoutingDecision {
	// Check custom rule override first if strategy is "custom"
	if r.cfg.Router.Strategy == "custom" && r.cfg.Router.CustomRules != "" {
		if decision := r.routeCustom(stageID, complexity, repoTypes); decision != nil {
			candidates, _ := r.usableItems(enabledItems(chain))
			decision.FallbackChain = withPrimary(decision.Model, candidates)
			return decision
		}
		return r.routeRecommended(chain, stageID, complexity, taskType, repoTypes)
	}

	enabled := enabledItems(chain)
	if len(enabled) == 0 || mode == ModeBestPractice {
		return r.routeRecommended(chain, stageID, complexity, taskType, repoTypes)
	}
	candidates, degraded := r.usableItems(enabled)

	var d *RoutingDecision
	switch mode {
	case ModePrioritySequence:
		d = r.routeOrdered(ModePrioritySequence, candidates, 0, func(first PriorityModelItem, n int) string {
			return fmt.Sprintf("Priority Sequence: Custom waterfall order starting with %s (%d models in chain)", first.FullModel(), n)
		})
	case ModeCostOptimized:
		sorted := append([]PriorityModelItem(nil), candidates...)
		sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].CostPer1k < sorted[j].CostPer1k })
		d = r.routeOrdered(ModeCostOptimized, sorted, 80000, func(first PriorityModelItem, _ int) string {
			return fmt.Sprintf("Cost-Optimized: Lowest cost model %s ($%.4f/1k) prioritized first", first.FullModel(), first.CostPer1k)
		})
	case ModeLatencyOptimized:
		sorted := append([]PriorityModelItem(nil), candidates...)
		sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].LatencyMs < sorted[j].LatencyMs })
		d = r.routeOrdered(ModeLatencyOptimized, sorted, 100000, func(first PriorityModelItem, _ int) string {
			return fmt.Sprintf("Latency-Optimized: Fastest responding model %s (%dms) prioritized first", first.FullModel(), first.LatencyMs)
		})
	case ModeRoundRobin:
		idx := r.roundRobinIdx % len(candidates)
		if advance {
			r.roundRobinIdx++
		}
		rotated := append([]PriorityModelItem{candidates[idx]}, append(append([]PriorityModelItem(nil), candidates[:idx]...), candidates[idx+1:]...)...)
		d = r.routeOrdered(ModeRoundRobin, rotated, 100000, func(first PriorityModelItem, _ int) string {
			return fmt.Sprintf("Round-Robin: Distributed request to slot #%d (%s) for load balancing", idx+1, first.FullModel())
		})
	default:
		return r.routeRecommended(chain, stageID, complexity, taskType, repoTypes)
	}
	if degraded {
		d.Reasoning += " | Warning: no provider in your chain is verified as connected; trying them anyway"
	}
	return d
}

// routeRecommended is best-practice tiering (plus advisors) bound to the operator's chain.
func (r *Router) routeRecommended(chain []PriorityModelItem, stageID, complexity, taskType string, repoTypes []string) *RoutingDecision {
	d := r.applyAdvisor(r.routeBestPractice(stageID, complexity, repoTypes), stageID, complexity, taskType)
	r.bindToPool(d, chain)
	return d
}

// routeOrdered builds a chain-mode decision whose primary is items[0]. budget 0 uses the configured cap.
func (r *Router) routeOrdered(mode RouterMode, items []PriorityModelItem, budget int64, reasoning func(first PriorityModelItem, n int) string) *RoutingDecision {
	if budget == 0 {
		budget = 100000
		if r.cfg.Router.MaxTokenBudget > 0 {
			budget = r.cfg.Router.MaxTokenBudget
		}
	}
	fallback := chainModels(items)
	return &RoutingDecision{
		Strategy:       string(mode),
		Model:          fallback[0],
		FallbackChain:  fallback,
		Method:         "react",
		TokenBudget:    budget,
		Reasoning:      reasoning(items[0], len(items)),
		RequiresDocker: true,
	}
}

// modelForTier maps a tier to its configured model.
func (r *Router) modelForTier(tier string) string {
	switch tier {
	case TierReasoning:
		return r.cfg.ModelTiers.Tier1Reasoning
	case TierCodeGen:
		return r.cfg.ModelTiers.Tier2CodeGen
	case TierLogParse:
		return r.cfg.ModelTiers.Tier3LogParse
	}
	return ""
}

// applyAdvisor layers the benchmark method matrix and then calibrated tier weights
// (including admin locks, which therefore win) over a best-practice decision. It only changes
// the recommendation; bindToPool then fits it to the chain.
func (r *Router) applyAdvisor(d *RoutingDecision, stageID, complexity, taskType string) *RoutingDecision {
	if d == nil {
		return d
	}
	if r.methodAdvisor != nil {
		if method, tier, model, reason, ok := r.methodAdvisor.AdviseMethod(stageID, complexity); ok {
			d.Method = method
			// Prefer the benchmarked model itself; bindToPool falls back to the tier when the
			// operator's chain does not include it.
			if model == "" {
				model = r.modelForTier(tier)
			}
			if model != "" {
				d.Tier = tier
				d.Model = model
			}
			d.Reasoning = fmt.Sprintf("%s | %s", d.Reasoning, reason)
		}
	}
	if r.advisor == nil || taskType == "" {
		return d
	}
	tier, reason, ok := r.advisor.AdviseTier(taskType, strings.ToUpper(complexity), stageID, d.Tier)
	if !ok || tier == d.Tier {
		return d
	}
	model := r.modelForTier(tier)
	if model == "" {
		return d
	}
	d.Tier = tier
	d.Model = model
	d.Reasoning = fmt.Sprintf("%s | Calibrated: %s", d.Reasoning, reason)
	return d
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
	var tier string
	var method string
	var budget int64
	var reasoning string
	requiresDocker := false

	switch normStage {
	case "INTAKE_PRD", "TECH_DOC_RFC", "CONTRACT_SPEC":
		model, tier = r.cfg.ModelTiers.Tier1Reasoning, TierReasoning
		method = "bmad"
		budget = 120000
		reasoning = "Best practice: Tier 1 high-reasoning model paired with BMAD product/architecture methodology"

	case "REPO_DISCOVERY", "TASK_BREAKDOWN":
		model, tier = r.cfg.ModelTiers.Tier1Reasoning, TierReasoning
		method = "supervisor"
		budget = 80000
		reasoning = "Best practice: Tier 1 model with Supervisor role for dependency mapping and atomic task decomposition"

	case "ATDD_RED_PHASE", "MOCK_ATDD":
		model, tier = r.cfg.ModelTiers.Tier1Reasoning, TierReasoning
		method = "bmad"
		budget = 100000
		requiresDocker = true
		reasoning = "Best practice: Tier 1 model + BMAD QA role for AST-grounded Red Phase test generation"

	case "IMPLEMENTATION_GREEN", "PATCH_IMPLEMENTATION", "CODEGEN_IMPLEMENT":
		if normComplexity == "HIGH" || normComplexity == "CRITICAL" {
			model, tier = r.cfg.ModelTiers.Tier1Reasoning, TierReasoning
			method = "bmad"
			budget = 200000
			reasoning = fmt.Sprintf("Best practice: Complex task (%s) routed to Tier 1 with BMAD developer/QA pairing", normComplexity)
		} else {
			model, tier = r.cfg.ModelTiers.Tier2CodeGen, TierCodeGen
			method = "react"
			budget = 100000
			reasoning = fmt.Sprintf("Best practice: Standard task (%s) routed to Tier 2 with fast ReAct iteration", normComplexity)
		}
		requiresDocker = true

	case "E2E_AUTOMATION", "UAT_EVIDENCE", "VERIFY_REGRESSION", "CONTRACT_VERIFY":
		model, tier = r.cfg.ModelTiers.Tier2CodeGen, TierCodeGen
		method = "superpower"
		budget = 90000
		requiresDocker = true
		reasoning = "Best practice: Browser/terminal verification routed to Superpower automation harness"

	default:
		// Default fallback
		model, tier = r.cfg.ModelTiers.Tier2CodeGen, TierCodeGen
		method = "react"
		budget = 50000
		reasoning = "Default best-practice fallback for custom stage"
	}

	if r.cfg.Router.MaxTokenBudget > 0 && budget > r.cfg.Router.MaxTokenBudget {
		budget = r.cfg.Router.MaxTokenBudget
	}

	return &RoutingDecision{
		Strategy:       "best_practice",
		Model:          model,
		Tier:           tier,
		FallbackChain:  []string{model},
		Method:         method,
		TokenBudget:    budget,
		Reasoning:      reasoning,
		RequiresDocker: requiresDocker,
	}
}

func (r *Router) routeCustom(stageID string, complexity string, repoTypes []string) *RoutingDecision {
	rules := r.cfg.Router.CustomRules

	// Simple declarative rule matching (e.g. "stage:ATDD_RED_PHASE -> model:openai/gpt-4o,method:bmad")
	lines := strings.Split(rules, "\n")
	for lineNo, line := range lines {
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
				RuleID:         fmt.Sprintf("rule-%d", lineNo+1),
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
