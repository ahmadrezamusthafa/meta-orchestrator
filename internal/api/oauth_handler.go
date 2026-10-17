package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// OAuthProviderMeta stores metadata required for each provider's OAuth flow.
type OAuthProviderMeta struct {
	ProviderID   string `json:"provider_id"`
	AuthURL      string `json:"auth_url"`
	TokenURL     string `json:"token_url"`
	Scopes       string `json:"scopes"`
	ClientID     string `json:"client_id,omitempty"`
	ClientSecret string `json:"-"` // never exposed to frontend
}

// OAuthSession tracks an in-flight OAuth authorization flow.
type OAuthSession struct {
	State      string    `json:"state"`
	ProviderID string    `json:"provider_id"`
	CreatedAt  time.Time `json:"created_at"`
}

// OAuthTokenInfo represents the stored token for a provider.
type OAuthTokenInfo struct {
	ProviderID   string    `json:"provider_id"`
	AccessToken  string    `json:"-"` // never exposed
	RefreshToken string    `json:"-"` // never exposed
	ExpiresAt    time.Time `json:"expires_at"`
	TokenType    string    `json:"token_type"`
	Scopes       string    `json:"scopes"`
	Email        string    `json:"email,omitempty"`
	AuthMethod   string    `json:"auth_method"` // "oauth" or "api_key"
	IsConnected  bool      `json:"is_connected"`
	ConnectedAt  time.Time `json:"connected_at,omitempty"`
}

// OAuthStore holds in-memory OAuth sessions and tokens.
type OAuthStore struct {
	mu       sync.RWMutex
	sessions map[string]*OAuthSession  // state -> session
	tokens   map[string]*OAuthTokenInfo // provider_id -> token info
}

var oauthStore = &OAuthStore{
	sessions: make(map[string]*OAuthSession),
	tokens:   make(map[string]*OAuthTokenInfo),
}

// Known OAuth provider configurations (endpoints & default scopes).
var oauthProviderConfigs = map[string]OAuthProviderMeta{
	"antigravity": {
		ProviderID: "antigravity",
		AuthURL:    "https://accounts.google.com/o/oauth2/v2/auth",
		TokenURL:   "https://oauth2.googleapis.com/token",
		Scopes:     "openid email https://www.googleapis.com/auth/generative-language",
	},
	"chatgpt": {
		ProviderID: "chatgpt",
		AuthURL:    "https://auth.openai.com/authorize",
		TokenURL:   "https://auth.openai.com/oauth/token",
		Scopes:     "openid profile email model.read model.request",
	},
	"claude": {
		ProviderID: "claude",
		AuthURL:    "https://console.anthropic.com/oauth/authorize",
		TokenURL:   "https://api.anthropic.com/oauth/token",
		Scopes:     "read write",
	},
}

// oauthSessionTTL bounds how long an authorization state stays valid.
const oauthSessionTTL = 10 * time.Minute

func generateState() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("state-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

// handleOAuthInitiate starts the OAuth flow for a given provider.
// POST /api/v1/providers/oauth/initiate
func (r *Router) handleOAuthInitiate(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		r.writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}

	var body struct {
		ProviderID  string `json:"provider_id"`
		RedirectURI string `json:"redirect_uri"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		r.writeError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	providerMeta, ok := oauthProviderConfigs[body.ProviderID]
	if !ok {
		r.writeError(w, http.StatusBadRequest, fmt.Sprintf("Provider '%s' does not support OAuth login", body.ProviderID))
		return
	}

	state := generateState()

	now := time.Now()
	oauthStore.mu.Lock()
	for k, s := range oauthStore.sessions {
		if now.Sub(s.CreatedAt) > oauthSessionTTL {
			delete(oauthStore.sessions, k) // abandoned flows must not pile up
		}
	}
	oauthStore.sessions[state] = &OAuthSession{
		State:      state,
		ProviderID: body.ProviderID,
		CreatedAt:  now,
	}
	oauthStore.mu.Unlock()

	redirectURI := body.RedirectURI
	if redirectURI == "" {
		redirectURI = fmt.Sprintf("%s/api/v1/providers/oauth/callback", req.Header.Get("Origin"))
	}

	// Encoded, so a redirect_uri carrying "&..." cannot add or override query parameters.
	authURL := providerMeta.AuthURL + "?" + url.Values{
		"response_type": {"code"},
		"client_id":     {providerMeta.ClientID},
		"redirect_uri":  {redirectURI},
		"scope":         {providerMeta.Scopes},
		"state":         {state},
		"access_type":   {"offline"},
		"prompt":        {"consent"},
	}.Encode()

	r.writeJSON(w, http.StatusOK, map[string]interface{}{
		"auth_url":    authURL,
		"state":       state,
		"provider_id": body.ProviderID,
		"scopes":      providerMeta.Scopes,
	})
}

// handleOAuthCallback receives the OAuth callback from the provider.
// GET /api/v1/providers/oauth/callback
func (r *Router) handleOAuthCallback(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		r.writeError(w, http.StatusMethodNotAllowed, "GET required")
		return
	}

	state := req.URL.Query().Get("state")
	code := req.URL.Query().Get("code")
	errorParam := req.URL.Query().Get("error")

	if errorParam != "" {
		// Render an HTML page that communicates failure back to parent window
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, oauthCallbackHTML("error", errorParam, "", ""))
		return
	}

	if state == "" || code == "" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, oauthCallbackHTML("error", "Missing state or code parameter", "", ""))
		return
	}

	oauthStore.mu.Lock()
	session, exists := oauthStore.sessions[state]
	if exists {
		delete(oauthStore.sessions, state) // consume once
	}
	oauthStore.mu.Unlock()

	if !exists || time.Since(session.CreatedAt) > oauthSessionTTL {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, oauthCallbackHTML("error", "Invalid or expired state", "", ""))
		return
	}

	// The authorization code is never exchanged for tokens (no client credentials are configured
	// and no driver reads OAuth tokens), so report that honestly instead of a made-up connection.
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusNotImplemented)
	fmt.Fprint(w, oauthCallbackHTML("error",
		"Browser sign-in is not supported for "+session.ProviderID+" yet. Add an API key instead.", session.ProviderID, ""))
}

// handleOAuthStatus returns the OAuth connection status for a provider.
// Also checks providerAuthStore for session_token auth state.
// GET /api/v1/providers/oauth/status?provider_id=xxx
func (r *Router) handleOAuthStatus(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		r.writeError(w, http.StatusMethodNotAllowed, "GET required")
		return
	}

	providerID := req.URL.Query().Get("provider_id")
	if providerID == "" {
		r.writeError(w, http.StatusBadRequest, "provider_id query parameter required")
		return
	}

	// Check OAuth token store first
	oauthStore.mu.RLock()
	tokenInfo, oauthExists := oauthStore.tokens[providerID]
	oauthStore.mu.RUnlock()

	if oauthExists && tokenInfo.IsConnected {
		r.writeJSON(w, http.StatusOK, map[string]interface{}{
			"provider_id":  tokenInfo.ProviderID,
			"auth_method":  tokenInfo.AuthMethod,
			"is_connected": tokenInfo.IsConnected,
			"connected_at": tokenInfo.ConnectedAt,
			"email":        tokenInfo.Email,
			"expires_at":   tokenInfo.ExpiresAt,
			"token_type":   tokenInfo.TokenType,
			"scopes":       tokenInfo.Scopes,
		})
		return
	}

	// Check provider auth store for session_token or api_key
	providerAuthStore.mu.RLock()
	authState, authExists := providerAuthStore.state[providerID]
	providerAuthStore.mu.RUnlock()

	if authExists && authState.AuthMethod == "session_token" && authState.SessionToken != "" {
		r.writeJSON(w, http.StatusOK, map[string]interface{}{
			"provider_id":  providerID,
			"auth_method":  "session_token",
			"is_connected": true,
			"connected_at": authState.SavedAt,
			"masked_key":   authState.MaskedKey,
		})
		return
	}

	if authExists && authState.AuthMethod == "api_key" && authState.APIKey != "" {
		r.writeJSON(w, http.StatusOK, map[string]interface{}{
			"provider_id":  providerID,
			"auth_method":  "api_key",
			"is_connected": true,
			"connected_at": authState.SavedAt,
			"masked_key":   authState.MaskedKey,
		})
		return
	}

	r.writeJSON(w, http.StatusOK, map[string]interface{}{
		"provider_id":  providerID,
		"auth_method":  "api_key",
		"is_connected": false,
	})
}

// handleOAuthDisconnect disconnects an OAuth session for a provider.
// POST /api/v1/providers/oauth/disconnect
func (r *Router) handleOAuthDisconnect(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		r.writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}

	var body struct {
		ProviderID string `json:"provider_id"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		r.writeError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	oauthStore.mu.Lock()
	delete(oauthStore.tokens, body.ProviderID)
	oauthStore.mu.Unlock()

	r.writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":      "disconnected",
		"provider_id": body.ProviderID,
	})
}

// handleOAuthProviders returns which providers support OAuth and their configuration.
// GET /api/v1/providers/oauth/providers
func (r *Router) handleOAuthProviders(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		r.writeError(w, http.StatusMethodNotAllowed, "GET required")
		return
	}

	type ProviderOAuthInfo struct {
		ProviderID    string `json:"provider_id"`
		SupportsOAuth bool   `json:"supports_oauth"`
		AuthURL       string `json:"auth_url"`
		Scopes        string `json:"scopes"`
		HasClientID   bool   `json:"has_client_id"`
	}

	result := make([]ProviderOAuthInfo, 0)
	for _, meta := range oauthProviderConfigs {
		result = append(result, ProviderOAuthInfo{
			ProviderID:    meta.ProviderID,
			SupportsOAuth: true,
			AuthURL:       meta.AuthURL,
			Scopes:        meta.Scopes,
			HasClientID:   meta.ClientID != "",
		})
	}

	r.writeJSON(w, http.StatusOK, result)
}

// oauthCallbackHTML generates the HTML page that communicates OAuth result back to the parent window via postMessage.
func oauthCallbackHTML(status, errorMsg, providerID, email string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>OAuth — Meta Orchestrator</title>
  <style>
    * { margin: 0; padding: 0; box-sizing: border-box; }
    body {
      font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', system-ui, sans-serif;
      background: #0f172a;
      color: #e2e8f0;
      display: flex;
      align-items: center;
      justify-content: center;
      height: 100vh;
      text-align: center;
    }
    .container {
      max-width: 380px;
      padding: 2rem;
    }
    .icon {
      width: 64px;
      height: 64px;
      border-radius: 50%%;
      display: flex;
      align-items: center;
      justify-content: center;
      margin: 0 auto 1.5rem;
      font-size: 32px;
    }
    .icon.success { background: #064e3b; color: #34d399; }
    .icon.error { background: #7f1d1d; color: #f87171; }
    h2 { font-size: 1.25rem; margin-bottom: 0.5rem; }
    p { font-size: 0.875rem; color: #94a3b8; margin-bottom: 1rem; }
    .closing { font-size: 0.75rem; color: #64748b; }
  </style>
</head>
<body>
  <div class="container">
    <div class="icon %s">%s</div>
    <h2>%s</h2>
    <p>%s</p>
    <p class="closing">This window will close automatically…</p>
  </div>
  <script>
    (function() {
      var result = %s;
      if (window.opener) {
        window.opener.postMessage(result, '*');
        setTimeout(function() { window.close(); }, 1500);
      } else {
        document.querySelector('.closing').textContent = 'You can close this window.';
      }
    })();
  </script>
</body>
</html>`,
		statusClass(status), statusIcon(status),
		statusTitle(status), html.EscapeString(statusDesc(status, errorMsg, providerID, email)),
		oauthCallbackResult(status, errorMsg, providerID, email),
	)
}

// oauthCallbackResult is the postMessage payload as a JS literal. errorMsg comes straight from the
// callback query string, so it is JSON-encoded (which escapes <, > and &) rather than spliced in.
func oauthCallbackResult(status, errorMsg, providerID, email string) string {
	b, _ := json.Marshal(map[string]string{
		"type":        "meta-orchestrator-oauth-callback",
		"status":      status,
		"provider_id": providerID,
		"email":       email,
		"error":       errorMsg,
	})
	return string(b)
}

func statusClass(status string) string {
	if status == "success" {
		return "success"
	}
	return "error"
}

func statusIcon(status string) string {
	if status == "success" {
		return "✓"
	}
	return "✗"
}

func statusTitle(status string) string {
	if status == "success" {
		return "Authorization Successful"
	}
	return "Authorization Failed"
}

func statusDesc(status, errorMsg, providerID, email string) string {
	if status == "success" {
		return fmt.Sprintf("Connected to %s as %s", providerID, email)
	}
	if errorMsg != "" {
		return errorMsg
	}
	return "An unknown error occurred during OAuth authorization."
}
