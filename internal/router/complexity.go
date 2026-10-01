package router

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Assessment sources.
const (
	SourceAI        = "ai"
	SourceHeuristic = "heuristic"
	SourceOperator  = "operator"
)

// ComplexityAssessment is the analysed complexity of a task, before any routing decision.
type ComplexityAssessment struct {
	Complexity string   `json:"complexity"` // LOW, MEDIUM, HIGH or SYSTEM
	TaskType   string   `json:"task_type"`
	Rationale  string   `json:"rationale"`
	Signals    []string `json:"signals,omitempty"`
	Source     string   `json:"source"`          // ai, heuristic or operator
	Model      string   `json:"model,omitempty"` // model that analysed it, when Source is ai
	// Fallback explains why the AI analysis was not used, when it failed.
	Fallback string `json:"fallback,omitempty"`
}

// Classifier sends a prompt to a model and returns its text answer.
type Classifier func(ctx context.Context, system, prompt string) (answer, model string, err error)

// complexityRubric defines the strata for both the AI analyst and the operator-facing UI.
const complexityRubric = `LOW: one repository, a small contained change (copy, docs, config value, styling, a guard or small bug fix). No API, schema or contract change.
MEDIUM: a feature or fix inside one service or module (new endpoint, CRUD, validation, a component), possibly touching two repositories, with no breaking change.
HIGH: cross-service or multi-repository work, API/contract versioning, breaking changes, security or payment-critical logic, concurrency, large refactors, or unclear requirements.
SYSTEM: infrastructure and environment changes: containers, CI/CD pipelines, database schema migrations, deployment or cluster configuration.`

var taskTypes = []string{"architecture", "migration", "docs", "bugfix", "crud", "general"}

// AnalyzeComplexity asks the classifier to grade the task and falls back to keyword heuristics
// when it is unavailable or answers with something unusable. It never fails.
func AnalyzeComplexity(ctx context.Context, classify Classifier, title, description string, repos []string) ComplexityAssessment {
	heuristic := HeuristicComplexity(title, description, repos)
	if classify == nil {
		return heuristic
	}
	system := "You grade software tasks by complexity so a router can pick a model and methodology. " +
		"Answer with one JSON object and nothing else."
	prompt := fmt.Sprintf(`Grade this task.

Complexity levels:
%s

Task types: %s.

Title: %s
Description: %s
Repositories in scope: %s

Reply with exactly: {"complexity": "LOW|MEDIUM|HIGH|SYSTEM", "task_type": "<one task type>", "rationale": "<one or two sentences>", "signals": ["<short evidence>", ...]}`,
		complexityRubric, strings.Join(taskTypes, ", "), title, orNone(description), orNone(strings.Join(repos, ", ")))

	answer, model, err := classify(ctx, system, prompt)
	if err != nil {
		heuristic.Fallback = "AI analysis unavailable: " + err.Error()
		return heuristic
	}
	a, err := parseAssessment(answer)
	if err != nil {
		heuristic.Fallback = "AI analysis unusable: " + err.Error()
		return heuristic
	}
	a.Source, a.Model = SourceAI, model
	if a.TaskType == "" {
		a.TaskType = heuristic.TaskType
	}
	// Scope the model cannot see from text alone: a standard feature across more than two
	// repositories is cross-service work. A change the model grades LOW stays LOW — a typo fix
	// does not grow because more repositories happen to be selected.
	if len(repos) > 2 && a.Complexity == "MEDIUM" {
		a.Complexity = "HIGH"
		a.Signals = append(a.Signals, fmt.Sprintf("%d repositories in scope", len(repos)))
	}
	return a
}

func orNone(s string) string {
	if strings.TrimSpace(s) == "" {
		return "(none)"
	}
	return s
}

func parseAssessment(answer string) (ComplexityAssessment, error) {
	start, end := strings.Index(answer, "{"), strings.LastIndex(answer, "}")
	if start < 0 || end <= start {
		return ComplexityAssessment{}, fmt.Errorf("no JSON object in the answer")
	}
	var a ComplexityAssessment
	if err := json.Unmarshal([]byte(answer[start:end+1]), &a); err != nil {
		return ComplexityAssessment{}, fmt.Errorf("invalid JSON: %w", err)
	}
	cx := strings.ToUpper(strings.TrimSpace(a.Complexity))
	switch cx {
	case "LOW", "MEDIUM", "HIGH", "SYSTEM":
	case "CRITICAL":
		cx = "HIGH"
	default:
		return ComplexityAssessment{}, fmt.Errorf("unknown complexity %q", a.Complexity)
	}
	a.Complexity = cx
	a.TaskType = strings.ToLower(strings.TrimSpace(a.TaskType))
	known := false
	for _, t := range taskTypes {
		known = known || t == a.TaskType
	}
	if !known {
		a.TaskType = ""
	}
	if len(a.Signals) > 6 {
		a.Signals = a.Signals[:6]
	}
	return a, nil
}

var (
	systemSignals = []string{"dockerfile", "docker", "kubernetes", "k8s", "helm", "terraform", "ci pipeline", "ci/cd", "ci job",
		"github actions", "pipeline", "database migration", "schema migration", "db migration", "infrastructure", "deployment"}
	highSignals = []string{"refactor", "migration", "breaking change", "contract", "versioning", "multi-repo", "cross-service",
		"payment", "security", "authentication", "concurrency", "race condition", "distributed", "architecture"}
	lowSignals = []string{"typo", "readme", "css tweak", "color change", "copy change", "wording", "rename", "docs",
		"documentation", "comment", "bump version", "badge", "styling", "label text"}
)

// HeuristicComplexity grades a task from keywords and repository count.
func HeuristicComplexity(title, description string, repos []string) ComplexityAssessment {
	text := " " + strings.ToLower(title+" "+description) + " "
	match := func(words []string) []string {
		var hits []string
		for _, w := range words {
			if strings.Contains(text, w) {
				hits = append(hits, w)
			}
		}
		return hits
	}
	a := ComplexityAssessment{TaskType: ClassifyTaskType(title, description), Source: SourceHeuristic}
	switch sys, high, low := match(systemSignals), match(highSignals), match(lowSignals); {
	case len(sys) > 0:
		a.Complexity, a.Signals = "SYSTEM", sys
		a.Rationale = "Mentions infrastructure or environment changes."
	case len(high) > 0:
		a.Complexity, a.Signals = "HIGH", high
		a.Rationale = "Mentions cross-cutting or high-risk work."
	case len(low) > 0:
		// Clear small-change signals outrank repository count (the selection often defaults to all repos).
		a.Complexity, a.Signals = "LOW", low
		a.Rationale = "A small, contained change."
	case len(repos) > 2:
		a.Complexity, a.Signals = "HIGH", []string{fmt.Sprintf("%d repositories in scope", len(repos))}
		a.Rationale = "Spans more than two repositories."
	default:
		a.Complexity = "MEDIUM"
		a.Rationale = "No strong signals either way; treated as a standard feature."
	}
	return a
}

// PlannedStage is the proposed routing of one stage.
type PlannedStage struct {
	StageID   string `json:"stage_id"`
	Method    string `json:"method"`
	Model     string `json:"model"`
	Tier      string `json:"tier,omitempty"`
	Reasoning string `json:"reasoning"`
	Strategy  string `json:"strategy"`
	// Wanted is the recommended model when the chain could not run it (not allowed or not connected).
	Wanted string `json:"wanted,omitempty"`
}

// Plan proposes the routing of each stage for a task without changing router state.
func (r *Router) Plan(stages []string, complexity, taskType string, repos []string) []PlannedStage {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]PlannedStage, 0, len(stages))
	for _, st := range stages {
		d := r.route(r.mode, r.priorityChain, st, complexity, taskType, repos, false)
		ps := PlannedStage{StageID: st, Method: d.Method, Model: d.Model, Tier: d.Tier, Reasoning: d.Reasoning, Strategy: d.Strategy}
		if d.Suggestion != nil {
			ps.Wanted = d.Suggestion.Model
		}
		out = append(out, ps)
	}
	return out
}

// Complexities are the routing strata, in display order.
var Complexities = []string{"LOW", "MEDIUM", "HIGH", "SYSTEM"}

// MatrixCell is what the router would decide for one stage at one complexity.
type MatrixCell struct {
	StageID    string           `json:"stage_id"`
	Complexity string           `json:"complexity"`
	Decision   *RoutingDecision `json:"decision"`
}

// PreviewMatrix routes every stage at every complexity under settings (or the live settings when
// nil) without changing router state, so the UI can show exactly what will run.
func (r *Router) PreviewMatrix(settings *RouterSettings, stages []string) []MatrixCell {
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
	out := make([]MatrixCell, 0, len(stages)*len(Complexities))
	for _, st := range stages {
		for _, cx := range Complexities {
			out = append(out, MatrixCell{StageID: st, Complexity: cx, Decision: r.route(mode, chain, st, cx, "", nil, false)})
		}
	}
	return out
}

// ModelOption is a model the operator may choose for a stage.
type ModelOption struct {
	Model string   `json:"model"`
	Tiers []string `json:"tiers,omitempty"`
}

// ModelOptions lists the enabled chain models, or the configured tier models when the chain is empty.
func (r *Router) ModelOptions() []ModelOption {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []ModelOption
	seen := map[string]bool{}
	for _, it := range enabledItems(r.priorityChain) {
		m := it.FullModel()
		if !seen[m] {
			seen[m] = true
			out = append(out, ModelOption{Model: m, Tiers: it.Tiers})
		}
	}
	if len(out) == 0 {
		for _, t := range []string{TierReasoning, TierCodeGen, TierLogParse} {
			if m := r.modelForTier(t); m != "" && !seen[m] {
				seen[m] = true
				out = append(out, ModelOption{Model: m, Tiers: []string{t}})
			}
		}
	}
	return out
}

// AnalysisModel is the model used to grade complexity: the Tier 3 model bound to the chain,
// since grading is a short classification that does not need a reasoning model.
func (r *Router) AnalysisModel() (model string, fallback []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	d := &RoutingDecision{Model: r.modelForTier(TierLogParse), Tier: TierLogParse}
	r.bindToPool(d, r.priorityChain)
	return d.Model, d.FallbackChain
}

// ApplyPlan replaces a live decision's method and model with the stage's confirmed route. The
// confirmed model leads the fallback chain; the live chain's other models stay behind it.
func ApplyPlan(d *RoutingDecision, method, model, tier string, overridden bool) *RoutingDecision {
	if d == nil {
		return d
	}
	proposed := fmt.Sprintf("%s on %s", d.Method, d.Model)
	if method != "" {
		d.Method = strings.ToLower(method)
	}
	if model != "" && !SameModel(model, d.Model) {
		chain := []string{model}
		for _, m := range d.FallbackChain {
			if !SameModel(m, model) {
				chain = append(chain, m)
			}
		}
		d.Model, d.FallbackChain = model, chain
		if tier != "" {
			d.Tier = tier
		}
	}
	if overridden {
		d.Strategy, d.MethodSource, d.TierSource = "user_override", "user", "user"
		d.Reasoning = fmt.Sprintf("Operator choice: %s on %s (router would pick %s)", d.Method, d.Model, proposed)
	} else {
		d.Reasoning = fmt.Sprintf("Confirmed plan: %s on %s | %s", d.Method, d.Model, d.Reasoning)
	}
	return d
}
