package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"
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
		ID:            "claude",
		Name:          "Anthropic Claude",
		Enabled:       true,
		LatencyMs:     142,
		MaskedAPIKey:  "sk-ant-api03-••••••••••••••••••••9F3a",
		DefaultModel:  "claude-3-5-sonnet-20241022",
		Models:        []string{"claude-3-5-sonnet-20241022", "claude-3-5-haiku-20241022", "claude-3-opus-20240229"},
		AuthMethod:    "api_key",
		SupportsOAuth: true,
	},
	{
		ID:            "antigravity",
		Name:          "Google Antigravity / Gemini",
		Enabled:       true,
		LatencyMs:     98,
		MaskedAPIKey:  "AIzaSy••••••••••••••••••••x91B",
		DefaultModel:  "gemini-2.0-flash",
		Models:        []string{"gemini-2.0-flash", "gemini-1.5-pro", "gemini-1.5-flash"},
		AuthMethod:    "api_key",
		SupportsOAuth: true,
	},
	{
		ID:            "chatgpt",
		Name:          "OpenAI ChatGPT",
		Enabled:       true,
		LatencyMs:     185,
		MaskedAPIKey:  "sk-proj-••••••••••••••••••••83Kl",
		DefaultModel:  "gpt-4o",
		Models:        []string{"gpt-4o", "gpt-4o-mini", "o1-preview"},
		AuthMethod:    "api_key",
		SupportsOAuth: true,
	},
	{
		ID:            "opencode",
		Name:          "OpenCode / Local vLLM",
		Enabled:       false,
		LatencyMs:     12,
		BaseURL:       "http://localhost:8000/v1",
		DefaultModel:  "deepseek-coder-v2",
		Models:        []string{"deepseek-coder-v2", "qwen2.5-coder-32b", "starcoder2-15b"},
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

