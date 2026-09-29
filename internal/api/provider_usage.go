package api

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/quota"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/telemetry"
)

// UsageTotals sums token usage and cost over a set of LLM calls.
type UsageTotals struct {
	Calls            int     `json:"calls"`
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	CachedTokens     int64   `json:"cached_tokens"`
	TotalTokens      int64   `json:"total_tokens"`
	CostUSD          float64 `json:"cost_usd"`
}

func (t *UsageTotals) add(ev telemetry.TokenEvent) {
	t.Calls++
	t.PromptTokens += ev.PromptTokens
	t.CompletionTokens += ev.CompletionTokens
	t.CachedTokens += ev.CachedTokens
	t.TotalTokens += ev.PromptTokens + ev.CompletionTokens
	t.CostUSD += ev.CostUSD
}

// ModelUsage is one model's share of a provider's usage.
type ModelUsage struct {
	Model string `json:"model"`
	UsageTotals
}

// QuotaStatus reports month-to-date usage against the operator-set quota. A percent is only set
// when that limit is set; 0 limits mean "no limit".
type QuotaStatus struct {
	ProviderQuota
	TokenPercent *float64 `json:"token_percent,omitempty"`
	CostPercent  *float64 `json:"cost_percent,omitempty"`
	Exceeded     bool     `json:"exceeded"`
}

// ProviderUsage is the usage panel for one provider card.
type ProviderUsage struct {
	ProviderID  string       `json:"provider_id"`
	Window      UsageTotals  `json:"window"`
	Last24h     UsageTotals  `json:"last_24h"`
	MonthToDate UsageTotals  `json:"month_to_date"`
	Models      []ModelUsage `json:"models"`
	LastUsedAt  *time.Time   `json:"last_used_at,omitempty"`
	Quota       QuotaStatus  `json:"quota"`
	// Limits is the provider-reported plan usage (e.g. Claude's 5h/weekly %), when available.
	Limits *quota.ProviderLimits `json:"limits,omitempty"`
	Source string                `json:"source"`
}

// usageProviderID maps a telemetry event onto a provider card id. The routed model string carries
// the provider prefix ("claude/…"); the driver-reported provider is the fallback.
func usageProviderID(ev telemetry.TokenEvent) string {
	if i := strings.IndexByte(ev.Model, '/'); i > 0 {
		return normalizeProviderID(ev.Model[:i])
	}
	if ev.Provider != "" {
		return normalizeProviderID(ev.Provider)
	}
	return "claude" // unprefixed models resolve to the Claude driver (see ClientFactory.GetClient)
}

func normalizeProviderID(p string) string {
	switch strings.ToLower(p) {
	case "anthropic", "claude":
		return "claude"
	case "openai", "chatgpt":
		return "chatgpt"
	case "google", "gemini", "antigravity":
		return "antigravity"
	case "ollama", "vllm", "opencode":
		return "opencode"
	}
	return strings.ToLower(p)
}

// usageModelName prefers the model that actually served the call over the routed one.
func usageModelName(ev telemetry.TokenEvent) string {
	if ev.ServedModel != "" {
		return ev.ServedModel
	}
	if i := strings.IndexByte(ev.Model, '/'); i >= 0 {
		return ev.Model[i+1:]
	}
	if ev.Model == "" {
		return "unknown"
	}
	return ev.Model
}

func parseUsageWindow(s string) (time.Duration, string) {
	switch s {
	case "24h":
		return 24 * time.Hour, "24h"
	case "30d":
		return 30 * 24 * time.Hour, "30d"
	default:
		return 7 * 24 * time.Hour, "7d"
	}
}

// buildProviderUsage aggregates telemetry token events per provider.
func buildProviderUsage(events []telemetry.TokenEvent, providerIDs []string, quotas map[string]ProviderQuota, now time.Time, window time.Duration) []ProviderUsage {
	windowStart := now.Add(-window)
	dayStart := now.Add(-24 * time.Hour)
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	byID := map[string]*ProviderUsage{}
	models := map[string]map[string]*ModelUsage{}
	order := []string{}
	get := func(id string) *ProviderUsage {
		if u, ok := byID[id]; ok {
			return u
		}
		u := &ProviderUsage{ProviderID: id, Models: []ModelUsage{}, Source: "orchestrator telemetry"}
		byID[id] = u
		models[id] = map[string]*ModelUsage{}
		order = append(order, id)
		return u
	}
	for _, id := range providerIDs {
		get(id)
	}

	for _, ev := range events {
		id := usageProviderID(ev)
		u := get(id)
		if !ev.Timestamp.Before(monthStart) {
			u.MonthToDate.add(ev)
		}
		if !ev.Timestamp.Before(dayStart) {
			u.Last24h.add(ev)
		}
		if !ev.Timestamp.Before(windowStart) {
			u.Window.add(ev)
			name := usageModelName(ev)
			m, ok := models[id][name]
			if !ok {
				m = &ModelUsage{Model: name}
				models[id][name] = m
			}
			m.add(ev)
		}
		if u.LastUsedAt == nil || ev.Timestamp.After(*u.LastUsedAt) {
			ts := ev.Timestamp
			u.LastUsedAt = &ts
		}
	}

	out := make([]ProviderUsage, 0, len(order))
	for _, id := range order {
		u := byID[id]
		for _, m := range models[id] {
			u.Models = append(u.Models, *m)
		}
		sort.Slice(u.Models, func(i, j int) bool { return u.Models[i].TotalTokens > u.Models[j].TotalTokens })

		q := QuotaStatus{ProviderQuota: quotas[id]}
		if q.MonthlyTokenLimit > 0 {
			p := float64(u.MonthToDate.TotalTokens) / float64(q.MonthlyTokenLimit) * 100
			q.TokenPercent = &p
			q.Exceeded = q.Exceeded || u.MonthToDate.TotalTokens >= q.MonthlyTokenLimit
		}
		if q.MonthlyCostLimitUSD > 0 {
			p := u.MonthToDate.CostUSD / q.MonthlyCostLimitUSD * 100
			q.CostPercent = &p
			q.Exceeded = q.Exceeded || u.MonthToDate.CostUSD >= q.MonthlyCostLimitUSD
		}
		u.Quota = q
		out = append(out, *u)
	}
	return out
}

// handleProviderUsage serves GET /api/v1/providers/usage?window=24h|7d|30d.
func (r *Router) handleProviderUsage(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		r.writeError(w, http.StatusMethodNotAllowed, "GET required")
		return
	}
	window, label := parseUsageWindow(req.URL.Query().Get("window"))
	now := time.Now()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	since := now.Add(-window)
	if monthStart.Before(since) {
		since = monthStart // month-to-date quota needs the whole month
	}

	var events []telemetry.TokenEvent
	if r.telemetry != nil && r.telemetry.store != nil {
		events = r.telemetry.store.Tokens(telemetry.Filter{Since: since})
	}

	providerAuthStore.mu.RLock()
	ids := make([]string, 0, len(defaultProviders))
	for _, p := range defaultProviders {
		ids = append(ids, p.ID)
	}
	quotas := make(map[string]ProviderQuota, len(providerQuotas))
	for id, q := range providerQuotas {
		quotas[id] = q
	}
	providerAuthStore.mu.RUnlock()

	usage := buildProviderUsage(events, ids, quotas, now, window)
	if r.quota != nil {
		force := req.URL.Query().Get("refresh") == "1"
		limits := r.quota.Limits(req.Context(), ids, force)
		for i := range usage {
			if l, ok := limits[usage[i].ProviderID]; ok {
				usage[i].Limits = &l
			}
		}
	}

	r.writeJSON(w, http.StatusOK, map[string]interface{}{
		"window":       label,
		"generated_at": now,
		"providers":    usage,
	})
}

// handleProviderQuota serves POST /api/v1/providers/quota — set or clear a provider's monthly quota.
func (r *Router) handleProviderQuota(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		r.writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var body struct {
		ProviderID string `json:"provider_id"`
		ProviderQuota
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		r.writeError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}
	if body.ProviderID == "" {
		r.writeError(w, http.StatusBadRequest, "provider_id is required")
		return
	}
	if body.MonthlyTokenLimit < 0 || body.MonthlyCostLimitUSD < 0 {
		r.writeError(w, http.StatusBadRequest, "quota limits must be zero (no limit) or positive")
		return
	}

	providerAuthStore.mu.Lock()
	if body.MonthlyTokenLimit == 0 && body.MonthlyCostLimitUSD == 0 {
		delete(providerQuotas, body.ProviderID)
	} else {
		providerQuotas[body.ProviderID] = body.ProviderQuota
	}
	err := saveProviderStateLocked()
	providerAuthStore.mu.Unlock()

	resp := map[string]interface{}{"status": "saved", "provider_id": body.ProviderID, "quota": body.ProviderQuota}
	if err != nil {
		resp["persisted"] = false
		resp["warning"] = "saved for this session only: " + err.Error()
	}
	r.writeJSON(w, http.StatusOK, resp)
}
