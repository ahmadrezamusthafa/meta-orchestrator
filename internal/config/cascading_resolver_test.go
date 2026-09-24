package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCascadingConfigResolver(t *testing.T) {
	tmpProject := t.TempDir()
	tmpSystem := t.TempDir()

	// 1. Write System config overriding Tier 1 and tools dir
	sysYAML := `
model_tiers:
  tier1_reasoning: openai/gpt-4o
tools:
  tool_dir: /opt/shared/tools
`
	if err := os.WriteFile(filepath.Join(tmpSystem, "config.yaml"), []byte(sysYAML), 0644); err != nil {
		t.Fatalf("failed to write system config: %v", err)
	}

	// 2. Write Project config overriding Tier 2 (to local OpenCode) and Router strategy
	projSDLC := filepath.Join(tmpProject, ".sdlc")
	if err := os.MkdirAll(projSDLC, 0755); err != nil {
		t.Fatalf("failed to mkdir: %v", err)
	}
	projYAML := `
model_tiers:
  tier2_codegen: opencode/deepseek-coder-v2
router:
  strategy: custom
  max_token_budget: 350000
active_sdlc: hotfix-fast-track
`
	if err := os.WriteFile(filepath.Join(projSDLC, "config.yaml"), []byte(projYAML), 0644); err != nil {
		t.Fatalf("failed to write project config: %v", err)
	}

	resolver := NewCascadingConfigResolver(tmpProject)
	resolver.systemConfDir = tmpSystem

	cfg, err := resolver.Resolve()
	if err != nil {
		t.Fatalf("failed to resolve cascading config: %v", err)
	}

	// Verify project override
	if cfg.ModelTiers.Tier2CodeGen != "opencode/deepseek-coder-v2" {
		t.Errorf("expected project Tier 2 override 'opencode/deepseek-coder-v2', got '%s'", cfg.ModelTiers.Tier2CodeGen)
	}
	if cfg.Router.Strategy != "custom" {
		t.Errorf("expected router strategy 'custom', got '%s'", cfg.Router.Strategy)
	}
	if cfg.Router.MaxTokenBudget != 350000 {
		t.Errorf("expected token budget 350000, got %d", cfg.Router.MaxTokenBudget)
	}
	if cfg.ActiveSDLC != "hotfix-fast-track" {
		t.Errorf("expected active SDLC hotfix-fast-track, got '%s'", cfg.ActiveSDLC)
	}

	// Verify system override inheritance
	if cfg.ModelTiers.Tier1Reasoning != "openai/gpt-4o" {
		t.Errorf("expected system Tier 1 override 'openai/gpt-4o', got '%s'", cfg.ModelTiers.Tier1Reasoning)
	}
	if cfg.Tools.ToolDir != "/opt/shared/tools" {
		t.Errorf("expected system tool dir override '/opt/shared/tools', got '%s'", cfg.Tools.ToolDir)
	}

	// Verify default fallback inheritance for non-overridden fields
	if cfg.ModelTiers.Tier3LogParse != "claude/claude-3-5-haiku-20241022" {
		t.Errorf("expected default Tier 3 fallback 'claude/claude-3-5-haiku-20241022', got '%s'", cfg.ModelTiers.Tier3LogParse)
	}
}
