package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/connectors"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

func setupTestRouterWithConnectors(t *testing.T) (*Router, string, *httptest.Server) {
	tempDir, err := os.MkdirTemp("", "connectors_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	// Real mock HTTP server handling JIRA, Confluence, Slack, Bitbucket test endpoints
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "myself") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"displayName": "DevOps Engineer", "key": "devops"}`))
			return
		}
		if strings.Contains(r.URL.Path, "space") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"key": "ARCH", "name": "Architecture"}`))
			return
		}
		if strings.Contains(r.URL.Path, "content") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id": "1048576", "title": "Published Tech Doc", "_links": {"webui": "/spaces/ARCH/pages/1048576"}}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status": "ok", "ok": true, "user": "sdlc-bot", "team": "acme"}`))
	}))

	mgr := connectors.NewManager(tempDir)
	_ = mgr.UpdateJira(types.JiraConfig{
		Enabled:    true,
		BaseURL:    mockServer.URL,
		Username:   "devops@test.local",
		APIToken:   "test-token",
		ProjectKey: "PAY",
	})
	_ = mgr.UpdateConfluence(types.ConfluenceConfig{
		Enabled:   true,
		BaseURL:   mockServer.URL,
		Username:  "devops@test.local",
		APIToken:  "test-token",
		SpaceKey:  "ARCH",
	})

	router := NewRouter(RouterConfig{
		RootDir:           tempDir,
		ConnectorsManager: mgr,
	})

	return router, tempDir, mockServer
}

func TestConnectorsEndpoints(t *testing.T) {
	router, tempDir, mockServer := setupTestRouterWithConnectors(t)
	defer os.RemoveAll(tempDir)
	defer mockServer.Close()

	// 1. GET /api/v1/connectors
	req := httptest.NewRequest(http.MethodGet, "/api/v1/connectors", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/connectors returned status %d: %s", rec.Code, rec.Body.String())
	}
	var cfg types.ConnectorsConfig
	if err := json.Unmarshal(rec.Body.Bytes(), &cfg); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !cfg.Jira.Enabled || !cfg.Confluence.Enabled {
		t.Errorf("expected both JIRA and Confluence enabled by default, got jira=%v, conf=%v", cfg.Jira.Enabled, cfg.Confluence.Enabled)
	}

	// 2. POST /api/v1/connectors/test (Jira)
	testReqBody := types.TestConnectorRequest{Type: "jira"}
	b, _ := json.Marshal(testReqBody)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/connectors/test", bytes.NewReader(b))
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/v1/connectors/test (jira) returned %d", rec.Code)
	}
	var testResp types.TestConnectorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &testResp); err != nil {
		t.Fatalf("failed to unmarshal test response: %v", err)
	}
	if !testResp.Success {
		t.Errorf("expected testResp.Success to be true, got false. Message: %s", testResp.Message)
	}

	// 3. POST /api/v1/connectors/test (Confluence)
	testReqBody = types.TestConnectorRequest{Type: "confluence"}
	b, _ = json.Marshal(testReqBody)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/connectors/test", bytes.NewReader(b))
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/v1/connectors/test (confluence) returned %d", rec.Code)
	}
	testResp = types.TestConnectorResponse{}
	if err := json.Unmarshal(rec.Body.Bytes(), &testResp); err != nil {
		t.Fatalf("failed to unmarshal confluence test response: %v", err)
	}
	if !testResp.Success {
		t.Errorf("expected Confluence test success, got false. Message: %s", testResp.Message)
	}

	// 4. GET /api/v1/connectors/jira/issues
	req = httptest.NewRequest(http.MethodGet, "/api/v1/connectors/jira/issues?q=PAY", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/connectors/jira/issues returned %d", rec.Code)
	}
	var issues []types.JiraIssueDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &issues); err != nil {
		t.Fatalf("failed to decode issues: %v", err)
	}
	if len(issues) == 0 {
		t.Fatalf("expected non-empty issues list")
	}
	if issues[0].Key != "PAY-1044" {
		t.Errorf("expected first issue key PAY-1044, got %s", issues[0].Key)
	}

	// 5. POST /api/v1/connectors/jira/import
	importReq := types.ImportJiraIssueRequest{
		IssueKey:       "PAY-1044",
		WorkflowID:     "general_ai_sdlc",
		SelectedMethod: "BMAD",
		AssignedRepos:  []string{"backend-core", "frontend-portal"},
		StartStageID:   "prd_discovery",
	}
	b, _ = json.Marshal(importReq)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/connectors/jira/import", bytes.NewReader(b))
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/v1/connectors/jira/import returned %d: %s", rec.Code, rec.Body.String())
	}
	var createdTask types.Task
	if err := json.Unmarshal(rec.Body.Bytes(), &createdTask); err != nil {
		t.Fatalf("failed to unmarshal imported task: %v", err)
	}
	if createdTask.Metadata["jira_key"] != "PAY-1044" {
		t.Errorf("expected jira_key PAY-1044, got %s", createdTask.Metadata["jira_key"])
	}
	if createdTask.Metadata["jira_url"] == "" {
		t.Errorf("expected jira_url to be set")
	}
	if !strings.Contains(createdTask.Title, "PAY-1044") {
		t.Errorf("expected title to contain PAY-1044, got %s", createdTask.Title)
	}

	// 6. POST /api/v1/connectors/confluence/publish
	pubReq := types.ConfluencePublishRequest{
		TaskID:          createdTask.ID,
		Title:           "Technical Design: Stripe 3D-Secure 2.0 Flow",
		ContentMarkdown: "# Stripe 3D-Secure Architecture\n\nFull design specifications.",
		SpaceKey:        "ARCH",
		DocType:         "TECH_DOC_RFC",
	}
	b, _ = json.Marshal(pubReq)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/connectors/confluence/publish", bytes.NewReader(b))
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/v1/connectors/confluence/publish returned %d: %s", rec.Code, rec.Body.String())
	}
	var pubResp types.ConfluencePublishResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &pubResp); err != nil {
		t.Fatalf("failed to unmarshal confluence publish response: %v", err)
	}
	if pubResp.PageURL == "" || pubResp.PageID == "" {
		t.Errorf("expected PageURL and PageID in publish response, got url=%s id=%s", pubResp.PageURL, pubResp.PageID)
	}
	if pubResp.SpaceKey != "ARCH" {
		t.Errorf("expected space ARCH, got %s", pubResp.SpaceKey)
	}

	// Verify task metadata has been updated with confluence link
	router.mu.RLock()
	taskInStore := router.tasks[createdTask.ID]
	router.mu.RUnlock()
	if taskInStore == nil {
		t.Fatalf("task %s not found in store", createdTask.ID)
	}
	if taskInStore.Metadata["confluence_page_url"] != pubResp.PageURL {
		t.Errorf("expected task confluence_page_url %s, got %s", pubResp.PageURL, taskInStore.Metadata["confluence_page_url"])
	}
}

func TestConnectorsCatalogEndpoints(t *testing.T) {
	router, tempDir, mockServer := setupTestRouterWithConnectors(t)
	defer os.RemoveAll(tempDir)
	defer mockServer.Close()

	// 1. GET /api/v1/connectors/catalog (includes bitbucket)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/connectors/catalog", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/connectors/catalog returned %d", rec.Code)
	}
	var catalog []*types.ConnectorItem
	if err := json.Unmarshal(rec.Body.Bytes(), &catalog); err != nil {
		t.Fatalf("failed to decode catalog: %v", err)
	}
	if len(catalog) < 9 {
		t.Fatalf("expected at least 9 catalog items (including Bitbucket), got %d", len(catalog))
	}

	// 2. GET /api/v1/connectors/items/bitbucket
	req = httptest.NewRequest(http.MethodGet, "/api/v1/connectors/items/bitbucket", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/connectors/items/bitbucket returned %d", rec.Code)
	}
	var bbItem types.ConnectorItem
	if err := json.Unmarshal(rec.Body.Bytes(), &bbItem); err != nil {
		t.Fatalf("failed to decode bitbucket item: %v", err)
	}
	if bbItem.ID != "bitbucket" || bbItem.Category != types.ConnectorCategoryVCS || bbItem.MCP == nil {
		t.Errorf("unexpected bitbucket item: %+v", bbItem)
	}

	// 3. POST /api/v1/connectors/items/slack/toggle
	toggleReq := types.ToggleConnectorRequest{Enabled: true}
	b, _ := json.Marshal(toggleReq)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/connectors/items/slack/toggle", bytes.NewReader(b))
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/v1/connectors/items/slack/toggle returned %d: %s", rec.Code, rec.Body.String())
	}
	var toggledItem types.ConnectorItem
	if err := json.Unmarshal(rec.Body.Bytes(), &toggledItem); err != nil {
		t.Fatalf("failed to decode toggled item: %v", err)
	}
	if !toggledItem.Enabled {
		t.Errorf("expected slack to be enabled after toggle")
	}

	// 4. PUT /api/v1/connectors/items/slack
	toggledItem.TargetEntity = "#engineering-alerts"
	toggledItem.BaseURL = mockServer.URL
	b, _ = json.Marshal(toggledItem)
	req = httptest.NewRequest(http.MethodPut, "/api/v1/connectors/items/slack", bytes.NewReader(b))
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("PUT /api/v1/connectors/items/slack returned %d", rec.Code)
	}

	// 5. POST /api/v1/connectors/items/slack/test (testing against real loopback mock server)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/connectors/items/slack/test", bytes.NewReader(b))
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/v1/connectors/items/slack/test returned %d", rec.Code)
	}
	var testResp types.TestConnectorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &testResp); err != nil {
		t.Fatalf("failed to decode test response: %v", err)
	}
	if !testResp.Success {
		t.Errorf("expected test success, got false. Message: %s", testResp.Message)
	}

	// 6. POST /api/v1/connectors/items/github/test-mcp (validates real npx on system)
	mcpTestPayload := types.MCPConfig{
		Enabled:   true,
		Command:   "npx",
		Args:      []string{"-y", "@modelcontextprotocol/server-github"},
		Transport: "stdio",
	}
	b, _ = json.Marshal(mcpTestPayload)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/connectors/items/github/test-mcp", bytes.NewReader(b))
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/v1/connectors/items/github/test-mcp returned %d", rec.Code)
	}
	var mcpResp types.TestConnectorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &mcpResp); err != nil {
		t.Fatalf("failed to decode mcp test response: %v", err)
	}
	if !mcpResp.Success {
		t.Errorf("expected MCP test success for npx, got false: %s", mcpResp.Message)
	}

	// 7. GET /api/v1/connectors/mcp-config (export to mcpServers dictionary)
	req = httptest.NewRequest(http.MethodGet, "/api/v1/connectors/mcp-config", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/connectors/mcp-config returned %d", rec.Code)
	}
	var mcpExport map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &mcpExport); err != nil {
		t.Fatalf("failed to decode mcp config: %v", err)
	}
	servers, ok := mcpExport["mcpServers"].(map[string]interface{})
	if !ok || len(servers) == 0 {
		t.Errorf("expected non-empty mcpServers in export, got: %+v", mcpExport)
	}
}

