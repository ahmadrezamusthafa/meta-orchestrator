package connectors

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// TestDisabledConnectorsMakeNoCalls: a connector switched off never reaches its service, and a
// test of it does not report it as connected again.
func TestDisabledConnectorsMakeNoCalls(t *testing.T) {
	isolateJiraEnv(t)
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"issues":[]}`))
	}))
	defer srv.Close()

	mgr := NewManager(t.TempDir())
	cfg := mgr.GetConfig()
	cfg.Jira.BaseURL, cfg.Jira.Username, cfg.Jira.APIToken = srv.URL, "devops@test.local", "secret-token"
	if err := mgr.UpdateJira(cfg.Jira); err != nil {
		t.Fatal(err)
	}
	cfg.Confluence.BaseURL, cfg.Confluence.Username, cfg.Confluence.APIToken = srv.URL, "devops@test.local", "secret-token"
	if err := mgr.UpdateConfluence(cfg.Confluence); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"jira", "confluence"} {
		if _, err := mgr.ToggleConnector(id, false); err != nil {
			t.Fatal(err)
		}
	}

	if mgr.JiraEnabled() || mgr.ConfluenceEnabled() {
		t.Fatal("expected JIRA and Confluence to report disabled")
	}
	if _, err := mgr.QueryJiraIssues(context.Background(), "project = PAY", 10); !errors.Is(err, ErrJiraDisabled) {
		t.Errorf("QueryJiraIssues: want ErrJiraDisabled, got %v", err)
	}
	if _, err := mgr.GetJiraIssue(context.Background(), "PAY-1"); !errors.Is(err, ErrJiraDisabled) {
		t.Errorf("GetJiraIssue: want ErrJiraDisabled, got %v", err)
	}
	if _, err := mgr.PublishToConfluence(context.Background(), types.ConfluencePublishRequest{Title: "x", ContentMarkdown: "y"}); !errors.Is(err, ErrConfluenceDisabled) {
		t.Errorf("PublishToConfluence: want ErrConfluenceDisabled, got %v", err)
	}
	if n := hits.Load(); n != 0 {
		t.Errorf("disabled connectors made %d HTTP calls", n)
	}
	pinged := mgr.PingAll(context.Background()).Results
	if _, ok := pinged["jira"]; ok {
		t.Error("PingAll probed the disabled JIRA connector")
	}
	if _, ok := pinged["confluence"]; ok {
		t.Error("PingAll probed the disabled Confluence connector")
	}

	mgr.updateItemStatus("jira", true, 5, "")
	if it, _ := mgr.GetConnector("jira"); it.Status != "disabled" {
		t.Errorf("a passing test of a disabled connector set status %q, want disabled", it.Status)
	}

	if _, err := mgr.ToggleConnector("jira", true); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.QueryJiraIssues(context.Background(), "project = PAY", 10); err != nil {
		t.Errorf("re-enabled JIRA: %v", err)
	}
}
