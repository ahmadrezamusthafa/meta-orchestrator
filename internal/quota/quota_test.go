package quota

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

var fixedNow = time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)

func TestClaudeFetcher(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/oauth/usage" || r.Header.Get("Authorization") != "Bearer test-access" || r.Header.Get("anthropic-beta") == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"five_hour":{"utilization":16.0,"resets_at":"2026-09-29T14:09:59.894104+00:00"},
			"seven_day":{"utilization":39.0,"resets_at":"2026-10-03T01:59:59.894135+00:00"},
			"seven_day_opus":null,"seven_day_sonnet":null}`))
	}))
	defer srv.Close()

	f := &ClaudeFetcher{BaseURL: srv.URL, Client: srv.Client(), Now: func() time.Time { return fixedNow },
		Credentials: func() (ClaudeCredentials, error) {
			return ClaudeCredentials{AccessToken: "test-access", ExpiresAt: fixedNow.Add(time.Hour), SubscriptionType: "team"}, nil
		}}
	l := f.Fetch(context.Background())
	if l.Status != StatusOK || l.Plan != "team" || len(l.Windows) != 2 {
		t.Fatalf("unexpected limits: %+v", l)
	}
	if l.Windows[0].ID != "session" || l.Windows[0].UsedPercent != 16 || l.Windows[0].ResetsAt == nil {
		t.Fatalf("session window: %+v", l.Windows[0])
	}
	if l.Windows[1].ID != "weekly" || l.Windows[1].UsedPercent != 39 {
		t.Fatalf("weekly window: %+v", l.Windows[1])
	}

	// Rejected token → stale, and the message never carries the token.
	f.Credentials = func() (ClaudeCredentials, error) { return ClaudeCredentials{AccessToken: "wrong"}, nil }
	if l := f.Fetch(context.Background()); l.Status != StatusStale {
		t.Fatalf("want stale for rejected token, got %+v", l)
	}

	f.Credentials = func() (ClaudeCredentials, error) {
		return ClaudeCredentials{AccessToken: "test-access", ExpiresAt: fixedNow.Add(-time.Minute)}, nil
	}
	if l := f.Fetch(context.Background()); l.Status != StatusStale {
		t.Fatalf("want stale for expired token, got %+v", l)
	}

	f.Credentials = func() (ClaudeCredentials, error) { return ClaudeCredentials{}, errNoLogin }
	if l := f.Fetch(context.Background()); l.Status != StatusUnavailable {
		t.Fatalf("want unavailable without login, got %+v", l)
	}
}

func TestAntigravityFetcher(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1internal:fetchAvailableModels" || r.Header.Get("Authorization") != "Bearer ag-access" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"models":{
			"gemini-3.1-pro-high":{"displayName":"Gemini 3.1 Pro (High)","quotaInfo":{"remainingFraction":0.25,"resetTime":"2026-10-01T04:06:25Z"}},
			"gemini-3-flash":{"displayName":"Gemini 3 Flash","quotaInfo":{"remainingFraction":0.25,"resetTime":"2026-10-01T04:06:25Z"}},
			"claude-sonnet-4-6":{"displayName":"Claude Sonnet 4.6 (Thinking)","quotaInfo":{"remainingFraction":0.8,"resetTime":"2026-10-04T22:01:31Z"}},
			"gpt-oss-120b-medium":{"displayName":"GPT-OSS 120B (Medium)","quotaInfo":{"remainingFraction":0.8,"resetTime":"2026-10-04T22:01:31Z"}},
			"tab_flash_lite_preview":{"quotaInfo":{"remainingFraction":1}}
		}}`))
	}))
	defer srv.Close()

	path := filepath.Join(t.TempDir(), "token")
	write := func(expiry time.Time) {
		data := `{"token":{"access_token":"ag-access","expiry":"` + expiry.Format(time.RFC3339Nano) + `"}}`
		if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(fixedNow.Add(time.Hour))

	f := &AntigravityFetcher{BaseURL: srv.URL, Client: srv.Client(), TokenPath: path, Now: func() time.Time { return fixedNow }}
	l := f.Fetch(context.Background())
	if l.Status != StatusOK || len(l.Windows) != 2 {
		t.Fatalf("unexpected limits: %+v", l)
	}
	if l.Windows[0].Label != "Gemini models" || l.Windows[0].UsedPercent != 75 || len(l.Windows[0].Models) != 2 {
		t.Fatalf("gemini pool: %+v", l.Windows[0])
	}
	if l.Windows[1].Label != "Claude · GPT-OSS models" || l.Windows[1].UsedPercent < 19.99 || l.Windows[1].UsedPercent > 20.01 {
		t.Fatalf("claude/gpt pool: %+v", l.Windows[1])
	}

	write(fixedNow.Add(-time.Minute))
	if l := f.Fetch(context.Background()); l.Status != StatusStale {
		t.Fatalf("want stale for expired login, got %+v", l)
	}

	f.TokenPath = filepath.Join(t.TempDir(), "missing")
	if l := f.Fetch(context.Background()); l.Status != StatusUnavailable {
		t.Fatalf("want unavailable without login, got %+v", l)
	}
}

type countingFetcher struct{ n int }

func (c *countingFetcher) Fetch(context.Context) ProviderLimits {
	c.n++
	return ProviderLimits{Status: StatusOK}
}

func TestServiceCachesAndReportsUnsupported(t *testing.T) {
	cf := &countingFetcher{}
	s := NewService(map[string]Fetcher{"claude": cf}, time.Minute)
	now := fixedNow
	s.now = func() time.Time { return now }

	out := s.Limits(context.Background(), []string{"claude", "opencode"}, false)
	if out["claude"].Status != StatusOK || out["claude"].ProviderID != "claude" {
		t.Fatalf("claude: %+v", out["claude"])
	}
	if out["opencode"].Status != StatusUnavailable {
		t.Fatalf("unsupported provider should be unavailable: %+v", out["opencode"])
	}
	s.Limits(context.Background(), []string{"claude"}, false)
	if cf.n != 1 {
		t.Fatalf("second call within TTL should hit cache, fetched %d times", cf.n)
	}
	s.Limits(context.Background(), []string{"claude"}, true)
	now = now.Add(2 * time.Minute)
	s.Limits(context.Background(), []string{"claude"}, false)
	if cf.n != 3 {
		t.Fatalf("force and expiry should refetch, fetched %d times", cf.n)
	}
}
