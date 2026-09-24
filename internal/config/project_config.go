package config

// AIProviderConfig defines credentials and settings for an LLM provider.
type AIProviderConfig struct {
	Enabled bool   `json:"enabled" yaml:"enabled"`
	APIKey  string `json:"api_key" yaml:"api_key"`
	BaseURL string `json:"base_url,omitempty" yaml:"base_url,omitempty"`
	Model   string `json:"model" yaml:"model"`
}

// ModelTiersConfig defines model allocations per reasoning/codegen tier.
type ModelTiersConfig struct {
	Tier1Reasoning string `json:"tier1_reasoning" yaml:"tier1_reasoning"` // Architecture, PRD, ATDD
	Tier2CodeGen   string `json:"tier2_codegen" yaml:"tier2_codegen"`     // Code Implementation
	Tier3LogParse  string `json:"tier3_log_parse" yaml:"tier3_log_parse"` // Compiler logs, diff summaries
}

// RouterConfig defines routing strategy and token budgets.
type RouterConfig struct {
	Strategy       string `json:"strategy" yaml:"strategy"` // "best_practice" or "custom"
	MaxTokenBudget int64  `json:"max_token_budget" yaml:"max_token_budget"`
	CustomRules    string `json:"custom_rules,omitempty" yaml:"custom_rules,omitempty"`
}

// ToolConfig settings for tool management.
type ToolConfig struct {
	AutoUpdate bool   `json:"auto_update" yaml:"auto_update"`
	ToolDir    string `json:"tool_dir" yaml:"tool_dir"`
}

// OrchestratorConfig represents unified orchestrator settings.
type OrchestratorConfig struct {
	Providers   map[string]AIProviderConfig `json:"providers" yaml:"providers"`
	ModelTiers  ModelTiersConfig            `json:"model_tiers" yaml:"model_tiers"`
	Router      RouterConfig                `json:"router" yaml:"router"`
	Tools       ToolConfig                  `json:"tools" yaml:"tools"`
	ActiveSDLC  string                      `json:"active_sdlc" yaml:"active_sdlc"`
	WorkspaceID string                      `json:"workspace_id,omitempty" yaml:"workspace_id,omitempty"`
}
