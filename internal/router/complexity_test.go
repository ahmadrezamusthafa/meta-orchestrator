package router

import (
	"context"
	"errors"
	"testing"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/config"
)

func TestBestPracticeTierScalesWithComplexity(t *testing.T) {
	cfg := config.GetDefaultConfig()
	cfg.ModelTiers.Tier1Reasoning = "claude/opus"
	cfg.ModelTiers.Tier2CodeGen = "claude/sonnet"
	r := NewRouter(cfg)
	r.SetSettings(RouterSettings{Mode: ModeBestPractice, PriorityChain: []PriorityModelItem{}})
	cases := []struct {
		stage, cx, model, method string
	}{
		{"prd_discovery", "LOW", "claude/sonnet", "react"},
		{"prd_discovery", "MEDIUM", "claude/opus", "bmad"},
		{"techdoc_rfc", "HIGH", "claude/opus", "bmad"}, // workflow id spelling, previously fell to the generic fallback
		{"task_breakdown", "LOW", "claude/sonnet", "react"},
		{"task_breakdown", "MEDIUM", "claude/opus", "supervisor"},
		{"task_implementation", "LOW", "claude/sonnet", "react"},
		{"task_implementation", "MEDIUM", "claude/sonnet", "react"},
		{"task_implementation", "HIGH", "claude/opus", "bmad"},
		{"task_implementation", "CRITICAL", "claude/opus", "bmad"},
		{"task_implementation", "SYSTEM", "claude/opus", "superpower"},
		{"e2e_validation", "LOW", "claude/sonnet", "superpower"},
		{"e2e_validation", "HIGH", "claude/opus", "superpower"},
	}
	for _, c := range cases {
		d := r.Route(c.stage, c.cx, nil)
		if d.Model != c.model || d.Method != c.method {
			t.Errorf("%s/%s = %s on %s, want %s on %s", c.stage, c.cx, d.Method, d.Model, c.method, c.model)
		}
	}
}

func TestAnalyzeComplexityUsesAIAnswer(t *testing.T) {
	classify := func(ctx context.Context, system, prompt string) (string, string, error) {
		return "Here you go:\n```json\n{\"complexity\":\"low\",\"task_type\":\"docs\",\"rationale\":\"README only.\",\"signals\":[\"readme\"]}\n```", "claude/haiku", nil
	}
	a := AnalyzeComplexity(context.Background(), classify, "Update README", "", []string{"web"})
	if a.Source != SourceAI || a.Complexity != "LOW" || a.TaskType != "docs" || a.Model != "claude/haiku" || a.Fallback != "" {
		t.Fatalf("assessment = %+v", a)
	}
}

func TestAnalyzeComplexityRaisesForRepoScope(t *testing.T) {
	classify := func(context.Context, string, string) (string, string, error) {
		return `{"complexity":"MEDIUM","task_type":"crud","rationale":"r"}`, "m", nil
	}
	a := AnalyzeComplexity(context.Background(), classify, "Add field", "", []string{"a", "b", "c"})
	if a.Complexity != "HIGH" {
		t.Fatalf("3 repositories graded %s, want HIGH", a.Complexity)
	}
}

func TestAnalyzeComplexityFallsBackToHeuristic(t *testing.T) {
	for name, classify := range map[string]Classifier{
		"error":   func(context.Context, string, string) (string, string, error) { return "", "", errors.New("timeout") },
		"garbage": func(context.Context, string, string) (string, string, error) { return "it is fairly simple", "m", nil },
		"unknown": func(context.Context, string, string) (string, string, error) {
			return `{"complexity":"EASY"}`, "m", nil
		},
	} {
		a := AnalyzeComplexity(context.Background(), classify, "Fix typo in README", "", []string{"web"})
		if a.Source != SourceHeuristic || a.Complexity != "LOW" || a.Fallback == "" {
			t.Errorf("%s: assessment = %+v", name, a)
		}
	}
}

func TestHeuristicComplexity(t *testing.T) {
	cases := map[string]string{
		"Add a CI pipeline job that builds the Dockerfile": "SYSTEM",
		"Version the payments API contract":                "HIGH",
		"Fix typo in README":                               "LOW",
		"Add invoice line item endpoint":                   "MEDIUM",
	}
	for title, want := range cases {
		if got := HeuristicComplexity(title, "", []string{"one"}); got.Complexity != want {
			t.Errorf("%q = %s, want %s", title, got.Complexity, want)
		}
	}
	if got := HeuristicComplexity("Add audit endpoint", "", []string{"a", "b", "c"}); got.Complexity != "HIGH" {
		t.Errorf("feature over 3 repositories = %s, want HIGH", got.Complexity)
	}
	// A clear small change stays LOW even when every repository is selected.
	if got := HeuristicComplexity("Fix typo in README", "", []string{"a", "b", "c"}); got.Complexity != "LOW" {
		t.Errorf("typo over 3 repositories = %s, want LOW", got.Complexity)
	}
}

func TestApplyPlanPutsChosenModelFirst(t *testing.T) {
	d := &RoutingDecision{Strategy: "best_practice", Model: "claude/sonnet", Method: "react", FallbackChain: []string{"claude/sonnet", "openai/gpt"}}
	ApplyPlan(d, "BMAD", "openai/gpt", "tier1", true)
	if d.Method != "bmad" || d.Model != "openai/gpt" || d.Strategy != "user_override" || d.Tier != "tier1" {
		t.Fatalf("decision = %+v", d)
	}
	if len(d.FallbackChain) != 2 || d.FallbackChain[0] != "openai/gpt" || d.FallbackChain[1] != "claude/sonnet" {
		t.Fatalf("fallback chain = %v", d.FallbackChain)
	}

	confirmed := &RoutingDecision{Strategy: "best_practice", Model: "claude/sonnet", Method: "react", FallbackChain: []string{"claude/sonnet"}}
	ApplyPlan(confirmed, "react", "claude/sonnet", "", false)
	if confirmed.Strategy != "best_practice" || confirmed.Model != "claude/sonnet" {
		t.Fatalf("unchanged confirmed plan altered the decision: %+v", confirmed)
	}
}

// With a fresh install's defaults, each complexity lands on its own Claude tier model.
func TestDefaultsRouteComplexityToTierModels(t *testing.T) {
	r := NewRouter(config.GetDefaultConfig())
	r.SetSettings(RouterSettings{Mode: ModeBestPractice, PriorityChain: DefaultPriorityChain()})
	for cx, want := range map[string]string{"LOW": "claude/claude-sonnet-5-5", "HIGH": "claude/claude-opus-5-5"} {
		if d := r.Route("task_implementation", cx, nil); d.Model != want {
			t.Errorf("%s implementation = %s, want %s (%s)", cx, d.Model, want, d.Reasoning)
		}
	}
	if m, _ := r.AnalysisModel(); m != "claude/claude-haiku-4-5" {
		t.Errorf("complexity analysis model = %s, want claude/claude-haiku-4-5", m)
	}
}
