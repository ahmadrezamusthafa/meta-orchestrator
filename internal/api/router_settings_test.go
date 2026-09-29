package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/router"
)

func TestRouterSettingsSurviveRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "providers.json")
	isolateProviderState(t, path)
	cfg := RouterConfig{RootDir: t.TempDir(), ProviderStatePath: path}

	r := NewRouter(cfg)
	defer r.Close()
	payload, _ := json.Marshal(map[string]interface{}{
		"mode": "cost_optimized",
		"priority_chain": []router.PriorityModelItem{
			{ID: "a", Provider: "claude", Model: "claude-opus-5-5", Enabled: true, Tiers: []string{router.TierReasoning}},
		},
	})
	w := httptest.NewRecorder()
	r.handleRouterSettings(w, httptest.NewRequest(http.MethodPost, "/api/v1/router/settings", bytes.NewReader(payload)))
	if w.Code != http.StatusOK {
		t.Fatalf("POST settings: %d %s", w.Code, w.Body.String())
	}

	restarted := NewRouter(cfg)
	defer restarted.Close()
	got := restarted.strategyRouter.GetSettings()
	if got.Mode != router.ModeCostOptimized || len(got.PriorityChain) != 1 || got.PriorityChain[0].Model != "claude-opus-5-5" {
		t.Fatalf("settings not restored: %+v", got)
	}
	if tiers := got.PriorityChain[0].Tiers; len(tiers) != 1 || tiers[0] != router.TierReasoning {
		t.Fatalf("tier tags not restored: %v", tiers)
	}
}

func TestRouterPreviewDoesNotApply(t *testing.T) {
	isolateProviderState(t, "")
	r := NewRouter(RouterConfig{RootDir: t.TempDir()})
	defer r.Close()
	before := r.strategyRouter.GetSettings()

	payload, _ := json.Marshal(router.RouterSettings{Mode: router.ModeBestPractice, PriorityChain: []router.PriorityModelItem{
		{Provider: "chatgpt", Model: "gpt-4o", Enabled: true},
	}})
	w := httptest.NewRecorder()
	r.handleRouterPreview(w, httptest.NewRequest(http.MethodPost, "/api/v1/router/preview", bytes.NewReader(payload)))
	if w.Code != http.StatusOK {
		t.Fatalf("POST preview: %d %s", w.Code, w.Body.String())
	}
	var res struct {
		Preview []router.RoutePreview   `json:"preview"`
		Tiers   []router.TierAssignment `json:"tiers"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if len(res.Preview) == 0 || res.Preview[0].Decision.Model != "chatgpt/gpt-4o" || res.Preview[0].Decision.Suggestion == nil {
		t.Fatalf("preview = %+v", res.Preview)
	}
	if after := r.strategyRouter.GetSettings(); after.Mode != before.Mode || len(after.PriorityChain) != len(before.PriorityChain) {
		t.Fatalf("preview applied settings: %+v", after)
	}
}

func TestProviderUsableUsesCachedStatusOnly(t *testing.T) {
	c := newConnectionChecker(nil)
	if !c.Usable("openai") {
		t.Fatal("unchecked provider must count as usable")
	}
	c.cache["chatgpt"] = ProviderConnection{Status: ConnInvalid}
	c.cache["claude"] = ProviderConnection{Status: ConnUnreachable}
	if c.Usable("openai") || c.Usable("chatgpt") {
		t.Fatal("invalid credential must make the provider unusable (alias folded)")
	}
	if !c.Usable("claude") {
		t.Fatal("unreachable is transient; failover should still try it")
	}
}
