package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"
)

// CascadingConfigResolver resolves configurations across Project > System > Env tiers.
type CascadingConfigResolver struct {
	mu            sync.RWMutex
	projectRoot   string
	systemConfDir string
	cachedConfig  *OrchestratorConfig
}

// NewCascadingConfigResolver initializes the resolver.
func NewCascadingConfigResolver(projectRoot string) *CascadingConfigResolver {
	userHome, _ := os.UserHomeDir()
	systemDir := filepath.Join(userHome, ".config", "meta-orchestrator")
	return &CascadingConfigResolver{
		projectRoot:   projectRoot,
		systemConfDir: systemDir,
	}
}

// GetDefaultConfig provides baseline fallback defaults.
func GetDefaultConfig() *OrchestratorConfig {
	return &OrchestratorConfig{
		Providers: map[string]AIProviderConfig{
			"claude": {
				Enabled: true,
				Model:   "claude-sonnet-5-5",
			},
			"antigravity": {
				Enabled: true,
				Model:   "gemini-1.5-pro",
			},
			"openai": {
				Enabled: true,
				Model:   "gpt-4o",
			},
			"opencode": {
				Enabled: false,
				BaseURL: "http://localhost:11434",
				Model:   "deepseek-coder-v2",
			},
		},
		ModelTiers: ModelTiersConfig{
			// Tier 1 plans and designs, Tier 2 writes code and handles low-complexity work,
			// Tier 3 grades complexity and parses logs.
			Tier1Reasoning: "claude/claude-opus-5-5",
			Tier2CodeGen:   "claude/claude-sonnet-5-5",
			Tier3LogParse:  "claude/claude-haiku-4-5",
		},
		Router: RouterConfig{
			Strategy:       "best_practice",
			MaxTokenBudget: 200000,
		},
		Tools: ToolConfig{
			AutoUpdate: true,
			ToolDir:    "~/.meta-orchestrator/tools",
		},
		ActiveSDLC: "general-ai-sdlc",
	}
}

// Resolve loads and deep-merges configuration according to priority:
// Project (.sdlc/config.yaml or .meta-orchestrator.yaml) > System (~/.config/meta-orchestrator/config.yaml) > Defaults + Env
func (r *CascadingConfigResolver) Resolve() (*OrchestratorConfig, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	base := GetDefaultConfig()

	// 1. Load System Config if present
	systemFile := filepath.Join(r.systemConfDir, "config.yaml")
	if sysCfg, err := loadYAMLFile(systemFile); err == nil {
		mergeConfigs(base, sysCfg)
	}

	// 2. Load Project Config if present (.sdlc/config.yaml or .meta-orchestrator.yaml)
	if r.projectRoot != "" {
		p1 := filepath.Join(r.projectRoot, ".sdlc", "config.yaml")
		p2 := filepath.Join(r.projectRoot, ".meta-orchestrator.yaml")

		if projCfg, err := loadYAMLFile(p1); err == nil {
			mergeConfigs(base, projCfg)
		} else if projCfg, err := loadYAMLFile(p2); err == nil {
			mergeConfigs(base, projCfg)
		}
	}

	// 3. Environment Variable overrides
	applyEnvOverrides(base)

	r.cachedConfig = base
	return base, nil
}

func loadYAMLFile(path string) (*OrchestratorConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg OrchestratorConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse YAML at %s: %w", path, err)
	}
	return &cfg, nil
}

func mergeConfigs(target *OrchestratorConfig, override *OrchestratorConfig) {
	if override == nil {
		return
	}

	// Merge Providers
	if override.Providers != nil {
		if target.Providers == nil {
			target.Providers = make(map[string]AIProviderConfig)
		}
		for k, v := range override.Providers {
			existing := target.Providers[k]
			if v.APIKey != "" {
				existing.APIKey = v.APIKey
			}
			if v.AuthMethod != "" {
				existing.AuthMethod = v.AuthMethod
			}
			if v.BaseURL != "" {
				existing.BaseURL = v.BaseURL
			}
			if v.Model != "" {
				existing.Model = v.Model
			}
			existing.Enabled = v.Enabled
			target.Providers[k] = existing
		}
	}

	// Merge Model Tiers
	if override.ModelTiers.Tier1Reasoning != "" {
		target.ModelTiers.Tier1Reasoning = override.ModelTiers.Tier1Reasoning
	}
	if override.ModelTiers.Tier2CodeGen != "" {
		target.ModelTiers.Tier2CodeGen = override.ModelTiers.Tier2CodeGen
	}
	if override.ModelTiers.Tier3LogParse != "" {
		target.ModelTiers.Tier3LogParse = override.ModelTiers.Tier3LogParse
	}

	// Merge Router
	if override.Router.Strategy != "" {
		target.Router.Strategy = override.Router.Strategy
	}
	if override.Router.MaxTokenBudget > 0 {
		target.Router.MaxTokenBudget = override.Router.MaxTokenBudget
	}
	if override.Router.CustomRules != "" {
		target.Router.CustomRules = override.Router.CustomRules
	}

	// Merge Tools
	if override.Tools.ToolDir != "" {
		target.Tools.ToolDir = override.Tools.ToolDir
	}

	// Merge Active SDLC
	if override.ActiveSDLC != "" {
		target.ActiveSDLC = override.ActiveSDLC
	}
}

func applyEnvOverrides(cfg *OrchestratorConfig) {
	if apiKey := os.Getenv("ANTHROPIC_API_KEY"); apiKey != "" {
		p := cfg.Providers["claude"]
		p.APIKey = apiKey
		p.Enabled = true
		cfg.Providers["claude"] = p
	}
	if apiKey := os.Getenv("OPENAI_API_KEY"); apiKey != "" {
		p := cfg.Providers["openai"]
		p.APIKey = apiKey
		p.Enabled = true
		cfg.Providers["openai"] = p
	}
	if apiKey := os.Getenv("GEMINI_API_KEY"); apiKey != "" {
		p := cfg.Providers["antigravity"]
		p.APIKey = apiKey
		p.Enabled = true
		cfg.Providers["antigravity"] = p
	}
	if strat := os.Getenv("META_ORCH_ROUTER_STRATEGY"); strat != "" {
		cfg.Router.Strategy = strat
	}
	if sdlc := os.Getenv("META_ORCH_ACTIVE_SDLC"); sdlc != "" {
		cfg.ActiveSDLC = sdlc
	}
}
