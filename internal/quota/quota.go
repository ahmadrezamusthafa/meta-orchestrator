// Package quota reads provider-side plan limits (the percentages the Claude Code CLI shows in
// /usage, Antigravity's per-model quota pools) using the operator's local CLI/IDE logins.
//
// Credentials are read on every fetch, kept in memory only for the request, and never returned
// or logged. Tokens are not refreshed here: the owning CLI/IDE refreshes them, and an expired
// token is reported as "stale" with a hint to reopen that tool.
package quota

import (
	"context"
	"sync"
	"time"
)

// Status values for ProviderLimits.
const (
	StatusOK          = "ok"
	StatusUnavailable = "unavailable" // no local login / not supported for this provider
	StatusStale       = "stale"       // login found but expired or rejected
	StatusError       = "error"       // provider call failed
)

// Window is one limit bucket, e.g. Claude's 5-hour session or an Antigravity model pool.
type Window struct {
	ID          string     `json:"id"`
	Label       string     `json:"label"`
	UsedPercent float64    `json:"used_percent"`
	ResetsAt    *time.Time `json:"resets_at,omitempty"`
	Models      []string   `json:"models,omitempty"`
}

// ProviderLimits is the provider-reported plan usage for one provider card.
type ProviderLimits struct {
	ProviderID string    `json:"provider_id"`
	Status     string    `json:"status"`
	Source     string    `json:"source"`
	Plan       string    `json:"plan,omitempty"`
	Message    string    `json:"message,omitempty"`
	Windows    []Window  `json:"windows"`
	FetchedAt  time.Time `json:"fetched_at"`
}

// Fetcher reads plan limits for one provider.
type Fetcher interface {
	Fetch(ctx context.Context) ProviderLimits
}

// Service caches fetcher results so page polling does not hit provider APIs on every request.
type Service struct {
	fetchers map[string]Fetcher
	ttl      time.Duration
	now      func() time.Time

	mu    sync.Mutex
	cache map[string]ProviderLimits
}

// NewService builds a service over fetchers keyed by provider card id.
func NewService(fetchers map[string]Fetcher, ttl time.Duration) *Service {
	return &Service{fetchers: fetchers, ttl: ttl, now: time.Now, cache: map[string]ProviderLimits{}}
}

// DefaultFetchers reads the local Claude Code and Antigravity logins.
func DefaultFetchers() map[string]Fetcher {
	return map[string]Fetcher{
		"claude":      NewClaudeFetcher(),
		"antigravity": NewAntigravityFetcher(),
	}
}

// Limits returns limits for the given provider ids. Providers without a fetcher report
// "unavailable". force bypasses the cache.
func (s *Service) Limits(ctx context.Context, providerIDs []string, force bool) map[string]ProviderLimits {
	out := make(map[string]ProviderLimits, len(providerIDs))
	var wg sync.WaitGroup
	var mu sync.Mutex
	for _, id := range providerIDs {
		f, ok := s.fetchers[id]
		if !ok {
			out[id] = ProviderLimits{ProviderID: id, Status: StatusUnavailable, Windows: []Window{},
				Message: "This provider does not expose plan usage.", FetchedAt: s.now()}
			continue
		}
		if !force {
			s.mu.Lock()
			cached, hit := s.cache[id]
			s.mu.Unlock()
			if hit && s.now().Sub(cached.FetchedAt) < s.ttl {
				out[id] = cached
				continue
			}
		}
		wg.Add(1)
		go func(id string, f Fetcher) {
			defer wg.Done()
			l := f.Fetch(ctx)
			l.ProviderID = id
			if l.Windows == nil {
				l.Windows = []Window{}
			}
			if l.FetchedAt.IsZero() {
				l.FetchedAt = s.now()
			}
			s.mu.Lock()
			s.cache[id] = l
			s.mu.Unlock()
			mu.Lock()
			out[id] = l
			mu.Unlock()
		}(id, f)
	}
	wg.Wait()
	return out
}
