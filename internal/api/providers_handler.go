package api

import (
	"encoding/json"
	"fmt"
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
	HasAPIKey     bool     `json:"has_api_key"`
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
		DefaultModel: "claude-sonnet-5-5",
		Models: []string{
			"claude-opus-5-5",
			"claude-sonnet-5-5",
			"claude-haiku-4-5",
			"claude-fable-5-1",
			// Earlier generations, kept so saved settings that name them still resolve.
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

// tierMapping is the model each tier actually resolves to under the current priority chain.
func (r *Router) tierMapping() TierMappingDTO {
	_, tiers := r.strategyRouter.Preview(nil)
	entry := func(model string) TierEntry {
		p, m, ok := strings.Cut(model, "/")
		if !ok {
			return TierEntry{ProviderID: "claude", ModelID: model}
		}
		return TierEntry{ProviderID: p, ModelID: m}
	}
	var out TierMappingDTO
	for _, t := range tiers {
		switch t.Tier {
		case router.TierReasoning:
			out.Tier1Reasoning = entry(t.Model)
		case router.TierCodeGen:
			out.Tier2CodeGen = entry(t.Model)
		case router.TierLogParse:
			out.Tier3LogParsing = entry(t.Model)
		}
	}
	return out
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
			result[i].HasAPIKey = auth.APIKey != ""
			if auth.APIKey != "" {
				result[i].MaskedAPIKey = maskToken(auth.APIKey)
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
			"tier_mapping": r.tierMapping(),
			"is_override":  false,
		})

	case http.MethodPost:
		var body struct {
			ProviderID string `json:"provider_id"`
			APIKey     string `json:"api_key"`
			// SessionToken is a pointer so an explicit "" (clear) differs from "not sent".
			SessionToken *string `json:"session_token"`
			AuthMethod   string  `json:"auth_method"`
			BaseURL      string  `json:"base_url"`
			Model        string  `json:"model"`
			ClearAPIKey  bool    `json:"clear_api_key"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			r.writeError(w, http.StatusBadRequest, "Invalid JSON payload")
			return
		}

		if body.ProviderID == "" {
			r.writeError(w, http.StatusBadRequest, "provider_id is required")
			return
		}

		body.APIKey = strings.TrimSpace(body.APIKey)
		if body.APIKey != "" && r.connections != nil {
			// Verify before saving: a key the provider rejects is never stored.
			pc := snapshotCreds(body.ProviderID)
			pc.apiKey = body.APIKey
			if conn := r.connections.check(req.Context(), body.ProviderID, pc, true); conn.Status == ConnInvalid {
				r.writeError(w, http.StatusBadRequest, conn.Detail)
				return
			}
		}

		providerAuthStore.mu.Lock()
		prev := providerAuthStore.state[body.ProviderID]
		authChange := body.AuthMethod != "" || body.APIKey != "" || body.SessionToken != nil || body.ClearAPIKey
		state := applyProviderAuthUpdate(prev, body.AuthMethod, body.APIKey, body.SessionToken)
		if body.ClearAPIKey {
			state.APIKey = ""
			if state.AuthMethod == "api_key" {
				state.MaskedKey = ""
			}
			state.SavedAt = time.Now()
		}
		if prev != nil || authChange {
			providerAuthStore.state[body.ProviderID] = state // a model-only change creates no auth record
		}
		if body.Model != "" {
			for i := range defaultProviders {
				if defaultProviders[i].ID == body.ProviderID {
					defaultProviders[i].DefaultModel = body.Model
					break
				}
			}
		}
		saveErr := saveProviderStateLocked()
		resp := map[string]interface{}{
			"status":      "saved",
			"provider_id": body.ProviderID,
			"auth_method": state.AuthMethod,
			"masked_key":  state.MaskedKey,
			"updated":     state.SavedAt,
		}
		providerAuthStore.mu.Unlock()

		if r.connections != nil {
			r.connections.invalidate(body.ProviderID)
			if authChange {
				resp["connection"] = r.connections.Check(req.Context(), body.ProviderID, true)
			}
		}
		if saveErr != nil {
			// The change is live for this daemon run but will not survive a restart — say so.
			fmt.Printf("[Providers] State not persisted: %v\n", saveErr)
			resp["persisted"] = false
			resp["warning"] = "saved for this session only: " + saveErr.Error()
		}
		r.writeJSON(w, http.StatusOK, resp)

	default:
		r.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

type AvailableModelDTO struct {
	ProviderID   string  `json:"provider_id"`
	ProviderName string  `json:"provider_name"`
	ModelID      string  `json:"model_id"`
	ModelName    string  `json:"model_name"`
	CostPer1k    float64  `json:"cost_per_1k"`
	LatencyMs    int      `json:"latency_ms"`
	Tiers        []string `json:"tiers"` // inferred tier tags, the default when added to the chain
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
		Description: "Always starts at rank #1 of your chain and fails over down the list.",
	},
	{
		ID:          "best_practice",
		Name:        "Tiered Best Practice",
		Description: "Picks the best chain model per SDLC stage and complexity; suggests stronger models you have not allowed.",
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
	// CostPer1k is the input list price per 1k tokens.
	"claude-opus-5-5":            {Name: "Claude Opus 5.5", CostPer1k: 0.004, LatencyMs: 250},
	"claude-sonnet-5-5":          {Name: "Claude Sonnet 5.5", CostPer1k: 0.002, LatencyMs: 140},
	"claude-haiku-4-5":           {Name: "Claude Haiku 4.5", CostPer1k: 0.001, LatencyMs: 75},
	"claude-fable-5-1":           {Name: "Claude Fable 5.1", CostPer1k: 0.010, LatencyMs: 320},
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
				Tiers:        router.InferTiers(m),
			})
		}
	}
	return list
}

func (r *Router) handleRouterSettings(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		settings := r.strategyRouter.GetSettings()
		preview, tiers := r.strategyRouter.Preview(nil)
		r.writeJSON(w, http.StatusOK, map[string]interface{}{
			"mode":            settings.Mode,
			"priority_chain":  settings.PriorityChain,
			"default_chain":   router.DefaultPriorityChain(),
			"available_modes": availableRouterModes,
			"all_models":      getAllAvailableModels(),
			"preview":         preview,
			"tiers":           tiers,
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
			if err := saveRouterSettings(r.strategyRouter.GetSettings()); err != nil {
				r.writeError(w, http.StatusInternalServerError, fmt.Sprintf("settings applied but not saved: %v", err))
				return
			}
		}

		preview, tiers := r.strategyRouter.Preview(nil)
		r.writeJSON(w, http.StatusOK, map[string]interface{}{
			"status":   "updated",
			"settings": r.strategyRouter.GetSettings(),
			"preview":  preview,
			"tiers":    tiers,
		})

	default:
		r.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// handleRouterPreview shows what an unsaved mode/chain would route to, without applying it.
func (r *Router) handleRouterPreview(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		r.writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var body router.RouterSettings
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		r.writeError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}
	preview, tiers := r.strategyRouter.Preview(&body)
	r.writeJSON(w, http.StatusOK, map[string]interface{}{"preview": preview, "tiers": tiers})
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

	if body.ProviderID == "" {
		r.writeError(w, http.StatusBadRequest, "provider_id is required")
		return
	}
	conn := r.connections.Check(req.Context(), body.ProviderID, true)
	r.writeJSON(w, http.StatusOK, map[string]interface{}{
		"provider_id": body.ProviderID,
		"status":      conn.Status,
		"auth_method": conn.Method,
		"latency_ms":  conn.LatencyMs,
		"connection":  conn,
		"timestamp":   conn.CheckedAt,
	})
}

// applyProviderAuthUpdate merges one POST /providers into the provider's existing auth state.
// Fields the request leaves out keep their saved value: changing only the default model must not
// drop a saved session token or API key.
func applyProviderAuthUpdate(prev *ProviderAuthState, authMethod, apiKey string, sessionToken *string) *ProviderAuthState {
	state := &ProviderAuthState{AuthMethod: "api_key"}
	if prev != nil {
		cp := *prev
		state = &cp
	}
	if authMethod == "" && apiKey == "" && sessionToken == nil {
		return state // e.g. a model-only update
	}

	if apiKey != "" {
		state.APIKey = apiKey
	}
	if sessionToken != nil {
		state.SessionToken = *sessionToken
	}
	switch {
	case authMethod != "":
		state.AuthMethod = authMethod
	case sessionToken != nil && *sessionToken != "":
		state.AuthMethod = "session_token"
	case apiKey != "":
		state.AuthMethod = "api_key"
	}
	if state.AuthMethod == "session_token" && state.SessionToken == "" {
		state.AuthMethod = "api_key" // clearing the session token falls back to the API key
	}

	state.MaskedKey = ""
	switch state.AuthMethod {
	case "session_token":
		state.MaskedKey = maskToken(state.SessionToken)
	case "api_key":
		if state.APIKey != "" {
			state.MaskedKey = maskToken(state.APIKey)
		}
	}
	state.SavedAt = time.Now()
	return state
}

// maskToken masks a token for safe display, showing first 6 and last 4 chars.
func maskToken(token string) string {
	if len(token) <= 12 {
		return strings.Repeat("•", len(token))
	}
	return token[:6] + strings.Repeat("•", 16) + token[len(token)-4:]
}

