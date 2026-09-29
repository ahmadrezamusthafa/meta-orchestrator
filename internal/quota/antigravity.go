package quota

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// AntigravityFetcher reads Antigravity's per-model quota pools.
type AntigravityFetcher struct {
	BaseURL   string
	Client    *http.Client
	TokenPath string
	Now       func() time.Time
}

// NewAntigravityFetcher uses the Antigravity login at ~/.gemini/jetski-standalone-oauth-token.
func NewAntigravityFetcher() *AntigravityFetcher {
	home, _ := os.UserHomeDir()
	return &AntigravityFetcher{
		BaseURL:   "https://cloudcode-pa.googleapis.com",
		Client:    &http.Client{Timeout: 10 * time.Second},
		TokenPath: filepath.Join(home, ".gemini", "jetski-standalone-oauth-token"),
		Now:       time.Now,
	}
}

type antigravityModel struct {
	DisplayName string `json:"displayName"`
	QuotaInfo   *struct {
		RemainingFraction *float64 `json:"remainingFraction"`
		ResetTime         string   `json:"resetTime"`
	} `json:"quotaInfo"`
}

// Fetch implements Fetcher.
func (f *AntigravityFetcher) Fetch(ctx context.Context) ProviderLimits {
	l := ProviderLimits{Source: "Antigravity login · cloudcode-pa.googleapis.com", FetchedAt: f.Now()}
	raw, err := os.ReadFile(f.TokenPath)
	if err != nil {
		l.Status = StatusUnavailable
		l.Message = "No Antigravity login found. Open Antigravity and sign in."
		return l
	}
	var tok struct {
		Token struct {
			AccessToken string `json:"access_token"`
			Expiry      string `json:"expiry"`
		} `json:"token"`
	}
	if json.Unmarshal(raw, &tok) != nil || tok.Token.AccessToken == "" {
		l.Status, l.Message = StatusUnavailable, "Antigravity login file is not readable."
		return l
	}
	if exp, err := time.Parse(time.RFC3339Nano, tok.Token.Expiry); err == nil && f.Now().After(exp) {
		l.Status = StatusStale
		l.Message = "Antigravity login has expired. Open Antigravity to refresh it."
		return l
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(f.BaseURL, "/")+"/v1internal:fetchAvailableModels", strings.NewReader("{}"))
	if err != nil {
		l.Status, l.Message = StatusError, err.Error()
		return l
	}
	req.Header.Set("Authorization", "Bearer "+tok.Token.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "antigravity")
	resp, err := f.Client.Do(req)
	if err != nil {
		l.Status, l.Message = StatusError, "quota request failed: "+err.Error()
		return l
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		l.Status = StatusStale
		l.Message = "Antigravity rejected the local login. Open Antigravity to refresh it."
		return l
	case resp.StatusCode != http.StatusOK:
		l.Status, l.Message = StatusError, fmt.Sprintf("quota endpoint returned HTTP %d", resp.StatusCode)
		return l
	}

	var data struct {
		Models map[string]antigravityModel `json:"models"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		l.Status, l.Message = StatusError, "unexpected quota response"
		return l
	}
	l.Windows = antigravityPools(data.Models)
	l.Status = StatusOK
	return l
}

// antigravityPools groups models sharing one quota (same remaining fraction and reset time) into
// a window. Internal models (no display name) are skipped.
func antigravityPools(models map[string]antigravityModel) []Window {
	type pool struct {
		remaining float64
		reset     string
		names     map[string]bool
	}
	pools := map[string]*pool{}
	for _, m := range models {
		if m.DisplayName == "" || m.QuotaInfo == nil || m.QuotaInfo.RemainingFraction == nil {
			continue
		}
		key := fmt.Sprintf("%.6f|%s", *m.QuotaInfo.RemainingFraction, m.QuotaInfo.ResetTime)
		p, ok := pools[key]
		if !ok {
			p = &pool{remaining: *m.QuotaInfo.RemainingFraction, reset: m.QuotaInfo.ResetTime, names: map[string]bool{}}
			pools[key] = p
		}
		p.names[m.DisplayName] = true
	}

	out := make([]Window, 0, len(pools))
	for _, p := range pools {
		names := make([]string, 0, len(p.names))
		for n := range p.names {
			names = append(names, n)
		}
		sort.Strings(names)
		w := Window{Label: poolLabel(names), UsedPercent: clampPercent((1 - p.remaining) * 100), Models: names}
		w.ID = strings.ToLower(strings.ReplaceAll(w.Label, " ", "_"))
		if t, err := time.Parse(time.RFC3339Nano, p.reset); err == nil {
			w.ResetsAt = &t
		}
		out = append(out, w)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].UsedPercent != out[j].UsedPercent {
			return out[i].UsedPercent > out[j].UsedPercent
		}
		return out[i].Label < out[j].Label
	})
	return out
}

// poolLabel names a pool by the model families in it, e.g. "Gemini models", "Claude · GPT-OSS models".
func poolLabel(names []string) string {
	seen := map[string]bool{}
	var families []string
	for _, n := range names {
		fam := n
		if i := strings.IndexByte(n, ' '); i > 0 {
			fam = n[:i]
		}
		if !seen[fam] {
			seen[fam] = true
			families = append(families, fam)
		}
	}
	return strings.Join(families, " · ") + " models"
}

func clampPercent(p float64) float64 {
	if p < 0 {
		return 0
	}
	if p > 100 {
		return 100
	}
	return p
}
