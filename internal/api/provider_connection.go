package api

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/quota"
)

// Connection status values: what the orchestrator will actually get when it calls the provider.
const (
	ConnConnected     = "connected"      // verified live against the provider
	ConnNotConfigured = "not_configured" // nothing the driver can use
	ConnInvalid       = "invalid"        // the provider rejected the credential
	ConnExpired       = "expired"        // a CLI/IDE login exists but has expired
	ConnUnreachable   = "unreachable"    // could not reach the provider to verify
)

// Connection methods: the credential the driver really uses for requests.
const (
	MethodAPIKey    = "api_key"
	MethodClaudeCLI = "claude_cli"
	MethodLocal     = "local_endpoint"
	MethodNone      = "none"
)

// ProviderConnection answers "is this provider usable, and through what?" for one card.
type ProviderConnection struct {
	ProviderID string    `json:"provider_id"`
	Status     string    `json:"status"`
	Method     string    `json:"method"`
	Label      string    `json:"label"`            // e.g. "Claude Code CLI"
	Detail     string    `json:"detail,omitempty"` // e.g. "Signed in · team plan"
	Hint       string    `json:"hint,omitempty"`   // what to do when not connected
	Plan       string    `json:"plan,omitempty"`
	LatencyMs  int64     `json:"latency_ms,omitempty"`
	CheckedAt  time.Time `json:"checked_at"`
	// LegacyAuth flags a saved session token / OAuth login that no driver uses for requests.
	LegacyAuth string `json:"legacy_auth,omitempty"`
}

// connectionChecker verifies provider credentials. Endpoints are fields so tests can point them
// at fakes. Credentials go only to the provider they belong to and never into messages.
type connectionChecker struct {
	client       *http.Client
	anthropicURL string
	openaiURL    string
	geminiURL    string
	lookPath     func(string) (string, error)
	claudeLogin  func(ctx context.Context, force bool) quota.ProviderLimits
	now          func() time.Time
	ttl          time.Duration

	mu    sync.Mutex
	cache map[string]ProviderConnection
}

func newConnectionChecker(q *quota.Service) *connectionChecker {
	c := &connectionChecker{
		client:       &http.Client{Timeout: 10 * time.Second},
		anthropicURL: "https://api.anthropic.com/v1",
		openaiURL:    "https://api.openai.com/v1",
		geminiURL:    "https://generativelanguage.googleapis.com/v1beta",
		lookPath:     exec.LookPath,
		now:          time.Now,
		ttl:          time.Minute,
		cache:        map[string]ProviderConnection{},
	}
	c.claudeLogin = func(ctx context.Context, force bool) quota.ProviderLimits {
		if q == nil {
			return quota.ProviderLimits{Status: quota.StatusUnavailable}
		}
		return q.Limits(ctx, []string{"claude"}, force)["claude"]
	}
	return c
}

func (c *connectionChecker) invalidate(providerID string) {
	c.mu.Lock()
	delete(c.cache, providerID)
	c.mu.Unlock()
}

// Usable reports, from cached checks only (never the network), whether the router may send work
// to a provider. Unchecked or unreachable providers count as usable so failover still tries them.
func (c *connectionChecker) Usable(provider string) bool {
	c.mu.Lock()
	conn, ok := c.cache[normalizeProviderID(provider)]
	c.mu.Unlock()
	if !ok {
		return true
	}
	switch conn.Status {
	case ConnNotConfigured, ConnInvalid, ConnExpired:
		return false
	}
	return true
}

// providerCreds is a snapshot of what the driver would use for one provider.
type providerCreds struct {
	apiKey     string
	authMethod string
	baseURL    string
}

func snapshotCreds(providerID string) providerCreds {
	providerAuthStore.mu.RLock()
	defer providerAuthStore.mu.RUnlock()
	var pc providerCreds
	if s := providerAuthStore.state[providerID]; s != nil {
		pc.apiKey, pc.authMethod = s.APIKey, s.AuthMethod
		if s.SessionToken != "" && pc.authMethod == "api_key" {
			pc.authMethod = "session_token" // token still stored, still unused
		}
	}
	for _, p := range defaultProviders {
		if p.ID == providerID {
			pc.baseURL = p.BaseURL
		}
	}
	return pc
}

// Check returns the provider's connection, cached for ttl unless force is set.
func (c *connectionChecker) Check(ctx context.Context, providerID string, force bool) ProviderConnection {
	if !force {
		c.mu.Lock()
		cached, ok := c.cache[providerID]
		c.mu.Unlock()
		if ok && c.now().Sub(cached.CheckedAt) < c.ttl {
			return cached
		}
	}
	conn := c.check(ctx, providerID, snapshotCreds(providerID), force)
	c.mu.Lock()
	c.cache[providerID] = conn
	c.mu.Unlock()
	return conn
}

// CheckAll checks every provider concurrently.
func (c *connectionChecker) CheckAll(ctx context.Context, ids []string, force bool) map[string]ProviderConnection {
	out := make(map[string]ProviderConnection, len(ids))
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			conn := c.Check(ctx, id, force)
			mu.Lock()
			out[id] = conn
			mu.Unlock()
		}(id)
	}
	wg.Wait()
	return out
}

func (c *connectionChecker) check(ctx context.Context, providerID string, pc providerCreds, force bool) ProviderConnection {
	conn := ProviderConnection{ProviderID: providerID, CheckedAt: c.now(), Method: MethodNone}
	switch pc.authMethod {
	case "session_token":
		conn.LegacyAuth = "session_token"
	case "oauth":
		conn.LegacyAuth = "oauth"
	}

	switch providerID {
	case "claude":
		// Mirrors AnthropicDriver.StreamActivity: an sk-ant- key uses the API, anything else the CLI.
		if pc.apiKey != "" && !strings.HasPrefix(pc.apiKey, "sk-ant-") {
			c.checkClaudeCLI(ctx, &conn, force)
			conn.Detail += " · saved key " + maskToken(pc.apiKey) + " is ignored (not an sk-ant- key)"
			return conn
		}
		if pc.apiKey != "" {
			c.verifyKey(ctx, &conn, "Anthropic API key", pc.apiKey, "sk-ant-", func(req *http.Request) {
				req.Header.Set("x-api-key", pc.apiKey)
				req.Header.Set("anthropic-version", "2023-06-01")
			}, c.anthropicURL+"/models?limit=1")
			return conn
		}
		c.checkClaudeCLI(ctx, &conn, force)
	case "chatgpt":
		if pc.apiKey == "" {
			notConfigured(&conn, "OpenAI API key", "Add an OpenAI API key from platform.openai.com/api-keys.")
			return conn
		}
		c.verifyKey(ctx, &conn, "OpenAI API key", pc.apiKey, "", func(req *http.Request) {
			req.Header.Set("Authorization", "Bearer "+pc.apiKey)
		}, c.openaiURL+"/models")
	case "antigravity":
		if pc.apiKey == "" {
			notConfigured(&conn, "Gemini API key", "Add a Gemini API key from aistudio.google.com/apikey.")
			return conn
		}
		c.verifyKey(ctx, &conn, "Gemini API key", pc.apiKey, "", func(req *http.Request) {
			req.Header.Set("x-goog-api-key", pc.apiKey)
		}, c.geminiURL+"/models?pageSize=1")
	case "opencode":
		c.checkLocal(ctx, &conn, pc.baseURL)
	default:
		notConfigured(&conn, "Unknown provider", "")
	}
	return conn
}

func notConfigured(conn *ProviderConnection, label, hint string) {
	conn.Status, conn.Method, conn.Label, conn.Hint = ConnNotConfigured, MethodNone, label, hint
}

// verifyKey calls a cheap authenticated read endpoint (model list) to prove the key works.
func (c *connectionChecker) verifyKey(ctx context.Context, conn *ProviderConnection, label, key, prefix string, auth func(*http.Request), url string) {
	conn.Method, conn.Label = MethodAPIKey, label
	masked := maskToken(key)
	if prefix != "" && !strings.HasPrefix(key, prefix) {
		conn.Status = ConnInvalid
		conn.Detail = fmt.Sprintf("Key %s is not a %s… key, so it is ignored.", masked, prefix)
		conn.Hint = "Replace it with a key that starts with " + prefix
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		conn.Status, conn.Detail = ConnUnreachable, err.Error()
		return
	}
	auth(req)
	start := c.now()
	resp, err := c.client.Do(req)
	if err != nil {
		conn.Status = ConnUnreachable
		conn.Detail = "Could not reach the provider to verify key " + masked + "."
		conn.Hint = "Check your network, then re-check."
		return
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	conn.LatencyMs = time.Since(start).Milliseconds()
	switch {
	case resp.StatusCode == http.StatusOK:
		conn.Status, conn.Detail = ConnConnected, "Key "+masked+" verified"
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden ||
		(resp.StatusCode == http.StatusBadRequest && strings.Contains(url, "googleapis")):
		conn.Status = ConnInvalid
		conn.Detail = fmt.Sprintf("Key %s was rejected (HTTP %d).", masked, resp.StatusCode)
		conn.Hint = "Paste a new key, or remove it."
	default:
		conn.Status = ConnUnreachable
		conn.Detail = fmt.Sprintf("Provider returned HTTP %d while verifying key %s.", resp.StatusCode, masked)
		conn.Hint = "Try re-checking in a moment."
	}
}

// checkClaudeCLI reports the Claude Code CLI login, which the driver uses when no API key is set.
func (c *connectionChecker) checkClaudeCLI(ctx context.Context, conn *ProviderConnection, force bool) {
	conn.Method, conn.Label = MethodClaudeCLI, "Claude Code CLI"
	if _, err := c.lookPath("claude"); err != nil {
		conn.Status, conn.Method = ConnNotConfigured, MethodNone
		conn.Label = "Claude subscription or API key"
		conn.Hint = "Install Claude Code (npm i -g @anthropic-ai/claude-code) and run `claude` to sign in — or add an Anthropic API key."
		return
	}
	start := c.now()
	login := c.claudeLogin(ctx, force)
	if force {
		conn.LatencyMs = time.Since(start).Milliseconds()
	}
	conn.Plan = login.Plan
	plan := ""
	if login.Plan != "" {
		plan = " · " + login.Plan + " plan"
	}
	switch login.Status {
	case quota.StatusOK:
		conn.Status, conn.Detail = ConnConnected, "Signed in"+plan
	case quota.StatusStale:
		conn.Status, conn.Detail = ConnExpired, "Login expired"+plan
		conn.Hint = "Run `claude` in a terminal once; it refreshes the login automatically."
	case quota.StatusUnavailable:
		conn.Status, conn.Detail = ConnNotConfigured, "CLI installed, but not signed in"
		conn.Hint = "Run `claude` in a terminal and sign in with your Claude account (/login)."
	default:
		conn.Status, conn.Detail = ConnUnreachable, "CLI installed; could not verify the login"+plan
		conn.Hint = "Check your network, then re-check."
	}
}

// checkLocal pings a local Ollama / OpenAI-compatible server.
func (c *connectionChecker) checkLocal(ctx context.Context, conn *ProviderConnection, baseURL string) {
	conn.Method, conn.Label = MethodLocal, "Local endpoint"
	if baseURL == "" {
		notConfigured(conn, "Local endpoint", "Set a base URL for your local model server.")
		return
	}
	base := strings.TrimRight(baseURL, "/")
	for _, path := range []string{"/api/tags", "/v1/models"} {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+path, nil)
		if err != nil {
			break
		}
		start := c.now()
		resp, err := c.client.Do(req)
		if err != nil {
			continue
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			conn.Status, conn.Detail = ConnConnected, "Reachable at "+base
			conn.LatencyMs = time.Since(start).Milliseconds()
			return
		}
	}
	conn.Status = ConnUnreachable
	conn.Detail = "Nothing answering at " + base
	conn.Hint = "Start Ollama (`ollama serve`) or your vLLM server, then re-check."
}

// handleProviderConnections serves GET /api/v1/providers/connection[?refresh=1].
func (r *Router) handleProviderConnections(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		r.writeError(w, http.StatusMethodNotAllowed, "GET required")
		return
	}
	providerAuthStore.mu.RLock()
	ids := make([]string, 0, len(defaultProviders))
	for _, p := range defaultProviders {
		ids = append(ids, p.ID)
	}
	providerAuthStore.mu.RUnlock()
	force := req.URL.Query().Get("refresh") == "1"
	r.writeJSON(w, http.StatusOK, map[string]interface{}{"connections": r.connections.CheckAll(req.Context(), ids, force)})
}
