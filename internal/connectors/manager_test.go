package connectors

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

func TestConnectorsManager(t *testing.T) {
	tempDir := t.TempDir()
	mgr := NewManager(tempDir)

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
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status": "ok"}`))
	}))
	defer mockServer.Close()

	// 1. Get initial config
	cfg := mgr.GetConfig()
	if !cfg.Jira.Enabled || !cfg.Confluence.Enabled {
		t.Errorf("Expected JIRA and Confluence to be enabled by default")
	}

	cfg.Jira.BaseURL = mockServer.URL
	cfg.Jira.Username = "devops@test.local"
	cfg.Jira.APIToken = "secret-token"

	cfg.Confluence.BaseURL = mockServer.URL
	cfg.Confluence.Username = "devops@test.local"
	cfg.Confluence.APIToken = "secret-token"

	// 2. Test JIRA connectivity check against real server
	jiraResp := mgr.TestJira(context.Background(), cfg.Jira)
	if !jiraResp.Success {
		t.Fatalf("Expected JIRA test to succeed, got: %s", jiraResp.Message)
	}

	// 3. Test Confluence connectivity check against real server
	confResp := mgr.TestConfluence(context.Background(), cfg.Confluence)
	if !confResp.Success {
		t.Fatalf("Expected Confluence test to succeed, got: %s", confResp.Message)
	}

	// 4. Test MCP verification (verifies real npx on system)
	mcpResp := mgr.TestMCPConnector(context.Background(), "github", &types.MCPConfig{
		Enabled: true,
		Command: "npx",
		Args:    []string{"-y", "@modelcontextprotocol/server-github"},
	})
	if !mcpResp.Success {
		t.Fatalf("Expected MCP check for npx to succeed, got: %s", mcpResp.Message)
	}

	// 5. Test JIRA issue search
	issues, err := mgr.SearchJiraIssues(context.Background(), "")
	if err != nil || len(issues) == 0 {
		t.Fatalf("Expected mock JIRA issues to be returned, got err: %v, count: %d", err, len(issues))
	}

	// 6. Test JIRA key detection
	detectedKey := mgr.DetectJiraKey("Implement Stripe idempotency in [PAY-1042]")
	if detectedKey != "PAY-1042" {
		t.Errorf("Expected PAY-1042, got: %s", detectedKey)
	}

	// 7. Test Confluence Document Publishing
	pubResp, err := mgr.PublishToConfluence(context.Background(), types.ConfluencePublishRequest{
		TaskID:          "TASK-8942",
		Title:           "Stripe Payment Gateway Architecture RFC",
		ContentMarkdown: "# Architecture Overview\n- Database migration\n- Redis locks\n```go\nfunc process(){}\n```",
		SpaceKey:        "ARCH",
	})
	if err != nil || !pubResp.Success {
		t.Fatalf("Expected Confluence publish to succeed, got err: %v", err)
	}
	if pubResp.PageURL == "" {
		t.Errorf("Expected non-empty Confluence page URL")
	}
}
