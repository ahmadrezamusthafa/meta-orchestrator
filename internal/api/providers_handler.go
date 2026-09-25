package api

import (
	"encoding/json"
	"net/http"
	"time"
)

type ProviderDTO struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Enabled      bool     `json:"enabled"`
	LatencyMs    int      `json:"latency_ms"`
	MaskedAPIKey string   `json:"masked_api_key"`
	BaseURL      string   `json:"base_url,omitempty"`
	DefaultModel string   `json:"default_model"`
	Models       []string `json:"models"`
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

var defaultProviders = []ProviderDTO{
	{
		ID:           "claude",
		Name:         "Anthropic Claude",
		Enabled:      true,
		LatencyMs:    142,
		MaskedAPIKey: "sk-ant-api03-••••••••••••••••••••9F3a",
		DefaultModel: "claude-3-5-sonnet-20241022",
		Models:       []string{"claude-3-5-sonnet-20241022", "claude-3-5-haiku-20241022", "claude-3-opus-20240229"},
	},
	{
		ID:           "antigravity",
		Name:         "Google Antigravity / Gemini",
		Enabled:      true,
		LatencyMs:    98,
		MaskedAPIKey: "AIzaSy••••••••••••••••••••x91B",
		DefaultModel: "gemini-2.0-flash",
		Models:       []string{"gemini-2.0-flash", "gemini-1.5-pro", "gemini-1.5-flash"},
	},
	{
		ID:           "chatgpt",
		Name:         "OpenAI ChatGPT",
		Enabled:      true,
		LatencyMs:    185,
		MaskedAPIKey: "sk-proj-••••••••••••••••••••83Kl",
		DefaultModel: "gpt-4o",
		Models:       []string{"gpt-4o", "gpt-4o-mini", "o1-preview"},
	},
	{
		ID:           "opencode",
		Name:         "OpenCode / Local vLLM",
		Enabled:      false,
		LatencyMs:    12,
		BaseURL:      "http://localhost:8000/v1",
		DefaultModel: "deepseek-coder-v2",
		Models:       []string{"deepseek-coder-v2", "qwen2.5-coder-32b", "starcoder2-15b"},
	},
}

var currentTierMapping = TierMappingDTO{
	Tier1Reasoning:  TierEntry{ProviderID: "claude", ModelID: "claude-3-5-sonnet-20241022"},
	Tier2CodeGen:    TierEntry{ProviderID: "claude", ModelID: "claude-3-5-sonnet-20241022"},
	Tier3LogParsing: TierEntry{ProviderID: "antigravity", ModelID: "gemini-2.0-flash"},
}

func (r *Router) handleProviders(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		r.writeJSON(w, http.StatusOK, map[string]interface{}{
			"providers":    defaultProviders,
			"tier_mapping": currentTierMapping,
			"is_override":  false,
		})

	case http.MethodPost:
		var body map[string]interface{}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			r.writeError(w, http.StatusBadRequest, "Invalid JSON payload")
			return
		}
		r.writeJSON(w, http.StatusOK, map[string]interface{}{
			"status":  "saved",
			"updated": time.Now(),
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

	r.writeJSON(w, http.StatusOK, map[string]interface{}{
		"provider_id": body.ProviderID,
		"status":      "CONNECTED",
		"latency_ms":  latency,
		"timestamp":   time.Now(),
	})
}
