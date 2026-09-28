package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOAuthEndpoints(t *testing.T) {
	router := NewRouter(RouterConfig{RootDir: t.TempDir()})

	// 1. List OAuth-capable providers
	req := httptest.NewRequest(http.MethodGet, "/api/v1/providers/oauth/providers", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /providers/oauth/providers: expected 200, got %d", w.Code)
	}

	var providers []map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &providers); err != nil {
		t.Fatalf("Failed to decode OAuth providers list: %v", err)
	}
	if len(providers) == 0 {
		t.Fatal("Expected at least one OAuth-capable provider")
	}

	// Verify each provider has expected fields
	for _, p := range providers {
		if _, ok := p["provider_id"]; !ok {
			t.Error("Provider missing provider_id")
		}
		if _, ok := p["supports_oauth"]; !ok {
			t.Error("Provider missing supports_oauth")
		}
	}

	// 2. OAuth status for a provider that hasn't connected yet
	req = httptest.NewRequest(http.MethodGet, "/api/v1/providers/oauth/status?provider_id=claude", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /providers/oauth/status: expected 200, got %d", w.Code)
	}

	var status map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &status); err != nil {
		t.Fatalf("Failed to decode OAuth status: %v", err)
	}
	if status["is_connected"] != false {
		t.Error("Expected is_connected=false for unconfigured provider")
	}

	// 3. Initiate OAuth flow
	payload, _ := json.Marshal(map[string]string{
		"provider_id":  "antigravity",
		"redirect_uri": "http://localhost:3000/api/v1/providers/oauth/callback",
	})
	req = httptest.NewRequest(http.MethodPost, "/api/v1/providers/oauth/initiate", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("POST /providers/oauth/initiate: expected 200, got %d; body=%s", w.Code, w.Body.String())
	}

	var initiateResp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &initiateResp); err != nil {
		t.Fatalf("Failed to decode initiate response: %v", err)
	}
	if initiateResp["auth_url"] == nil || initiateResp["auth_url"] == "" {
		t.Error("Expected non-empty auth_url in initiate response")
	}
	if initiateResp["state"] == nil || initiateResp["state"] == "" {
		t.Error("Expected non-empty state in initiate response")
	}

	state := initiateResp["state"].(string)

	// 4. Simulate callback (as if provider redirected back)
	req = httptest.NewRequest(http.MethodGet, "/api/v1/providers/oauth/callback?state="+state+"&code=test-auth-code-123", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /providers/oauth/callback: expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("Expected text/html content type, got %s", ct)
	}
	body := w.Body.String()
	if !bytes.Contains([]byte(body), []byte("meta-orchestrator-oauth-callback")) {
		t.Error("Callback HTML should contain postMessage type identifier")
	}
	if !bytes.Contains([]byte(body), []byte("success")) {
		t.Error("Callback HTML should indicate success status")
	}

	// 5. Verify OAuth status is now connected
	req = httptest.NewRequest(http.MethodGet, "/api/v1/providers/oauth/status?provider_id=antigravity", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /providers/oauth/status after connect: expected 200, got %d", w.Code)
	}

	if err := json.Unmarshal(w.Body.Bytes(), &status); err != nil {
		t.Fatalf("Failed to decode status: %v", err)
	}
	if status["is_connected"] != true {
		t.Error("Expected is_connected=true after successful OAuth callback")
	}
	if status["auth_method"] != "oauth" {
		t.Errorf("Expected auth_method='oauth', got '%v'", status["auth_method"])
	}

	// 6. Disconnect OAuth
	disconnectPayload, _ := json.Marshal(map[string]string{"provider_id": "antigravity"})
	req = httptest.NewRequest(http.MethodPost, "/api/v1/providers/oauth/disconnect", bytes.NewReader(disconnectPayload))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("POST /providers/oauth/disconnect: expected 200, got %d", w.Code)
	}

	var disconnResp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &disconnResp); err != nil {
		t.Fatalf("Failed to decode disconnect response: %v", err)
	}
	if disconnResp["status"] != "disconnected" {
		t.Errorf("Expected status='disconnected', got '%v'", disconnResp["status"])
	}

	// 7. Verify status is disconnected
	req = httptest.NewRequest(http.MethodGet, "/api/v1/providers/oauth/status?provider_id=antigravity", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if err := json.Unmarshal(w.Body.Bytes(), &status); err != nil {
		t.Fatalf("Failed to decode status after disconnect: %v", err)
	}
	if status["is_connected"] != false {
		t.Error("Expected is_connected=false after disconnecting")
	}

	// 8. Error case: initiate with unsupported provider
	badPayload, _ := json.Marshal(map[string]string{"provider_id": "opencode"})
	req = httptest.NewRequest(http.MethodPost, "/api/v1/providers/oauth/initiate", bytes.NewReader(badPayload))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 for unsupported provider OAuth, got %d", w.Code)
	}

	// 9. Callback with invalid state
	req = httptest.NewRequest(http.MethodGet, "/api/v1/providers/oauth/callback?state=bogus-state&code=some-code", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 for invalid state, got %d", w.Code)
	}

	// 10. Callback with error parameter
	req = httptest.NewRequest(http.MethodGet, "/api/v1/providers/oauth/callback?error=access_denied", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("Expected 200 for error callback (renders HTML), got %d", w.Code)
	}
	body = w.Body.String()
	if !bytes.Contains([]byte(body), []byte("error")) {
		t.Error("Error callback should contain error status in HTML")
	}
}

func TestOAuthProvidersList_Contains_ExpectedProviders(t *testing.T) {
	router := NewRouter(RouterConfig{RootDir: t.TempDir()})

	// Verify the main providers endpoint now includes auth fields
	req := httptest.NewRequest(http.MethodGet, "/api/v1/providers", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to decode providers response: %v", err)
	}

	providersList, ok := resp["providers"].([]interface{})
	if !ok || len(providersList) == 0 {
		t.Fatal("Expected non-empty providers list")
	}

	// Verify that each provider has the auth_method and supports_oauth fields
	for _, p := range providersList {
		pMap := p.(map[string]interface{})
		if _, ok := pMap["auth_method"]; !ok {
			t.Errorf("Provider %s missing auth_method field", pMap["id"])
		}
		if _, ok := pMap["supports_oauth"]; !ok {
			t.Errorf("Provider %s missing supports_oauth field", pMap["id"])
		}
	}
}

func TestSessionTokenPersistence(t *testing.T) {
	router := NewRouter(RouterConfig{RootDir: t.TempDir()})

	// 1. Verify initial state: claude should be api_key
	req := httptest.NewRequest(http.MethodGet, "/api/v1/providers", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	providers := resp["providers"].([]interface{})

	var claudeProvider map[string]interface{}
	for _, p := range providers {
		pm := p.(map[string]interface{})
		if pm["id"] == "claude" {
			claudeProvider = pm
			break
		}
	}
	if claudeProvider["auth_method"] != "api_key" {
		t.Fatalf("Expected initial auth_method='api_key', got '%v'", claudeProvider["auth_method"])
	}

	// 2. Save a session token for claude
	savePayload, _ := json.Marshal(map[string]string{
		"provider_id":   "claude",
		"session_token": "sk-ant-sid01-verylongsessiontokenvalue1234567890",
		"auth_method":   "session_token",
	})
	req = httptest.NewRequest(http.MethodPost, "/api/v1/providers", bytes.NewReader(savePayload))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("POST /providers: expected 200, got %d", w.Code)
	}

	var saveResp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &saveResp)
	if saveResp["auth_method"] != "session_token" {
		t.Errorf("Expected saved auth_method='session_token', got '%v'", saveResp["auth_method"])
	}
	if saveResp["masked_key"] == nil || saveResp["masked_key"] == "" {
		t.Error("Expected non-empty masked_key in save response")
	}

	// 3. Simulate "switching tabs" — GET /providers should now return session_token for claude
	req = httptest.NewRequest(http.MethodGet, "/api/v1/providers", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	json.Unmarshal(w.Body.Bytes(), &resp)
	providers = resp["providers"].([]interface{})

	for _, p := range providers {
		pm := p.(map[string]interface{})
		if pm["id"] == "claude" {
			claudeProvider = pm
			break
		}
	}
	if claudeProvider["auth_method"] != "session_token" {
		t.Fatalf("After save, expected auth_method='session_token' on GET, got '%v'", claudeProvider["auth_method"])
	}

	// 4. Check via status endpoint — should also show connected
	req = httptest.NewRequest(http.MethodGet, "/api/v1/providers/oauth/status?provider_id=claude", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var status map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &status)
	if status["is_connected"] != true {
		t.Error("Expected is_connected=true from status endpoint after saving session token")
	}
	if status["auth_method"] != "session_token" {
		t.Errorf("Expected auth_method='session_token' from status, got '%v'", status["auth_method"])
	}

	// 5. Test connection — should report CONNECTED with session_token auth method
	testPayload, _ := json.Marshal(map[string]string{"provider_id": "claude"})
	req = httptest.NewRequest(http.MethodPost, "/api/v1/providers/test", bytes.NewReader(testPayload))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var testResp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &testResp)
	if testResp["status"] != "CONNECTED" {
		t.Errorf("Expected test status='CONNECTED', got '%v'", testResp["status"])
	}
	if testResp["auth_method"] != "session_token" {
		t.Errorf("Expected test auth_method='session_token', got '%v'", testResp["auth_method"])
	}

	// 6. Clear session token (send empty token)
	clearPayload, _ := json.Marshal(map[string]string{
		"provider_id":   "claude",
		"session_token": "",
		"auth_method":   "session_token",
	})
	req = httptest.NewRequest(http.MethodPost, "/api/v1/providers", bytes.NewReader(clearPayload))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var clearResp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &clearResp)
	if clearResp["auth_method"] != "api_key" {
		t.Errorf("After clearing, expected auth_method='api_key', got '%v'", clearResp["auth_method"])
	}

	// 7. Verify GET /providers now shows api_key again
	req = httptest.NewRequest(http.MethodGet, "/api/v1/providers", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	json.Unmarshal(w.Body.Bytes(), &resp)
	providers = resp["providers"].([]interface{})
	for _, p := range providers {
		pm := p.(map[string]interface{})
		if pm["id"] == "claude" {
			claudeProvider = pm
			break
		}
	}
	if claudeProvider["auth_method"] != "api_key" {
		t.Fatalf("After clearing, expected auth_method='api_key', got '%v'", claudeProvider["auth_method"])
	}
}
