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
	isolateJiraEnv(t)
	tempDir := t.TempDir()
	mgr := NewManager(tempDir)

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/search") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"issues":[{"key":"PAY-7","fields":{"summary":"Real issue","status":{"name":"To Do"}}}]}`))
			return
		}
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
	if _, err := NewManager(t.TempDir()).SearchJiraIssues(context.Background(), ""); err != ErrJiraNotConnected {
		t.Fatalf("Expected ErrJiraNotConnected without credentials, got %v", err)
	}
	if err := mgr.UpdateJira(cfg.Jira); err != nil {
		t.Fatalf("UpdateJira: %v", err)
	}
	issues, err := mgr.SearchJiraIssues(context.Background(), "")
	if err != nil || len(issues) != 1 || issues[0].Key != "PAY-7" {
		t.Fatalf("Expected the live JIRA issue, got err: %v, issues: %+v", err, issues)
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

	// 8. Test WebSocket event broadcast hook
	var receivedEvents []*types.OrchestratorEvent
	mgr.SetBroadcastFunc(func(ev *types.OrchestratorEvent) {
		receivedEvents = append(receivedEvents, ev)
	})

	// 9. Test PingAll
	summary := mgr.PingAll(context.Background())
	if summary == nil || summary.TotalPinged == 0 {
		t.Fatalf("Expected PingAll to return summary with pinged connectors")
	}
	if len(receivedEvents) == 0 {
		t.Errorf("Expected broadcast events to be emitted on PingAll")
	}

	// 10. Test PingConfig
	pingCfg := mgr.GetPingConfig()
	if !pingCfg.Enabled || pingCfg.IntervalSeconds <= 0 {
		t.Errorf("Expected default ping config enabled and interval > 0, got: %+v", pingCfg)
	}

	err = mgr.UpdatePingConfig(types.ConnectorPingConfig{
		Enabled:         true,
		IntervalSeconds: 45,
	})
	if err != nil {
		t.Fatalf("Failed to update ping config: %v", err)
	}
	updatedPing := mgr.GetPingConfig()
	if updatedPing.IntervalSeconds != 45 {
		t.Errorf("Expected interval 45, got %d", updatedPing.IntervalSeconds)
	}

	mgr.StopPeriodicPinger()
}

// isolateJiraEnv hides the developer's real JIRA credentials so tests only ever talk to httptest servers.
func isolateJiraEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{"JIRA_EMAIL", "JIRA_USERNAME", "ATLASSIAN_EMAIL", "JIRA_API_TOKEN", "JIRA_TOKEN",
		"ATLASSIAN_API_TOKEN", "JIRA_URL", "JIRA_BASE_URL", "CONFLUENCE_EMAIL", "CONFLUENCE_USERNAME",
		"CONFLUENCE_API_TOKEN", "CONFLUENCE_TOKEN", "CONFLUENCE_URL", "CONFLUENCE_BASE_URL"} {
		t.Setenv(k, "")
	}
}

func TestOpenOnlyJQL(t *testing.T) {
	cases := map[string]string{
		"":                                    "statusCategory != Done",
		"assignee = currentUser()":            "(assignee = currentUser()) AND statusCategory != Done",
		"project = PAY ORDER BY updated DESC": "(project = PAY) AND statusCategory != Done ORDER BY updated DESC",
		"project = PAY order  by rank":        "(project = PAY) AND statusCategory != Done order  by rank",
		"ORDER BY created":                    "statusCategory != Done ORDER BY created",
	}
	for in, want := range cases {
		if got := OpenOnlyJQL(in); got != want {
			t.Errorf("OpenOnlyJQL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIssueEpicResolution(t *testing.T) {
	one := 1
	var story jiraIssueFields
	story.Key = "PAY-2"
	story.Fields.Parent = &struct {
		Key    string `json:"key"`
		Fields struct {
			Summary   string        `json:"summary"`
			IssueType jiraIssueType `json:"issuetype"`
		} `json:"fields"`
	}{Key: "PAY-1"}
	story.Fields.Parent.Fields.Summary = "Checkout revamp"
	story.Fields.Parent.Fields.IssueType = jiraIssueType{Name: "Epic", HierarchyLevel: &one}
	if d := story.toDTO("https://x"); d.EpicKey != "PAY-1" || d.EpicSummary != "Checkout revamp" || d.ParentKey != "PAY-1" {
		t.Fatalf("story under epic: %+v", d)
	}

	var epic jiraIssueFields
	epic.Key = "PAY-1"
	epic.Fields.Summary = "Checkout revamp"
	epic.Fields.IssueType = jiraIssueType{Name: "Epic"}
	if d := epic.toDTO("https://x"); d.EpicKey != "PAY-1" {
		t.Fatalf("an epic groups under itself: %+v", d)
	}

	var legacy jiraIssueFields
	legacy.Key = "PAY-3"
	legacy.Fields.EpicLink = "PAY-9"
	if d := legacy.toDTO("https://x"); d.EpicKey != "PAY-9" {
		t.Fatalf("legacy Epic Link: %+v", d)
	}
}

func TestNewDefaultExclusionsReachSavedRules(t *testing.T) {
	mgr := NewManager(t.TempDir())
	mgr.mu.Lock()
	mgr.config.JiraSync = &types.JiraSyncConfig{JQL: "project = PAY", ExcludeStatuses: []string{"Done"}, DefaultsVersion: 1}
	mgr.mu.Unlock()
	cfg := mgr.GetJiraSyncConfig()
	if !containsFold(cfg.ExcludeStatuses, "Won't Fix") || len(cfg.ExcludeStatuses) != 2 {
		t.Fatalf("Won't Fix should be added to older saved rules without restoring others: %v", cfg.ExcludeStatuses)
	}
	saved, _ := mgr.UpdateJiraSyncConfig(types.JiraSyncConfig{JQL: "x", ExcludeStatuses: []string{"Done"}})
	if containsFold(saved.ExcludeStatuses, "Won't Fix") || containsFold(mgr.GetJiraSyncConfig().ExcludeStatuses, "Won't Fix") {
		t.Fatalf("an operator who removes Won't Fix keeps it removed: %v", mgr.GetJiraSyncConfig().ExcludeStatuses)
	}
}
