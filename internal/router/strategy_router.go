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
	// MethodSource says where Method came from: "policy" (stage × complexity best practice),
	// "benchmark" (a measured shadow-benchmark winner), "rule" (a custom rule) or "user" (the task's plan).
	MethodSource string `json:"method_source,omitempty"`
	// TierSource says where the model choice came from: "policy", "benchmark", "calibrated"
	// (learned from run history), "mode" (chain order of a non-tiered mode), "rule" or "user".
	TierSource string `json:"tier_source,omitempty"`
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
		// One Claude model per tier, so each tier routes to its own model by default.
		{ID: "item-1", Provider: "claude", Model: "claude-opus-5-5", Name: "Claude Opus 5.5",
			Enabled: true, CostPer1k: 0.004, LatencyMs: 250, Tiers: []string{TierReasoning}},
		{ID: "item-5", Provider: "claude", Model: "claude-sonnet-5-5", Name: "Claude Sonnet 5.5",
			Enabled: true, CostPer1k: 0.002, LatencyMs: 140, Tiers: []string{TierCodeGen}},
		{ID: "item-6", Provider: "claude", Model: "claude-haiku-4-5", Name: "Claude Haiku 4.5",
			Enabled: true, CostPer1k: 0.001, LatencyMs: 75, Tiers: []string{TierLogParse}},
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
			decision.MethodSource, decision.TierSource = "rule", "rule"
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
	// The mode only orders the models; the method and budget still follow the stage and complexity.
	policy := r.applyMethodAdvisor(r.routeBestPractice(stageID, complexity, repoTypes), stageID, complexity)
	d.Method, d.MethodSource, d.TokenBudget, d.RequiresDocker = policy.Method, policy.MethodSource, policy.TokenBudget, policy.RequiresDocker
	d.TierSource = "mode"
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
		MethodSource:   "policy",
		TierSource:     "mode",
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
	d = r.applyMethodAdvisor(d, stageID, complexity)
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
	d.TierSource = "calibrated"
	d.Reasoning = fmt.Sprintf("%s | Calibrated: %s", d.Reasoning, reason)
	return d
}

// applyMethodAdvisor replaces the policy method (and model) with the benchmark winner when the
// benchmark matrix has an applicable measurement for this stage and complexity.
func (r *Router) applyMethodAdvisor(d *RoutingDecision, stageID, complexity string) *RoutingDecision {
	if r.methodAdvisor != nil {
		if method, tier, model, reason, ok := r.methodAdvisor.AdviseMethod(stageID, complexity); ok {
			d.Method = method
			// Prefer the benchmarked model itself; bindToPool falls back to the tier when the
			// operator's chain does not include it.
			if model == "" {
				model = r.modelForTier(tier)
			}
			d.MethodSource = "benchmark"
			if model != "" {
				d.Tier = tier
				d.Model = model
				d.TierSource = "benchmark"
			}
			d.Reasoning = fmt.Sprintf("%s | %s", d.Reasoning, reason)
		}
	}
	return d
}

// stageClass groups SDLC stages that share a routing policy.
type stageClass int

const (
	classPlanning  stageClass = iota // PRD, RFC, ATDD: reasoning-heavy documents
	classDecompose                   // repo discovery, task breakdown: dependency mapping
	classImplement                   // code changes
	classVerify                      // E2E, UAT, sign-off
	classOther
)

// canonicalRouterStage maps every stage id spelling (workflow ids, legacy FSM ids) onto one name.
func canonicalRouterStage(stageID string) string {
	switch s := strings.ToUpper(strings.TrimSpace(stageID)); s {
	case "PRD_DISCOVERY", "INTAKE_PRD":
		return "INTAKE_PRD"
	case "TECHDOC_RFC", "TECH_DOC_RFC", "CONTRACT_SPEC":
		return "TECH_DOC_RFC"
	case "ATDD_CREATION", "ATDD_RED_PHASE", "MOCK_ATDD", "RED_VERIFICATION":
		return "ATDD_RED_PHASE"
	case "TASK_IMPLEMENTATION", "IMPLEMENTATION_GREEN", "PATCH_IMPLEMENTATION", "CODEGEN_IMPLEMENT":
		return "IMPLEMENTATION_GREEN"
	case "E2E_VALIDATION", "UAT_VERIFICATION", "E2E_AUTOMATION", "UAT_EVIDENCE", "VERIFY_REGRESSION":
		return "E2E_AUTOMATION"
	case "SIGNOFF_MERGE", "CONTRACT_VERIFY":
		return "CONTRACT_VERIFY"
	default:
		return s
	}
}

func classOf(canonical string) stageClass {
	switch canonical {
	case "INTAKE_PRD", "TECH_DOC_RFC", "ATDD_RED_PHASE":
		return classPlanning
	case "REPO_DISCOVERY", "TASK_BREAKDOWN":
		return classDecompose
	case "IMPLEMENTATION_GREEN":
		return classImplement
	case "E2E_AUTOMATION", "CONTRACT_VERIFY":
		return classVerify
	}
	return classOther
}

// NormalizeComplexity maps a complexity label onto LOW, MEDIUM, HIGH or SYSTEM (CRITICAL → HIGH).
func NormalizeComplexity(c string) string {
	switch strings.ToUpper(strings.TrimSpace(c)) {
	case "LOW":
		return "LOW"
	case "HIGH", "CRITICAL":
		return "HIGH"
	case "SYSTEM":
		return "SYSTEM"
	}
	return "MEDIUM"
}

// routeBestPractice is the stage × complexity policy. The model tier scales with complexity so
// simple work runs on the cheaper Tier 2 code model (capable enough to stay reliable; Tier 3 is
// reserved for log parsing) and only complex or system-level work pays for Tier 1:
//
//	              LOW              MEDIUM              HIGH                SYSTEM
//	planning      T2 ReAct         T1 BMAD             T1 BMAD             T1 Superpower
//	decompose     T2 ReAct         T1 Supervisor       T1 Supervisor       T1 Superpower
//	implement     T2 ReAct         T2 ReAct            T1 BMAD             T1 Superpower
//	verify        T2 Superpower    T2 Superpower       T1 Superpower       T1 Superpower
func (r *Router) routeBestPractice(stageID string, complexity string, repoTypes []string) *RoutingDecision {
	cx := NormalizeComplexity(complexity)
	stage := canonicalRouterStage(stageID)
	class := classOf(stage)

	tier, method := TierCodeGen, "react"
	var budget int64 = 50000
	var why string
	switch {
	case class == classOther:
		why = "Default best-practice fallback for custom stage"
	case cx == "SYSTEM":
		tier, method, budget = TierReasoning, "superpower", 150000
		why = "System-level change (containers, CI, migrations) needs Tier 1 with the Superpower plan-and-execute harness"
	case cx == "LOW" && class != classVerify:
		budget = 40000
		why = "Low complexity: cheaper Tier 2 model with a single fast ReAct agent is reliable enough"
	case class == classPlanning:
		tier, method, budget = TierReasoning, "bmad", 120000
		why = fmt.Sprintf("%s complexity: Tier 1 reasoning model with BMAD product/architecture/QA roles", cx[:1]+strings.ToLower(cx[1:]))
	case class == classDecompose:
		tier, method, budget = TierReasoning, "supervisor", 80000
		why = "Tier 1 model with a Supervisor for dependency mapping and atomic task decomposition"
	case class == classImplement && cx == "HIGH":
		tier, method, budget = TierReasoning, "bmad", 200000
		why = "High complexity implementation: Tier 1 with BMAD developer/QA pairing"
	case class == classImplement:
		budget = 100000
		why = "Medium complexity implementation: Tier 2 code model with fast ReAct iteration"
	case class == classVerify && cx == "HIGH":
		tier, method, budget = TierReasoning, "superpower", 120000
		why = "High complexity verification across services: Tier 1 with the Superpower automation harness"
	default: // verification, LOW or MEDIUM
		method, budget = "superpower", 90000
		why = "Browser/terminal verification on Tier 2 with the Superpower automation harness"
	}
	if r.cfg.Router.MaxTokenBudget > 0 && budget > r.cfg.Router.MaxTokenBudget {
		budget = r.cfg.Router.MaxTokenBudget
	}
	model := r.modelForTier(tier)
	return &RoutingDecision{
		Strategy:       "best_practice",
		Model:          model,
		Tier:           tier,
		FallbackChain:  []string{model},
		Method:         method,
		TokenBudget:    budget,
		Reasoning:      "Best practice: " + why,
		RequiresDocker: class == classImplement || class == classVerify || stage == "ATDD_RED_PHASE" || cx == "SYSTEM",
		MethodSource:   "policy",
		TierSource:     "policy",
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
