package quota

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// ClaudeCredentials is the part of the Claude Code login this package needs.
type ClaudeCredentials struct {
	AccessToken      string
	ExpiresAt        time.Time
	SubscriptionType string
}

// ClaudeFetcher reads the Claude plan usage shown by `claude /usage`.
type ClaudeFetcher struct {
	BaseURL     string
	Client      *http.Client
	Credentials func() (ClaudeCredentials, error)
	Now         func() time.Time
}

// NewClaudeFetcher uses the Claude Code login from the macOS keychain or ~/.claude/.credentials.json.
func NewClaudeFetcher() *ClaudeFetcher {
	return &ClaudeFetcher{
		BaseURL:     "https://api.anthropic.com",
		Client:      &http.Client{Timeout: 10 * time.Second},
		Credentials: loadClaudeCodeCredentials,
		Now:         time.Now,
	}
}

var errNoLogin = errors.New("no local login found")

func loadClaudeCodeCredentials() (ClaudeCredentials, error) {
	var raw []byte
	if runtime.GOOS == "darwin" {
		if out, err := exec.Command("security", "find-generic-password", "-s", "Claude Code-credentials", "-w").Output(); err == nil {
			raw = out
		}
	}
	if raw == nil {
		home, _ := os.UserHomeDir()
		b, err := os.ReadFile(filepath.Join(home, ".claude", ".credentials.json"))
		if err != nil {
			return ClaudeCredentials{}, errNoLogin
		}
		raw = b
	}
	var f struct {
		OAuth struct {
			AccessToken      string `json:"accessToken"`
			ExpiresAt        int64  `json:"expiresAt"` // unix ms
			SubscriptionType string `json:"subscriptionType"`
		} `json:"claudeAiOauth"`
	}
	if err := json.Unmarshal(raw, &f); err != nil || f.OAuth.AccessToken == "" {
		return ClaudeCredentials{}, errNoLogin
	}
	c := ClaudeCredentials{AccessToken: f.OAuth.AccessToken, SubscriptionType: f.OAuth.SubscriptionType}
	if f.OAuth.ExpiresAt > 0 {
		c.ExpiresAt = time.UnixMilli(f.OAuth.ExpiresAt)
	}
	return c, nil
}

type claudeUsageBucket struct {
	Utilization *float64 `json:"utilization"`
	ResetsAt    string   `json:"resets_at"`
}

// claudeBuckets are the known buckets, in display order. Null buckets do not apply to the plan.
var claudeBuckets = []struct{ key, id, label string }{
	{"five_hour", "session", "Current session (5h)"},
	{"seven_day", "weekly", "Weekly · all models"},
	{"seven_day_opus", "weekly_opus", "Weekly · Opus"},
	{"seven_day_sonnet", "weekly_sonnet", "Weekly · Sonnet"},
}

// Fetch implements Fetcher.
func (f *ClaudeFetcher) Fetch(ctx context.Context) ProviderLimits {
	l := ProviderLimits{Source: "Claude Code login · api.anthropic.com/api/oauth/usage", FetchedAt: f.Now()}
	creds, err := f.Credentials()
	if err != nil {
		l.Status = StatusUnavailable
		l.Message = "No Claude Code login found. Run `claude` and sign in with your Claude subscription."
		return l
	}
	l.Plan = creds.SubscriptionType
	if !creds.ExpiresAt.IsZero() && f.Now().After(creds.ExpiresAt) {
		l.Status = StatusStale
		l.Message = "Claude Code login has expired. Run `claude` once to refresh it."
		return l
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(f.BaseURL, "/")+"/api/oauth/usage", nil)
	if err != nil {
		l.Status, l.Message = StatusError, err.Error()
		return l
	}
	req.Header.Set("Authorization", "Bearer "+creds.AccessToken)
	req.Header.Set("anthropic-beta", "oauth-2025-04-20")
	req.Header.Set("User-Agent", "meta-orchestrator")
	resp, err := f.Client.Do(req)
	if err != nil {
		l.Status, l.Message = StatusError, "usage request failed: "+err.Error()
		return l
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		l.Status = StatusStale
		l.Message = "Claude rejected the local login. Run `claude` once to refresh it."
		return l
	case resp.StatusCode != http.StatusOK:
		l.Status, l.Message = StatusError, fmt.Sprintf("usage endpoint returned HTTP %d", resp.StatusCode)
		return l
	}

	var data map[string]json.RawMessage
	if err := json.Unmarshal(body, &data); err != nil {
		l.Status, l.Message = StatusError, "unexpected usage response"
		return l
	}
	for _, b := range claudeBuckets {
		raw, ok := data[b.key]
		if !ok || string(raw) == "null" {
			continue
		}
		var bucket claudeUsageBucket
		if json.Unmarshal(raw, &bucket) != nil || bucket.Utilization == nil {
			continue
		}
		w := Window{ID: b.id, Label: b.label, UsedPercent: *bucket.Utilization}
		if t, err := time.Parse(time.RFC3339Nano, bucket.ResetsAt); err == nil {
			w.ResetsAt = &t
		}
		l.Windows = append(l.Windows, w)
	}
	l.Status = StatusOK
	return l
}
