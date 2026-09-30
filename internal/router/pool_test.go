package router

import (
	"reflect"
	"testing"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/config"
)

func poolConfig() *config.OrchestratorConfig {
	cfg := config.GetDefaultConfig()
	cfg.ModelTiers.Tier1Reasoning = "claude/claude-opus-5-5"
	cfg.ModelTiers.Tier2CodeGen = "claude/claude-3-7-sonnet-20250219"
	cfg.ModelTiers.Tier3LogParse = "antigravity/gemini-2.0-flash"
	return cfg
}

var poolChain = []PriorityModelItem{
	{Provider: "claude", Model: "claude-3-7-sonnet-20250219", Enabled: true, CostPer1k: 0.003, LatencyMs: 140},
	{Provider: "chatgpt", Model: "gpt-4o", Enabled: true, CostPer1k: 0.0025, LatencyMs: 185},
	{Provider: "antigravity", Model: "gemini-2.0-flash", Enabled: true, CostPer1k: 0.0001, LatencyMs: 90},
}

func TestBestPracticeSuggestsRecommendedModelOutsideChain(t *testing.T) {
	r := NewRouter(poolConfig())
	r.SetSettings(RouterSettings{Mode: ModeBestPractice, PriorityChain: poolChain})

	d := r.Route("INTAKE_PRD", "MEDIUM", nil)
	if d.Model != "claude/claude-3-7-sonnet-20250219" {
		t.Fatalf("expected highest-ranked tier1 chain model, got %s", d.Model)
	}
	if d.Suggestion == nil || d.Suggestion.Model != "claude/claude-opus-5-5" || d.Suggestion.Action != SuggestAdd {
		t.Fatalf("expected an 'add opus' suggestion, got %+v", d.Suggestion)
	}
	want := []string{"claude/claude-3-7-sonnet-20250219", "chatgpt/gpt-4o", "antigravity/gemini-2.0-flash"}
	if !reflect.DeepEqual(d.FallbackChain, want) {
		t.Fatalf("fallbacks must come from the chain in rank order, got %v", d.FallbackChain)
	}
}

func TestBestPracticeUsesRecommendedModelWhenInChain(t *testing.T) {
	r := NewRouter(poolConfig())
	chain := append([]PriorityModelItem{{Provider: "claude", Model: "claude-opus-5-5", Enabled: true}}, poolChain...)
	r.SetSettings(RouterSettings{Mode: ModeBestPractice, PriorityChain: chain})

	d := r.Route("INTAKE_PRD", "MEDIUM", nil)
	if d.Model != "claude/claude-opus-5-5" || d.Suggestion != nil {
		t.Fatalf("expected opus with no suggestion, got %s / %+v", d.Model, d.Suggestion)
	}
	// Standard implementation is tier2: the recommended sonnet is in the chain.
	if impl := r.Route("IMPLEMENTATION_GREEN", "LOW", nil); impl.Model != "claude/claude-3-7-sonnet-20250219" || impl.FallbackChain[0] != impl.Model {
		t.Fatalf("tier2 decision = %+v", impl)
	}
}

func TestSuggestionActionsForDisabledAndDisconnected(t *testing.T) {
	r := NewRouter(poolConfig())
	chain := append([]PriorityModelItem{{Provider: "claude", Model: "claude-opus-5-5", Enabled: false}}, poolChain...)
	r.SetSettings(RouterSettings{Mode: ModeBestPractice, PriorityChain: chain})
	if d := r.Route("INTAKE_PRD", "MEDIUM", nil); d.Suggestion == nil || d.Suggestion.Action != SuggestEnable {
		t.Fatalf("expected enable suggestion, got %+v", d.Suggestion)
	}

	chain[0].Enabled = true
	r.SetSettings(RouterSettings{PriorityChain: chain})
	r.SetAvailability(func(p string) bool { return p != "claude" })
	d := r.Route("INTAKE_PRD", "MEDIUM", nil)
	if d.Suggestion == nil || d.Suggestion.Action != SuggestConnect {
		t.Fatalf("expected connect suggestion, got %+v", d.Suggestion)
	}
	if d.Model != "chatgpt/gpt-4o" {
		t.Fatalf("disconnected claude must be skipped, got %s", d.Model)
	}
	for _, m := range d.FallbackChain {
		if m == "claude/claude-3-7-sonnet-20250219" || m == "claude/claude-opus-5-5" {
			t.Fatalf("fallback chain contains disconnected provider: %v", d.FallbackChain)
		}
	}
}

func TestChainModesSkipUnavailableProviders(t *testing.T) {
	r := NewRouter(poolConfig())
	r.SetSettings(RouterSettings{Mode: ModePrioritySequence, PriorityChain: poolChain})
	r.SetAvailability(func(p string) bool { return p != "claude" })
	if d := r.Route("INTAKE_PRD", "MEDIUM", nil); d.Model != "chatgpt/gpt-4o" || len(d.FallbackChain) != 2 {
		t.Fatalf("priority sequence = %+v", d)
	}

	// Everything unavailable: still try the chain rather than refuse, and say so.
	r.SetAvailability(func(string) bool { return false })
	d := r.Route("INTAKE_PRD", "MEDIUM", nil)
	if d.Model != "claude/claude-3-7-sonnet-20250219" || len(d.FallbackChain) != 3 {
		t.Fatalf("degraded priority sequence = %+v", d)
	}
}

func TestEmptyChainLeavesBestPracticeUnrestricted(t *testing.T) {
	cfg := poolConfig()
	r := NewRouter(cfg)
	r.SetSettings(RouterSettings{Mode: ModePrioritySequence, PriorityChain: []PriorityModelItem{}})
	d := r.Route("INTAKE_PRD", "MEDIUM", nil)
	if d.Model != cfg.ModelTiers.Tier1Reasoning || d.Suggestion != nil || len(d.FallbackChain) != 1 {
		t.Fatalf("empty chain decision = %+v", d)
	}
}

func TestPreviewDoesNotMutateState(t *testing.T) {
	r := NewRouter(poolConfig())
	r.SetSettings(RouterSettings{Mode: ModeRoundRobin, PriorityChain: poolChain})

	draft := &RouterSettings{Mode: ModeBestPractice, PriorityChain: poolChain[1:]}
	rows, tiers := r.Preview(draft)
	if len(rows) != len(PreviewStages) || len(tiers) != 3 {
		t.Fatalf("preview sizes: %d rows, %d tiers", len(rows), len(tiers))
	}
	if rows[0].Decision.Model != "chatgpt/gpt-4o" {
		t.Fatalf("draft chain not used in preview: %+v", rows[0].Decision)
	}
	if tiers[0].Recommended != "claude/claude-opus-5-5" || tiers[0].Suggestion == nil {
		t.Fatalf("tier1 assignment = %+v", tiers[0])
	}
	if s := r.GetSettings(); s.Mode != ModeRoundRobin || len(s.PriorityChain) != 3 {
		t.Fatalf("preview changed live settings: %+v", s)
	}
	r.Preview(nil)
	r.Preview(nil)
	if first := r.Route("X", "LOW", nil); first.Model != "claude/claude-3-7-sonnet-20250219" {
		t.Fatalf("preview advanced round robin: %s", first.Model)
	}
}

func TestSameModelFoldsProviderAliases(t *testing.T) {
	if !SameModel("openai/gpt-4o", "chatgpt/gpt-4o") || !SameModel("claude-opus-5-5", "claude/claude-opus-5-5") {
		t.Fatal("aliases should compare equal")
	}
	if SameModel("openai/gpt-4o", "openai/gpt-4o-mini") {
		t.Fatal("different models compared equal")
	}
}

func TestInferTiers(t *testing.T) {
	cases := map[string][]string{
		"claude-opus-5-5":            {TierReasoning},
		"claude-3-7-sonnet-20250219": {TierReasoning, TierCodeGen},
		"claude-3-5-haiku-20241022":  {TierLogParse},
		"gemini-2.5-pro":             {TierReasoning},
		"gemini-2.0-flash":           {TierCodeGen, TierLogParse},
		"gpt-4o-mini":                {TierLogParse},
		"o1":                         {TierReasoning},
		"deepseek-coder-v2":          {TierCodeGen},
	}
	for model, want := range cases {
		if got := InferTiers(model); !reflect.DeepEqual(got, want) {
			t.Errorf("InferTiers(%q) = %v, want %v", model, got, want)
		}
	}
}
