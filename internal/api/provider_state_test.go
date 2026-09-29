package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/llm"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/quota"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/telemetry"
)

// isolateProviderState gives a test its own provider globals and restores them afterwards.
func isolateProviderState(t *testing.T, path string) {
	t.Helper()
	providerAuthStore.mu.Lock()
	prevState, prevQuotas, prevPath := providerAuthStore.state, providerQuotas, providerStatePath
	prevProviders := append([]ProviderDTO(nil), defaultProviders...)
	providerAuthStore.state = map[string]*ProviderAuthState{}
	providerQuotas = map[string]ProviderQuota{}
	providerStatePath = path
	providerAuthStore.mu.Unlock()
	t.Cleanup(func() {
		providerAuthStore.mu.Lock()
		providerAuthStore.state, providerQuotas, providerStatePath = prevState, prevQuotas, prevPath
		defaultProviders = prevProviders
		providerAuthStore.mu.Unlock()
	})
}

func postProvider(t *testing.T, r *Router, body map[string]interface{}) {
	t.Helper()
	payload, _ := json.Marshal(body)
	w := httptest.NewRecorder()
	r.handleProviders(w, httptest.NewRequest(http.MethodPost, "/api/v1/providers", bytes.NewReader(payload)))
	if w.Code != http.StatusOK {
		t.Fatalf("POST /providers %v: %d %s", body, w.Code, w.Body.String())
	}
}

func TestProviderModelChangeKeepsSessionToken(t *testing.T) {
	isolateProviderState(t, "")
	r := &Router{}

	postProvider(t, r, map[string]interface{}{"provider_id": "claude", "session_token": "sess-token-value-123456", "auth_method": "session_token"})
	postProvider(t, r, map[string]interface{}{"provider_id": "claude", "model": "claude-opus-5-5"})

	s := providerAuthStore.state["claude"]
	if s.AuthMethod != "session_token" || s.SessionToken != "sess-token-value-123456" {
		t.Fatalf("model change dropped the session token: %+v", s)
	}
}

func TestProviderClearSessionTokenFallsBackToAPIKey(t *testing.T) {
	isolateProviderState(t, "")
	r := &Router{}

	postProvider(t, r, map[string]interface{}{"provider_id": "claude", "api_key": "api-key-value-1234567890"})
	postProvider(t, r, map[string]interface{}{"provider_id": "claude", "session_token": "sess-token-value-123456", "auth_method": "session_token"})
	postProvider(t, r, map[string]interface{}{"provider_id": "claude", "session_token": "", "auth_method": "api_key"})

	s := providerAuthStore.state["claude"]
	if s.AuthMethod != "api_key" || s.SessionToken != "" || s.APIKey != "api-key-value-1234567890" {
		t.Fatalf("unexpected state after clearing session token: %+v", s)
	}
}

func TestProviderStateSurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".sdlc", "providers.json")
	isolateProviderState(t, path)
	r := &Router{}

	postProvider(t, r, map[string]interface{}{"provider_id": "claude", "session_token": "sess-token-value-123456", "auth_method": "session_token"})
	postProvider(t, r, map[string]interface{}{"provider_id": "claude", "model": "claude-opus-5-5"})

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("state file not written: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("state file holds credentials; want mode 0600, got %v", info.Mode().Perm())
	}

	// Simulate a daemon restart: wipe memory, then load from disk.
	providerAuthStore.mu.Lock()
	providerAuthStore.state = map[string]*ProviderAuthState{}
	defaultProviders[0].DefaultModel = "reset"
	providerAuthStore.mu.Unlock()
	loadProviderState(path)

	s := providerAuthStore.state["claude"]
	if s == nil || s.AuthMethod != "session_token" || s.SessionToken != "sess-token-value-123456" {
		t.Fatalf("session token not restored: %+v", s)
	}
	if defaultProviders[0].DefaultModel != "claude-opus-5-5" {
		t.Fatalf("default model not restored: %q", defaultProviders[0].DefaultModel)
	}
}

func TestBuildProviderUsage(t *testing.T) {
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	ev := func(model, served, provider string, age time.Duration, in, out int64, cost float64) telemetry.TokenEvent {
		return telemetry.TokenEvent{CallMeta: telemetry.CallMeta{Model: model, ServedModel: served, Provider: provider},
			PromptTokens: in, CompletionTokens: out, CostUSD: cost, Timestamp: now.Add(-age)}
	}
	events := []telemetry.TokenEvent{
		ev("claude/claude-3-5-sonnet-20241022", "claude-opus-5-5", "anthropic", time.Hour, 1000, 500, 0.10),
		ev("claude/claude-3-5-sonnet-20241022", "", "anthropic", 3*24*time.Hour, 200, 100, 0.02),
		ev("gpt-4o", "", "openai", time.Hour, 50, 50, 0.01),
		ev("claude/claude-3-5-sonnet-20241022", "", "anthropic", 60*24*time.Hour, 9999, 9999, 9), // outside everything
	}
	quotas := map[string]ProviderQuota{"claude": {MonthlyTokenLimit: 3600}}
	out := buildProviderUsage(events, []string{"claude", "chatgpt", "opencode"}, quotas, now, 7*24*time.Hour)

	byID := map[string]ProviderUsage{}
	for _, u := range out {
		byID[u.ProviderID] = u
	}
	c := byID["claude"]
	if c.Window.Calls != 2 || c.Window.TotalTokens != 1800 {
		t.Fatalf("claude 7d window should exclude the 60-day-old call: %+v", c.Window)
	}
	if c.Last24h.Calls != 1 || c.Last24h.TotalTokens != 1500 {
		t.Fatalf("claude last 24h: %+v", c.Last24h)
	}
	if c.MonthToDate.TotalTokens != 1800 {
		t.Fatalf("claude month-to-date: %+v", c.MonthToDate)
	}
	if c.Quota.TokenPercent == nil || *c.Quota.TokenPercent != 50 || c.Quota.Exceeded {
		t.Fatalf("claude quota: %+v", c.Quota)
	}
	if len(c.Models) != 2 || c.Models[0].Model != "claude-opus-5-5" {
		t.Fatalf("claude models should list the served model first: %+v", c.Models)
	}
	if byID["chatgpt"].Window.Calls != 1 {
		t.Fatalf("unprefixed openai model should map to chatgpt: %+v", byID["chatgpt"])
	}
	if byID["opencode"].Window.Calls != 0 || byID["opencode"].Quota.TokenPercent != nil {
		t.Fatalf("unused provider should be empty with no quota: %+v", byID["opencode"])
	}
}

func TestServedModel(t *testing.T) {
	cases := []struct {
		used, reported, want string
	}{
		{"claude/claude-3-5-sonnet-20241022", "claude-opus-5-5", "claude-opus-5-5"},
		{"claude/claude-opus-5-5", "claude-opus-5-5", "claude/claude-opus-5-5"},
		{"claude/claude-3-5-sonnet-20241022", "claude-code", "claude/claude-3-5-sonnet-20241022"},
		{"gpt-4o", "", "gpt-4o"},
	}
	for _, c := range cases {
		if got := servedModel(c.used, &llm.LLMResponse{Model: c.reported}); got != c.want {
			t.Errorf("servedModel(%q, %q) = %q, want %q", c.used, c.reported, got, c.want)
		}
	}
}

type fakeQuotaFetcher struct{}

func (fakeQuotaFetcher) Fetch(context.Context) quota.ProviderLimits {
	return quota.ProviderLimits{Status: quota.StatusOK, Plan: "max", Windows: []quota.Window{{ID: "session", Label: "Current session (5h)", UsedPercent: 42}}}
}

func TestProviderUsageIncludesPlanLimits(t *testing.T) {
	isolateProviderState(t, "")
	r := NewRouter(RouterConfig{RootDir: t.TempDir(), QuotaFetchers: map[string]quota.Fetcher{"claude": fakeQuotaFetcher{}}})
	w := httptest.NewRecorder()
	r.handleProviderUsage(w, httptest.NewRequest(http.MethodGet, "/api/v1/providers/usage?window=24h", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("GET /providers/usage: %d %s", w.Code, w.Body.String())
	}
	var body struct {
		Providers []ProviderUsage `json:"providers"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	for _, p := range body.Providers {
		switch p.ProviderID {
		case "claude":
			if p.Limits == nil || p.Limits.Status != quota.StatusOK || p.Limits.Windows[0].UsedPercent != 42 {
				t.Fatalf("claude limits: %+v", p.Limits)
			}
		case "opencode":
			if p.Limits == nil || p.Limits.Status != quota.StatusUnavailable {
				t.Fatalf("opencode should report unavailable limits: %+v", p.Limits)
			}
		}
	}
}
