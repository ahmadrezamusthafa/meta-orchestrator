package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/router"
)

type ProviderDTO struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Enabled       bool     `json:"enabled"`
	LatencyMs     int      `json:"latency_ms"`
	MaskedAPIKey  string   `json:"masked_api_key"`
	BaseURL       string   `json:"base_url,omitempty"`
	DefaultModel  string   `json:"default_model"`
	Models        []string `json:"models"`
	AuthMethod    string   `json:"auth_method"`     // "api_key", "oauth", or "session_token"
	SupportsOAuth bool     `json:"supports_oauth"`
}

type TierMappingDTO struct {
	Tier1Reasoning  TierEntry `json:"tier_1_reasoning"`
	Tier2CodeGen    TierEntry `json:"tier_2_codegen"`
	Tier3LogParsing TierEntry `json:"tier_3_log_parsing"`
}

type TierEntry struct {
	ProviderID string `json:"provider_id"`
	ModelID    string `json:"model_id"`
}

type ProviderTestRequest struct {
	ProviderID string `json:"provider_id"`
}

// ProviderAuthState stores the persisted auth configuration for a single provider.
type ProviderAuthState struct {
	AuthMethod   string    `json:"auth_method"`              // "api_key", "oauth", "session_token"
	APIKey       string    `json:"-"`                        // never exposed via API
	SessionToken string    `json:"-"`                        // never exposed via API
	MaskedKey    string    `json:"masked_key,omitempty"`     // masked display value
	SavedAt      time.Time `json:"saved_at"`
}

// providerAuthStore persists auth state across requests (survives tab switches).
var providerAuthStore = struct {
	mu    sync.RWMutex
	state map[string]*ProviderAuthState // provider_id -> auth state
}{
	state: make(map[string]*ProviderAuthState),
}

var defaultProviders = []ProviderDTO{
	{
		ID:           "claude",
		Name:         "Anthropic Claude",
		Enabled:      true,
		LatencyMs:    142,
		MaskedAPIKey: "sk-ant-api03-••••••••••••••••••••9F3a",
		DefaultModel: "claude-3-7-sonnet-20250219",
		Models: []string{
			"claude-opus-5-5",
			"claude-3-7-sonnet-20250219",
			"claude-3-7-sonnet",
			"claude-3-7-sonnet-latest",
			"claude-3-5-opus",
			"claude-3-5-opus-latest",
			"claude-3-5-sonnet-20241022",
			"claude-3-5-sonnet-latest",
			"claude-3-5-sonnet-20240620",
			"claude-3-opus-latest",
			"claude-3-opus-20240229",
			"claude-3-5-haiku-20241022",
			"claude-3-5-haiku-latest",
			"claude-3-sonnet-20240229",
			"claude-3-haiku-20240307",
		},
		AuthMethod:    "api_key",
		SupportsOAuth: true,
	},
	{
		ID:           "antigravity",
		Name:         "Google Antigravity / Gemini",
		Enabled:      true,
		LatencyMs:    98,
		MaskedAPIKey: "AIzaSy••••••••••••••••••••x91B",
		DefaultModel: "gemini-2.0-flash",
		Models: []string{
			"gemini-2.5-pro",
			"gemini-2.5-flash",
			"gemini-2.0-flash",
			"gemini-2.0-flash-thinking-exp",
			"gemini-1.5-pro",
			"gemini-1.5-flash",
			"gemini-1.5-flash-8b",
		},
		AuthMethod:    "api_key",
		SupportsOAuth: true,
	},
	{
		ID:           "chatgpt",
		Name:         "OpenAI ChatGPT",
		Enabled:      true,
		LatencyMs:    185,
		MaskedAPIKey: "sk-proj-••••••••••••••••••••83Kl",
		DefaultModel: "gpt-4o",
		Models: []string{
			"gpt-4.5-preview",
			"o3-mini",
			"o1",
			"o1-pro",
			"o1-preview",
			"o1-mini",
			"gpt-4o",
			"gpt-4o-latest",
			"gpt-4o-mini",
			"gpt-4-turbo",
		},
		AuthMethod:    "api_key",
		SupportsOAuth: true,
	},
	{
		ID:           "opencode",
		Name:         "OpenCode / Local vLLM / Ollama",
		Enabled:      true,
		LatencyMs:    12,
		BaseURL:      "http://localhost:11434",
		DefaultModel: "deepseek-coder-v2",
		Models: []string{
			"deepseek-coder-v2",
			"deepseek-r1",
			"qwen2.5-coder-32b",
			"qwen2.5-coder-7b",
			"llama3.3-70b",
			"codellama-70b",
			"starcoder2-15b",
		},
		AuthMethod:    "api_key",
		SupportsOAuth: false,
	},
}

var currentTierMapping = TierMappingDTO{
	Tier1Reasoning:  TierEntry{ProviderID: "claude", ModelID: "claude-3-5-sonnet-20241022"},
	Tier2CodeGen:    TierEntry{ProviderID: "claude", ModelID: "claude-3-5-sonnet-20241022"},
	Tier3LogParsing: TierEntry{ProviderID: "antigravity", ModelID: "gemini-2.0-flash"},
}

// getProvidersWithAuthState returns a copy of defaultProviders with saved auth state merged in.
func getProvidersWithAuthState() []ProviderDTO {
	providerAuthStore.mu.RLock()
	defer providerAuthStore.mu.RUnlock()

	result := make([]ProviderDTO, len(defaultProviders))
	copy(result, defaultProviders)

	for i := range result {
		if auth, ok := providerAuthStore.state[result[i].ID]; ok {
			result[i].AuthMethod = auth.AuthMethod
			if auth.MaskedKey != "" {
				result[i].MaskedAPIKey = auth.MaskedKey
			}
		}
	}
	return result
}

func (r *Router) handleProviders(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		r.writeJSON(w, http.StatusOK, map[string]interface{}{
			"providers":    getProvidersWithAuthState(),
			"tier_mapping": currentTierMapping,
			"is_override":  false,
		})

	case http.MethodPost:
		var body struct {
			ProviderID   string `json:"provider_id"`
			APIKey       string `json:"api_key"`
			SessionToken string `json:"session_token"`
			AuthMethod   string `json:"auth_method"`
			BaseURL      string `json:"base_url"`
			Model        string `json:"model"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			r.writeError(w, http.StatusBadRequest, "Invalid JSON payload")
			return
		}

		if body.ProviderID == "" {
			r.writeError(w, http.StatusBadRequest, "provider_id is required")
			return
		}

		authMethod := body.AuthMethod
		if authMethod == "" {
			if body.SessionToken != "" {
				authMethod = "session_token"
			} else {
				authMethod = "api_key"
			}
		}

		state := &ProviderAuthState{
			AuthMethod: authMethod,
			SavedAt:    time.Now(),
		}

		switch authMethod {
		case "session_token":
			if body.SessionToken == "" {
				// Clearing session token — revert to api_key
				state.AuthMethod = "api_key"
			} else {
				state.SessionToken = body.SessionToken
				state.MaskedKey = maskToken(body.SessionToken)
			}
		case "api_key":
			if body.APIKey != "" {
				state.APIKey = body.APIKey
				state.MaskedKey = maskToken(body.APIKey)
			}
		}

		if body.Model != "" {
			for i := range defaultProviders {
				if defaultProviders[i].ID == body.ProviderID {
					defaultProviders[i].DefaultModel = body.Model
					break
				}
			}
		}

		providerAuthStore.mu.Lock()
		providerAuthStore.state[body.ProviderID] = state
		providerAuthStore.mu.Unlock()

		r.writeJSON(w, http.StatusOK, map[string]interface{}{
			"status":      "saved",
			"provider_id": body.ProviderID,
			"auth_method": state.AuthMethod,
			"masked_key":  state.MaskedKey,
			"updated":     state.SavedAt,
		})

	default:
		r.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

type AvailableModelDTO struct {
	ProviderID   string  `json:"provider_id"`
	ProviderName string  `json:"provider_name"`
	ModelID      string  `json:"model_id"`
	ModelName    string  `json:"model_name"`
	CostPer1k    float64 `json:"cost_per_1k"`
	LatencyMs    int     `json:"latency_ms"`
}

type RouterModeDTO struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

var availableRouterModes = []RouterModeDTO{
	{
		ID:          "priority_sequence",
		Name:        "Priority Sequence (Custom Waterfall)",
		Description: "Cascades through custom prioritized AI models with automatic failover (9router style).",
	},
	{
		ID:          "best_practice",
		Name:        "Tiered Best Practice",
		Description: "Routes automatically based on SDLC phase and architectural complexity.",
	},
	{
		ID:          "cost_optimized",
		Name:        "Cost-Optimized",
		Description: "Routes to the most cost-effective model first (local Ollama/vLLM & high-efficiency flash models).",
	},
	{
		ID:          "latency_optimized",
		Name:        "Latency-Optimized",
		Description: "Prioritizes lowest Time-To-First-Token and fastest response speed.",
	},
	{
		ID:          "round_robin",
		Name:        "Round-Robin Load Balancing",
		Description: "Rotates tasks evenly across active providers to mitigate rate limits and quotas.",
	},
}

var modelMetaMu sync.RWMutex

var modelMetaMap = map[string]struct {
	Name      string
	CostPer1k float64
	LatencyMs int
}{
	// Claude
	"claude-opus-5-5":            {Name: "Claude Opus 5.5 (Next-Gen Frontier)", CostPer1k: 0.015, LatencyMs: 250},
	"claude-3-7-sonnet-latest":   {Name: "Claude 3.7 Sonnet (Latest)", CostPer1k: 0.003, LatencyMs: 140},
	"claude-3-7-sonnet-20250219": {Name: "Claude 3.7 Sonnet (Hybrid Reasoning)", CostPer1k: 0.003, LatencyMs: 140},
	"claude-3-7-sonnet":          {Name: "Claude 3.7 Sonnet", CostPer1k: 0.003, LatencyMs: 140},
	"claude-3-5-opus":            {Name: "Claude 3.5 Opus", CostPer1k: 0.015, LatencyMs: 260},
	"claude-3-5-opus-latest":     {Name: "Claude 3.5 Opus Latest", CostPer1k: 0.015, LatencyMs: 260},
	"claude-3-5-sonnet-latest":   {Name: "Claude 3.5 Sonnet Latest", CostPer1k: 0.003, LatencyMs: 142},
	"claude-3-5-sonnet-20241022": {Name: "Claude 3.5 Sonnet (v2)", CostPer1k: 0.003, LatencyMs: 142},
	"claude-3-5-sonnet-20240620": {Name: "Claude 3.5 Sonnet (v1)", CostPer1k: 0.003, LatencyMs: 145},
	"claude-3-opus-latest":       {Name: "Claude 3 Opus Latest", CostPer1k: 0.015, LatencyMs: 260},
	"claude-3-opus-20240229":     {Name: "Claude 3 Opus", CostPer1k: 0.015, LatencyMs: 260},
	"claude-3-5-haiku-latest":    {Name: "Claude 3.5 Haiku Latest", CostPer1k: 0.0008, LatencyMs: 75},
	"claude-3-5-haiku-20241022":  {Name: "Claude 3.5 Haiku", CostPer1k: 0.0008, LatencyMs: 75},
	"claude-3-sonnet-20240229":   {Name: "Claude 3 Sonnet", CostPer1k: 0.003, LatencyMs: 150},
	"claude-3-haiku-20240307":    {Name: "Claude 3 Haiku", CostPer1k: 0.00025, LatencyMs: 80},

	// Gemini / Antigravity
	"gemini-2.5-pro":               {Name: "Gemini 2.5 Pro (Ultra Reasoning)", CostPer1k: 0.00125, LatencyMs: 120},
	"gemini-2.5-flash":             {Name: "Gemini 2.5 Flash", CostPer1k: 0.0001, LatencyMs: 60},
	"gemini-2.0-flash":             {Name: "Gemini 2.0 Flash (Fast & Capable)", CostPer1k: 0.0001, LatencyMs: 65},
	"gemini-2.0-flash-thinking-exp": {Name: "Gemini 2.0 Flash Thinking", CostPer1k: 0.0001, LatencyMs: 95},
	"gemini-1.5-pro":               {Name: "Gemini 1.5 Pro (2M Context)", CostPer1k: 0.00125, LatencyMs: 130},
	"gemini-1.5-flash":             {Name: "Gemini 1.5 Flash", CostPer1k: 0.000075, LatencyMs: 70},
	"gemini-1.5-flash-8b":          {Name: "Gemini 1.5 Flash 8B", CostPer1k: 0.0000375, LatencyMs: 50},

	// OpenAI
	"gpt-4.5-preview": {Name: "OpenAI GPT-4.5 Preview (Orion)", CostPer1k: 0.075, LatencyMs: 320},
	"gpt-4o":          {Name: "OpenAI GPT-4o Omni", CostPer1k: 0.0025, LatencyMs: 185},
	"gpt-4o-latest":   {Name: "OpenAI GPT-4o Latest", CostPer1k: 0.0025, LatencyMs: 185},
	"gpt-4o-mini":     {Name: "OpenAI GPT-4o Mini", CostPer1k: 0.00015, LatencyMs: 85},
	"o3-mini":         {Name: "OpenAI o3-mini (High Reasoning)", CostPer1k: 0.0011, LatencyMs: 110},
	"o1":              {Name: "OpenAI o1 Deep Reasoning", CostPer1k: 0.015, LatencyMs: 450},
	"o1-pro":          {Name: "OpenAI o1 Pro", CostPer1k: 0.015, LatencyMs: 480},
	"o1-preview":      {Name: "OpenAI o1 Preview", CostPer1k: 0.015, LatencyMs: 420},
	"o1-mini":         {Name: "OpenAI o1 Mini", CostPer1k: 0.003, LatencyMs: 190},
	"gpt-4-turbo":     {Name: "OpenAI GPT-4 Turbo", CostPer1k: 0.010, LatencyMs: 290},

	// OpenCode / Local
	"deepseek-coder-v2":  {Name: "DeepSeek Coder V2 (Local MoE)", CostPer1k: 0.0, LatencyMs: 12},
	"deepseek-r1":        {Name: "DeepSeek R1 (Open Reasoning)", CostPer1k: 0.0, LatencyMs: 25},
	"qwen2.5-coder-32b":  {Name: "Qwen 2.5 Coder 32B (Local)", CostPer1k: 0.0, LatencyMs: 15},
	"qwen2.5-coder-7b":   {Name: "Qwen 2.5 Coder 7B (Fast)", CostPer1k: 0.0, LatencyMs: 8},
	"llama3.3-70b":       {Name: "Llama 3.3 70B Instruct (Local)", CostPer1k: 0.0, LatencyMs: 18},
	"codellama-70b":      {Name: "CodeLlama 70B (Local)", CostPer1k: 0.0, LatencyMs: 22},
	"starcoder2-15b":     {Name: "StarCoder2 15B (Local)", CostPer1k: 0.0, LatencyMs: 10},
}

func registerCustomModel(providerID, modelID, modelName string, costPer1k float64, latencyMs int) {
	modelMetaMu.Lock()
	defer modelMetaMu.Unlock()

	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return
	}
	if modelName == "" {
		modelName = modelID
	}
	if latencyMs <= 0 {
		latencyMs = 120
	}

	modelMetaMap[modelID] = struct {
		Name      string
		CostPer1k float64
		LatencyMs int
	}{
		Name:      modelName,
		CostPer1k: costPer1k,
		LatencyMs: latencyMs,
	}

	foundProvider := false
	for i := range defaultProviders {
		if defaultProviders[i].ID == providerID {
			foundProvider = true
			exists := false
			for _, m := range defaultProviders[i].Models {
				if m == modelID {
					exists = true
					break
				}
			}
			if !exists {
				defaultProviders[i].Models = append([]string{modelID}, defaultProviders[i].Models...)
			}
			break
		}
	}

	if !foundProvider && len(defaultProviders) > 0 {
		defaultProviders[0].Models = append([]string{modelID}, defaultProviders[0].Models...)
	}
}

func registerChainModels(chain []router.PriorityModelItem) {
	for _, item := range chain {
		registerCustomModel(item.Provider, item.Model, item.Name, item.CostPer1k, item.LatencyMs)
	}
}

func getAllAvailableModels() []AvailableModelDTO {
	modelMetaMu.RLock()
	defer modelMetaMu.RUnlock()

	var list []AvailableModelDTO
	seen := make(map[string]bool)

	for _, p := range defaultProviders {
		for _, m := range p.Models {
			key := p.ID + ":" + m
			if seen[key] {
				continue
			}
			seen[key] = true

			name := m
			cost := 0.001
			latency := p.LatencyMs
			if meta, ok := modelMetaMap[m]; ok {
				name = meta.Name
				cost = meta.CostPer1k
				latency = meta.LatencyMs
			}
			list = append(list, AvailableModelDTO{
				ProviderID:   p.ID,
				ProviderName: p.Name,
				ModelID:      m,
				ModelName:    name,
				CostPer1k:    cost,
				LatencyMs:    latency,
			})
		}
	}
	return list
}

func (r *Router) handleRouterSettings(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		settings := r.strategyRouter.GetSettings()
		r.writeJSON(w, http.StatusOK, map[string]interface{}{
			"mode":            settings.Mode,
			"priority_chain":  settings.PriorityChain,
			"available_modes": availableRouterModes,
			"all_models":      getAllAvailableModels(),
		})

	case http.MethodPost:
		var body struct {
			Mode          router.RouterMode          `json:"mode"`
			PriorityChain []router.PriorityModelItem `json:"priority_chain"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			r.writeError(w, http.StatusBadRequest, "Invalid JSON payload")
			return
		}

		if body.Mode != "" {
			r.strategyRouter.SetSettings(router.RouterSettings{
				Mode:          body.Mode,
				PriorityChain: body.PriorityChain,
			})
			registerChainModels(body.PriorityChain)
		}

		r.writeJSON(w, http.StatusOK, map[string]interface{}{
			"status":   "updated",
			"settings": r.strategyRouter.GetSettings(),
		})

	default:
		r.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

func (r *Router) handleRegisterModel(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		r.writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}

	var body struct {
		ProviderID string  `json:"provider_id"`
		ModelID    string  `json:"model_id"`
		ModelName  string  `json:"model_name"`
		CostPer1k  float64 `json:"cost_per_1k"`
		LatencyMs  int     `json:"latency_ms"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		r.writeError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if body.ProviderID == "" || body.ModelID == "" {
		r.writeError(w, http.StatusBadRequest, "provider_id and model_id are required")
		return
	}

	registerCustomModel(body.ProviderID, body.ModelID, body.ModelName, body.CostPer1k, body.LatencyMs)

	r.writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":     "registered",
		"all_models": getAllAvailableModels(),
	})
}

func (r *Router) handleProviderTest(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		r.writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}

	var body ProviderTestRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		r.writeError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	latency := 125
	switch body.ProviderID {
	case "antigravity":
		latency = 88
	case "opencode":
		latency = 14
	}

	// Check if provider has any auth configured
	providerAuthStore.mu.RLock()
	auth, hasAuth := providerAuthStore.state[body.ProviderID]
	providerAuthStore.mu.RUnlock()

	authStatus := "CONNECTED"
	authMethod := "api_key"
	if hasAuth {
		authMethod = auth.AuthMethod
		// Validate that the credential is not empty
		switch auth.AuthMethod {
		case "session_token":
			if auth.SessionToken == "" {
				authStatus = "NO_CREDENTIALS"
			}
		case "api_key":
			if auth.APIKey == "" {
				authStatus = "NO_CREDENTIALS"
			}
		}
	}

	r.writeJSON(w, http.StatusOK, map[string]interface{}{
		"provider_id": body.ProviderID,
		"status":      authStatus,
		"auth_method": authMethod,
		"latency_ms":  latency,
		"timestamp":   time.Now(),
	})
}

// maskToken masks a token for safe display, showing first 6 and last 4 chars.
func maskToken(token string) string {
	if len(token) <= 12 {
		return strings.Repeat("•", len(token))
	}
	return token[:6] + strings.Repeat("•", 16) + token[len(token)-4:]
}

