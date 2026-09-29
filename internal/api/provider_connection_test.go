package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/quota"
)

const goodAnthropicKey = "sk-ant-api03-good-key-000000000000"

// fakeProviders answers model-list calls: only the "good" keys are accepted.
func fakeProviders(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ok := r.Header.Get("x-api-key") == goodAnthropicKey ||
			r.Header.Get("Authorization") == "Bearer sk-openai-good" ||
			r.Header.Get("x-goog-api-key") == "gemini-good" ||
			r.URL.Path == "/local/api/tags"
		if !ok {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func testChecker(srv *httptest.Server, cli bool, login quota.ProviderLimits) *connectionChecker {
	c := newConnectionChecker(nil)
	c.client = srv.Client()
	c.anthropicURL, c.openaiURL, c.geminiURL = srv.URL, srv.URL, srv.URL
	c.lookPath = func(string) (string, error) {
		if cli {
			return "/usr/local/bin/claude", nil
		}
		return "", errors.New("not found")
	}
	c.claudeLogin = func(context.Context, bool) quota.ProviderLimits { return login }
	return c
}

func TestConnectionClaudeUsesCLILoginWithoutKey(t *testing.T) {
	isolateProviderState(t, "")
	srv := fakeProviders(t)
	ok := quota.ProviderLimits{Status: quota.StatusOK, Plan: "team"}

	conn := testChecker(srv, true, ok).Check(context.Background(), "claude", true)
	if conn.Status != ConnConnected || conn.Method != MethodClaudeCLI || !strings.Contains(conn.Detail, "team plan") {
		t.Fatalf("want connected via CLI: %+v", conn)
	}

	conn = testChecker(srv, true, quota.ProviderLimits{Status: quota.StatusStale}).Check(context.Background(), "claude", true)
	if conn.Status != ConnExpired || conn.Hint == "" {
		t.Fatalf("want expired with a hint: %+v", conn)
	}

	conn = testChecker(srv, false, ok).Check(context.Background(), "claude", true)
	if conn.Status != ConnNotConfigured || conn.Hint == "" {
		t.Fatalf("want not configured without CLI: %+v", conn)
	}
}

func TestConnectionFlagsUnusedSessionToken(t *testing.T) {
	isolateProviderState(t, "")
	providerAuthStore.state["claude"] = &ProviderAuthState{AuthMethod: "session_token", SessionToken: "legacy-session-token-123"}

	conn := testChecker(fakeProviders(t), true, quota.ProviderLimits{Status: quota.StatusOK}).Check(context.Background(), "claude", true)
	if conn.LegacyAuth != "session_token" || conn.Method != MethodClaudeCLI || conn.Status != ConnConnected {
		t.Fatalf("session token should be flagged as unused while the CLI connects: %+v", conn)
	}
	if strings.Contains(conn.Detail+conn.Hint, "legacy-session-token") {
		t.Fatalf("connection must never echo the stored token: %+v", conn)
	}
}

func TestConnectionAPIKeys(t *testing.T) {
	isolateProviderState(t, "")
	c := testChecker(fakeProviders(t), false, quota.ProviderLimits{})

	check := func(id, key string) ProviderConnection {
		return c.check(context.Background(), id, providerCreds{apiKey: key}, true)
	}
	if conn := check("claude", goodAnthropicKey); conn.Status != ConnConnected || conn.Method != MethodAPIKey {
		t.Fatalf("good anthropic key: %+v", conn)
	}
	if conn := check("claude", "sk-ant-api03-bad-key-000000000000"); conn.Status != ConnInvalid {
		t.Fatalf("rejected anthropic key: %+v", conn)
	}
	if conn := check("chatgpt", "sk-openai-good"); conn.Status != ConnConnected {
		t.Fatalf("good openai key: %+v", conn)
	}
	if conn := check("chatgpt", ""); conn.Status != ConnNotConfigured {
		t.Fatalf("missing openai key: %+v", conn)
	}
	if conn := check("antigravity", "gemini-good"); conn.Status != ConnConnected {
		t.Fatalf("good gemini key: %+v", conn)
	}
}

func TestSaveRejectsInvalidKeyAndClears(t *testing.T) {
	isolateProviderState(t, "")
	srv := fakeProviders(t)
	r := &Router{connections: testChecker(srv, false, quota.ProviderLimits{})}

	post := func(body map[string]interface{}) *httptest.ResponseRecorder {
		payload, _ := json.Marshal(body)
		w := httptest.NewRecorder()
		r.handleProviders(w, httptest.NewRequest(http.MethodPost, "/api/v1/providers", bytes.NewReader(payload)))
		return w
	}

	if w := post(map[string]interface{}{"provider_id": "chatgpt", "api_key": "sk-openai-bad"}); w.Code != http.StatusBadRequest {
		t.Fatalf("rejected key should not save: %d %s", w.Code, w.Body.String())
	}
	if providerAuthStore.state["chatgpt"] != nil {
		t.Fatalf("rejected key was stored: %+v", providerAuthStore.state["chatgpt"])
	}

	w := post(map[string]interface{}{"provider_id": "chatgpt", "api_key": "sk-openai-good"})
	var resp struct {
		Connection ProviderConnection `json:"connection"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if w.Code != http.StatusOK || resp.Connection.Status != ConnConnected {
		t.Fatalf("good key should save and report connected: %d %s", w.Code, w.Body.String())
	}

	post(map[string]interface{}{"provider_id": "chatgpt", "clear_api_key": true})
	if s := providerAuthStore.state["chatgpt"]; s == nil || s.APIKey != "" {
		t.Fatalf("clear_api_key should remove the key: %+v", s)
	}

	post(map[string]interface{}{"provider_id": "opencode", "model": "deepseek-r1"})
	if providerAuthStore.state["opencode"] != nil {
		t.Fatalf("a model-only change must not create an auth record")
	}
}
