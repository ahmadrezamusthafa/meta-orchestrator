package connectors

import (
	"context"
	"testing"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

func TestConnectorsManager(t *testing.T) {
	tempDir := t.TempDir()
	mgr := NewManager(tempDir)

	// 1. Get initial config
	cfg := mgr.GetConfig()
	if !cfg.Jira.Enabled || !cfg.Confluence.Enabled {
		t.Errorf("Expected JIRA and Confluence to be enabled by default")
	}

	// 2. Test JIRA connectivity check
	jiraResp := mgr.TestJira(context.Background(), cfg.Jira)
	if !jiraResp.Success {
		t.Fatalf("Expected JIRA test to succeed, got: %s", jiraResp.Message)
	}

	// 3. Test Confluence connectivity check
	confResp := mgr.TestConfluence(context.Background(), cfg.Confluence)
	if !confResp.Success {
		t.Fatalf("Expected Confluence test to succeed, got: %s", confResp.Message)
	}

	// 4. Test JIRA issue search
	issues, err := mgr.SearchJiraIssues(context.Background(), "")
	if err != nil || len(issues) == 0 {
		t.Fatalf("Expected mock JIRA issues to be returned, got err: %v, count: %d", err, len(issues))
	}

	// 5. Test JIRA key detection
	detectedKey := mgr.DetectJiraKey("Implement Stripe idempotency in [PAY-1042]")
	if detectedKey != "PAY-1042" {
		t.Errorf("Expected PAY-1042, got: %s", detectedKey)
	}

	// 6. Test Confluence Document Publishing
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
