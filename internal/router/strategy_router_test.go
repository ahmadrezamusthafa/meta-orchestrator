package router

import (
	"testing"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/config"
)

func TestBestPracticeRouter(t *testing.T) {
	cfg := config.GetDefaultConfig()
	r := NewRouter(cfg)

	// 1. Architecture stage should route to Tier 1 + BMAD
	dec1 := r.Route("TECH_DOC_RFC", "HIGH", []string{"backend"})
	if dec1.Strategy != "best_practice" {
		t.Errorf("expected best_practice strategy, got %s", dec1.Strategy)
	}
	if dec1.Model != cfg.ModelTiers.Tier1Reasoning {
		t.Errorf("expected Tier 1 model, got %s", dec1.Model)
	}
	if dec1.Method != "bmad" {
		t.Errorf("expected bmad method, got %s", dec1.Method)
	}

	// 2. Low complexity implementation should route to Tier 2 + ReAct
	dec2 := r.Route("IMPLEMENTATION_GREEN", "LOW", []string{"frontend"})
	if dec2.Method != "react" {
		t.Errorf("expected react method for LOW complexity implementation, got %s", dec2.Method)
	}
	if dec2.Model != cfg.ModelTiers.Tier2CodeGen {
		t.Errorf("expected Tier 2 model, got %s", dec2.Model)
	}

	// 3. E2E verification should route to Superpower automation harness
	dec3 := r.Route("E2E_AUTOMATION", "MEDIUM", []string{"frontend", "backend"})
	if dec3.Method != "superpower" {
		t.Errorf("expected superpower method for E2E, got %s", dec3.Method)
	}
	if !dec3.RequiresDocker {
		t.Errorf("expected RequiresDocker for E2E automation")
	}
}

func TestCustomRuleRouter(t *testing.T) {
	cfg := config.GetDefaultConfig()
	cfg.Router.Strategy = "custom"
	cfg.Router.CustomRules = `
stage:ATDD_RED_PHASE -> model:opencode/deepseek-coder-v2,method:supervisor
stage:IMPLEMENTATION_GREEN -> model:openai/gpt-4o,method:superpower
`
	r := NewRouter(cfg)

	// Custom rule match 1
	dec1 := r.Route("ATDD_RED_PHASE", "MEDIUM", nil)
	if dec1.Strategy != "custom" {
		t.Errorf("expected custom strategy, got %s", dec1.Strategy)
	}
	if dec1.Model != "opencode/deepseek-coder-v2" {
		t.Errorf("expected custom model opencode/deepseek-coder-v2, got %s", dec1.Model)
	}
	if dec1.Method != "supervisor" {
		t.Errorf("expected custom method supervisor, got %s", dec1.Method)
	}

	// Unmatched stage should gracefully fall back to best_practice
	dec2 := r.Route("TECH_DOC_RFC", "HIGH", nil)
	if dec2.Strategy != "best_practice" {
		t.Errorf("expected fallback to best_practice, got %s", dec2.Strategy)
	}
}

func TestRouterModesAndCustomPriority(t *testing.T) {
	cfg := config.GetDefaultConfig()
	r := NewRouter(cfg)

	customChain := []PriorityModelItem{
		{ID: "1", Provider: "openai", Model: "gpt-4o", Name: "GPT-4o", Enabled: true, CostPer1k: 0.005, LatencyMs: 200},
		{ID: "2", Provider: "claude", Model: "claude-3-5-sonnet", Name: "Claude Sonnet", Enabled: true, CostPer1k: 0.003, LatencyMs: 150},
		{ID: "3", Provider: "antigravity", Model: "gemini-2.0-flash", Name: "Gemini Flash", Enabled: true, CostPer1k: 0.0001, LatencyMs: 90},
	}

	// 1. Test Priority Sequence mode
	r.SetSettings(RouterSettings{
		Mode:          ModePrioritySequence,
		PriorityChain: customChain,
	})
	dec := r.Route("task_implementation", "HIGH", nil)
	if dec.Strategy != "priority_sequence" {
		t.Errorf("expected strategy 'priority_sequence', got %s", dec.Strategy)
	}
	if dec.Model != "openai/gpt-4o" {
		t.Errorf("expected primary model 'openai/gpt-4o', got %s", dec.Model)
	}
	if len(dec.FallbackChain) != 3 || dec.FallbackChain[1] != "claude/claude-3-5-sonnet" {
		t.Errorf("expected fallback chain with claude 2nd, got %v", dec.FallbackChain)
	}

	// 2. Test Cost-Optimized mode (Gemini Flash has lowest cost)
	r.SetSettings(RouterSettings{
		Mode:          ModeCostOptimized,
		PriorityChain: customChain,
	})
	decCost := r.Route("task_implementation", "HIGH", nil)
	if decCost.Strategy != "cost_optimized" {
		t.Errorf("expected strategy 'cost_optimized', got %s", decCost.Strategy)
	}
	if decCost.Model != "antigravity/gemini-2.0-flash" {
		t.Errorf("expected cheapest model 'antigravity/gemini-2.0-flash', got %s", decCost.Model)
	}

	// 3. Test Latency-Optimized mode (Gemini Flash has lowest latency 90ms)
	r.SetSettings(RouterSettings{
		Mode:          ModeLatencyOptimized,
		PriorityChain: customChain,
	})
	decLat := r.Route("task_implementation", "HIGH", nil)
	if decLat.Model != "antigravity/gemini-2.0-flash" {
		t.Errorf("expected fastest model 'antigravity/gemini-2.0-flash', got %s", decLat.Model)
	}
}

