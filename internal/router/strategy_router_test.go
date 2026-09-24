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
