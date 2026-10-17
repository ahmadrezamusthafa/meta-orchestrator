package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/connectors"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/projects"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

func TestConnectorSuggestionsAndBrief(t *testing.T) {
	for _, k := range []string{"GITHUB_TOKEN", "GH_TOKEN", "GITHUB_PERSONAL_ACCESS_TOKEN", "GRAFANA_SERVICE_ACCOUNT_TOKEN", "JIRA_API_TOKEN",
		"ATLASSIAN_API_TOKEN", "CONFLUENCE_API_TOKEN", "BITBUCKET_APP_PASSWORD", "BITBUCKET_TOKEN", "BITBUCKET_API_TOKEN"} {
		t.Setenv(k, "")
	}
	root, svc := t.TempDir(), t.TempDir()
	_ = os.WriteFile(filepath.Join(svc, "go.mod"), []byte("module svc\nrequire go.opentelemetry.io/otel v1.0.0"), 0o644)
	_ = os.MkdirAll(filepath.Join(svc, ".git"), 0o755)
	_ = os.WriteFile(filepath.Join(svc, ".git", "config"), []byte("[remote \"origin\"]\n\turl = git@github.com:acme/svc.git\n"), 0o644)

	pm := projects.NewProjectManager(root)
	if _, err := pm.Create(&types.Project{Name: "Payments", Repos: []types.ProjectRepo{{Name: "svc", Path: svc}}}); err != nil {
		t.Fatal(err)
	}
	cm := connectors.NewManager(root)
	t.Cleanup(cm.StopPeriodicPinger)
	r := NewRouter(RouterConfig{RootDir: root, ProjectManager: pm, ConnectorsManager: cm, Toolchain: bareMachine("brew")})
	r.mu.Lock()
	r.tasks["TASK-5"] = &types.Task{ID: "TASK-5", Title: "Trace refunds", CurrentStageID: "techdoc_rfc", AssignedRepos: []string{"svc"}, Metadata: map[string]string{}}
	r.mu.Unlock()

	w, _ := tcDo(t, r, http.MethodGet, "/api/v1/connectors/suggestions?task=TASK-5", "")
	if w.Code != http.StatusOK {
		t.Fatalf("suggestions = %d", w.Code)
	}
	got := r.connectorSuggestions([]string{"svc"})
	ids := []string{}
	for _, s := range got {
		ids = append(ids, s.ID)
		if s.Ready {
			t.Errorf("%s is not set up, so it is not ready", s.ID)
		}
	}
	if strings.Join(ids, ",") != "github,grafana" || !strings.Contains(got[1].Reasons[0], "OpenTelemetry") {
		t.Fatalf("suggestions = %+v", got)
	}

	brief := r.connectorsBrief(r.taskSnapshot("TASK-5"))
	if !strings.Contains(brief, "Not connected, though these repositories use them: GitHub Issues & PRs, Grafana") {
		t.Fatalf("brief should name the missing connectors:\n%s", brief)
	}

	// Connect Grafana: agents get it, with what they may do.
	it, _ := cm.GetConnector("grafana")
	cp := *it
	cp.Enabled, cp.Status, cp.BaseURL, cp.APIToken = true, "connected", "https://grafana.example.com", "glsa_token_for_test_1234"
	if _, err := cm.UpdateConnector(cp); err != nil {
		t.Fatal(err)
	}
	brief = r.connectorsBrief(r.taskSnapshot("TASK-5"))
	if !strings.Contains(brief, "- Grafana (`mcp__grafana__*`): Read-only") || strings.Contains(brief, "glsa_token") {
		t.Fatalf("brief should list Grafana without its token:\n%s", brief)
	}
	path, cleanup := r.agentMCPConfig("TASK-5", "turn-1")
	if path == "" {
		t.Fatal("a connected connector produces a per-turn MCP config")
	}
	data, _ := os.ReadFile(path)
	cleanup()
	if !strings.Contains(string(data), `"grafana"`) {
		t.Fatalf("per-turn config = %s", data)
	}

	// The connector's native server is recommended in the toolchain once it is set up.
	rep := r.toolchainFor([]string{"svc"}, false)
	found := false
	for _, item := range rep.Items {
		found = found || item.ID == "mcp-grafana"
	}
	if !found {
		t.Fatal("an enabled Grafana connector recommends mcp-grafana")
	}
}
