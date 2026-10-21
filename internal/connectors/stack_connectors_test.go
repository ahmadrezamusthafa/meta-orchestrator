package connectors

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// isolate keeps the host's tokens and tools out of the test.
func isolate(t *testing.T, installed ...string) {
	t.Helper()
	for _, k := range []string{"GITHUB_TOKEN", "GH_TOKEN", "GITHUB_PERSONAL_ACCESS_TOKEN", "GITHUB_API_TOKEN", "GRAFANA_SERVICE_ACCOUNT_TOKEN",
		"GRAFANA_API_KEY", "GRAFANA_TOKEN", "CLOUDFLARE_API_TOKEN", "CF_API_TOKEN", "JIRA_API_TOKEN", "JIRA_TOKEN", "ATLASSIAN_API_TOKEN",
		"CONFLUENCE_API_TOKEN", "CONFLUENCE_TOKEN", "BITBUCKET_APP_PASSWORD", "BITBUCKET_TOKEN", "BITBUCKET_API_TOKEN", "BITBUCKET_PASSWORD",
		"GITLAB_TOKEN", "GITLAB_PERSONAL_ACCESS_TOKEN", "GITLAB_API_TOKEN", "GL_TOKEN", "SLACK_BOT_TOKEN", "SLACK_TOKEN", "LINEAR_API_KEY",
		"LINEAR_TOKEN", "LINEAR_API_TOKEN", "NOTION_API_KEY", "NOTION_TOKEN", "NOTION_INTEGRATION_TOKEN", "CLICKUP_API_KEY", "CLICKUP_API_TOKEN", "CLICKUP_TOKEN",
		"CLICKUP_TEAM_ID", "TRELLO_API_KEY", "TRELLO_TOKEN", "TRELLO_API_TOKEN", "ASANA_ACCESS_TOKEN", "ASANA_TOKEN", "ASANA_PAT",
		"MONDAY_API_TOKEN", "MONDAY_TOKEN"} {
		t.Setenv(k, "")
	}
	have := map[string]bool{}
	for _, b := range installed {
		have[b] = true
	}
	old := lookPath
	lookPath = func(name string) (string, error) {
		if have[name] {
			return "/usr/local/bin/" + name, nil
		}
		return "", errors.New("not found")
	}
	t.Cleanup(func() { lookPath = old })
}

func newTestManager(t *testing.T, saved *types.ConnectorsConfig) *Manager {
	t.Helper()
	root := t.TempDir()
	if saved != nil {
		_ = os.MkdirAll(filepath.Join(root, ".sdlc"), 0o755)
		b, _ := json.Marshal(saved)
		_ = os.WriteFile(filepath.Join(root, ".sdlc", "connectors.json"), b, 0o644)
	}
	m := NewManager(root)
	t.Cleanup(m.StopPeriodicPinger)
	return m
}

func TestStackConnectorsAreInTheCatalogAndOptIn(t *testing.T) {
	isolate(t, "docker")
	m := newTestManager(t, nil)
	for _, id := range []string{"grafana", "azure", "cloudflare"} {
		it, err := m.GetConnector(id)
		if err != nil {
			t.Fatalf("%s missing: %v", id, err)
		}
		if it.Enabled || it.AgentAccess == "" || it.MCP == nil {
			t.Errorf("%s must be opt-in with a described agent access and an MCP launcher: %+v", id, it)
		}
	}
	gh, _ := m.GetConnector("github")
	if gh.MCP.Command != "docker" || !strings.Contains(strings.Join(gh.MCP.Args, " "), "ghcr.io/github/github-mcp-server") {
		t.Fatalf("without the native server GitHub runs GitHub's official image: %+v", gh.MCP)
	}
	if gh.MCP.Env["GITHUB_READ_ONLY"] != "1" {
		t.Error("the GitHub MCP server is read-only")
	}
	if gr, _ := m.GetConnector("grafana"); gr.MCP.Command != "docker" || !strings.Contains(strings.Join(gr.MCP.Args, " "), "mcp/grafana -t stdio") {
		t.Errorf("without mcp-grafana, Grafana runs in Docker: %+v", gr.MCP)
	}
	isolate(t)
	if az := newTestManager(t, nil); az != nil {
		if gr, _ := az.GetConnector("grafana"); gr.MCP.Command != "mcp-grafana" {
			t.Errorf("with nothing installed the first launcher is shown, so the operator knows what to install: %+v", gr.MCP)
		}
	}
}

func TestLegacyGitHubServerMigratesButCustomCommandsStay(t *testing.T) {
	isolate(t, "github-mcp-server", "docker")
	m := newTestManager(t, &types.ConnectorsConfig{Items: []*types.ConnectorItem{
		{ID: "github", Name: "GitHub Issues & PRs", Enabled: true, ConfigMode: "hybrid", Status: "configured",
			MCP: &types.MCPConfig{Enabled: true, Command: "npx", Args: []string{"-y", "@modelcontextprotocol/server-github"}, Transport: "stdio",
				Env: map[string]string{"GITHUB_PERSONAL_ACCESS_TOKEN": "ghp_legacytoken1234"}}},
		{ID: "grafana", Name: "Grafana", Enabled: true, ConfigMode: "mcp",
			MCP: &types.MCPConfig{Enabled: true, Command: "/opt/custom/grafana-mcp", Args: []string{"--debug"}, Transport: "stdio"}},
	}})
	gh, _ := m.GetConnector("github")
	if gh.MCP.Command != "github-mcp-server" || gh.MCP.Args[0] != "stdio" || !strings.Contains(strings.Join(gh.MCP.Args, " "), "--read-only") {
		t.Fatalf("legacy server should move to the native official one: %+v", gh.MCP)
	}
	if gh.MCP.Env["GITHUB_PERSONAL_ACCESS_TOKEN"] != "ghp_legacytoken1234" {
		t.Fatal("the token survives the migration")
	}
	if gr, _ := m.GetConnector("grafana"); gr.MCP.Command != "/opt/custom/grafana-mcp" {
		t.Fatalf("a command the operator wrote is never replaced: %+v", gr.MCP)
	}
}

func TestAgentServersOnlyIncludeReadyConnectors(t *testing.T) {
	isolate(t, "mcp-grafana", "npx")
	m := newTestManager(t, nil)
	mustUpdate := func(id string, f func(*types.ConnectorItem)) {
		it, _ := m.GetConnector(id)
		cp := *it
		f(&cp)
		if _, err := m.UpdateConnector(cp); err != nil {
			t.Fatal(err)
		}
	}
	mustUpdate("grafana", func(it *types.ConnectorItem) {
		it.Enabled, it.Status, it.BaseURL, it.APIToken = true, "configured", "https://grafana.example.com/", "glsa_secret_token_9876"
	})
	mustUpdate("cloudflare", func(it *types.ConnectorItem) { it.Enabled, it.Status = true, "configured" }) // no token yet
	mustUpdate("azure", func(it *types.ConnectorItem) { it.Enabled, it.Status, it.TargetEntity = true, "connected", "sub-123" })

	servers, list := m.AgentServers()
	if _, ok := servers["cloudflare"]; ok {
		t.Fatal("a connector without its credential would fail every call: leave it out")
	}
	gr := servers["grafana"].(map[string]interface{})
	env := gr["env"].(map[string]string)
	if gr["command"] != "mcp-grafana" || env["GRAFANA_URL"] != "https://grafana.example.com" || env["GRAFANA_SERVICE_ACCOUNT_TOKEN"] != "glsa_secret_token_9876" {
		t.Fatalf("grafana server = %+v", gr)
	}
	if az := servers["azure"].(map[string]interface{}); az["env"].(map[string]string)["AZURE_SUBSCRIPTION_ID"] != "sub-123" {
		t.Fatalf("azure server = %+v", az)
	}
	names := []string{}
	for _, c := range list {
		if _, stack := lookupStack(c.ID); stack {
			names = append(names, c.ID)
		}
	}
	if strings.Join(names, ",") != "azure,grafana" {
		t.Fatalf("agent connectors = %v", names)
	}

	// The token placeholder in Cloudflare's arguments is filled at run time only.
	mustUpdate("cloudflare", func(it *types.ConnectorItem) { it.APIToken = "cf_token_abcdef123456" })
	servers, _ = m.AgentServers()
	args := strings.Join(servers["cloudflare"].(map[string]interface{})["args"].([]string), " ")
	if !strings.Contains(args, "Authorization: Bearer cf_token_abcdef123456") {
		t.Fatalf("cloudflare args = %s", args)
	}
	if it, _ := m.GetConnector("cloudflare"); strings.Contains(strings.Join(it.MCP.Args, " "), "cf_token") {
		t.Fatal("the saved arguments keep the placeholder, never the token")
	}

	path, _, cleanup, err := m.WriteAgentMCPConfig("TASK-1/turn 2")
	if err != nil || path == "" {
		t.Fatalf("write = %q %v", path, err)
	}
	st, _ := os.Stat(path)
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("per-turn config mode = %v, want 0600", st.Mode().Perm())
	}
	if dir, _ := os.Stat(filepath.Dir(path)); dir.Mode().Perm() != 0o700 {
		t.Fatalf("per-turn config folder mode = %v, want 0700", dir.Mode().Perm())
	}
	cleanup()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("the per-turn config is deleted when the turn ends")
	}
}

func TestBrowsersOnlySeeMaskedSecrets(t *testing.T) {
	isolate(t)
	m := newTestManager(t, nil)
	it, _ := m.GetConnector("github")
	cp := *it
	cp.APIToken = "ghp_realsecret_ABCD"
	cp.MCP = &types.MCPConfig{Enabled: true, Command: "github-mcp-server", Args: []string{"stdio"}, Transport: "stdio",
		Env: map[string]string{"GITHUB_PERSONAL_ACCESS_TOKEN": "ghp_realsecret_ABCD", "GITHUB_TOOLSETS": "repos"}}
	if _, err := m.UpdateConnector(cp); err != nil {
		t.Fatal(err)
	}
	saved, _ := m.GetConnector("github")
	shown := Masked(saved)
	if shown.APIToken != "••••ABCD" || shown.MCP.Env["GITHUB_PERSONAL_ACCESS_TOKEN"] != "••••ABCD" || shown.MCP.Env["GITHUB_TOOLSETS"] != "repos" {
		t.Fatalf("masked = %+v %+v", shown.APIToken, shown.MCP.Env)
	}
	if saved.APIToken != "ghp_realsecret_ABCD" {
		t.Fatal("masking never changes the stored connector")
	}
	// The browser saves the form back with the masked values: the real secrets stay.
	if _, err := m.UpdateConnector(*shown); err != nil {
		t.Fatal(err)
	}
	after, _ := m.GetConnector("github")
	if after.APIToken != "ghp_realsecret_ABCD" || after.MCP.Env["GITHUB_PERSONAL_ACCESS_TOKEN"] != "ghp_realsecret_ABCD" {
		t.Fatalf("masked round trip lost the secret: %q %q", after.APIToken, after.MCP.Env["GITHUB_PERSONAL_ACCESS_TOKEN"])
	}
	if cfg := m.MaskedConfig(); strings.Contains(mustJSON(cfg), "ghp_realsecret") {
		t.Fatal("the masked config leaks a token")
	}
	if st, _ := os.Stat(m.configPath); st.Mode().Perm() != 0o600 {
		t.Fatalf("connectors.json mode = %v, want 0600", st.Mode().Perm())
	}
}

func TestMCPExportForBrowsersIsMasked(t *testing.T) {
	isolate(t, "npx")
	m := newTestManager(t, nil)
	cf, _ := m.GetConnector("cloudflare")
	cp := *cf
	cp.Enabled, cp.Status, cp.APIToken = true, "configured", "cf_token_abcdef123456"
	if _, err := m.UpdateConnector(cp); err != nil {
		t.Fatal(err)
	}
	sl, _ := m.GetConnector("slack")
	cp = *sl
	cp.Enabled = true
	cp.MCP = &types.MCPConfig{Enabled: true, Command: "npx", Args: []string{"-y", "@modelcontextprotocol/server-slack"}, Transport: "stdio",
		Env: map[string]string{"SLACK_BOT_TOKEN": "xoxb-slack-secret-7890", "SLACK_TEAM_ID": "T123"}}
	if _, err := m.UpdateConnector(cp); err != nil {
		t.Fatal(err)
	}

	shown := mustJSON(m.MaskedMCPExportConfig())
	for _, secret := range []string{"cf_token_abcdef123456", "xoxb-slack-secret-7890"} {
		if strings.Contains(shown, secret) {
			t.Fatalf("the browser export leaks %q: %s", secret[:6], shown)
		}
	}
	if !strings.Contains(shown, "T123") || !strings.Contains(shown, "Authorization: Bearer ••••3456") {
		t.Fatalf("only credentials are masked: %s", shown)
	}

	// The internal export (skills, .mcp.json) keeps live values, on copies of the saved settings.
	live := m.GetMCPExportConfig()["mcpServers"].(map[string]interface{})
	env := live["slack"].(map[string]interface{})["env"].(map[string]string)
	if env["SLACK_BOT_TOKEN"] != "xoxb-slack-secret-7890" {
		t.Fatalf("live export = %+v", env)
	}
	env["SLACK_BOT_TOKEN"] = "changed"
	if saved, _ := m.GetConnector("slack"); saved.MCP.Env["SLACK_BOT_TOKEN"] != "xoxb-slack-secret-7890" {
		t.Fatal("the export must not alias the saved connector")
	}
}

func TestMaskEnvCoversCommonCredentialNames(t *testing.T) {
	env := map[string]string{"ASANA_PAT": "1/12345:abcdefgh", "OPENAPI_MCP_HEADERS": `{"Authorization":"Bearer ntn_abcdefgh"}`,
		"SLACK_WEBHOOK_URL": "https://hooks.slack.com/services/T/B/xyzxyzxyz", "PATH": "/usr/bin", "SLACK_TEAM_ID": "T1"}
	got := MaskEnv(env)
	for _, k := range []string{"ASANA_PAT", "OPENAPI_MCP_HEADERS", "SLACK_WEBHOOK_URL"} {
		if !masked(got[k]) {
			t.Errorf("%s is shown in clear", k)
		}
	}
	if got["PATH"] != "/usr/bin" || got["SLACK_TEAM_ID"] != "T1" {
		t.Errorf("settings stay readable: %+v", got)
	}
	if env["ASANA_PAT"] != "1/12345:abcdefgh" {
		t.Error("MaskEnv must not change its input")
	}
	msg := RedactSecrets("server exited: auth failed for 1/12345:abcdefgh", env)
	if strings.Contains(msg, "abcdefgh") {
		t.Fatalf("redacted = %q", msg)
	}
}

func TestCredentialFilesArePrivateAndReplacedWhole(t *testing.T) {
	isolate(t)
	m := newTestManager(t, &types.ConnectorsConfig{}) // connectors.json starts out 0644
	if _, err := m.ToggleConnector("slack", true); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{m.configPath, filepath.Join(m.rootDir, ".mcp.json"), filepath.Join(m.rootDir, ".sdlc", "mcp.json")} {
		st, err := os.Stat(p)
		if err != nil || st.Mode().Perm() != 0o600 {
			t.Errorf("%s: mode %v err %v, want 0600", filepath.Base(p), st.Mode().Perm(), err)
		}
	}
	entries, _ := os.ReadDir(filepath.Dir(m.configPath))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".connectors.json-") || strings.HasPrefix(e.Name(), ".mcp.json-") {
			t.Errorf("temporary file left behind: %s", e.Name())
		}
	}
	var saved types.ConnectorsConfig
	if b, _ := os.ReadFile(m.configPath); json.Unmarshal(b, &saved) != nil || len(saved.Items) == 0 {
		t.Fatal("connectors.json is not a complete config")
	}
}

// Handlers read connectors while toggles and status probes change them: readers get copies.
func TestConnectorReadsDoNotRaceWithUpdates(t *testing.T) {
	isolate(t)
	m := newTestManager(t, nil)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 30; i++ {
			_, _ = m.ToggleConnector("grafana", i%2 == 0)
			m.updateItemStatus("grafana", i%2 == 0, int64(i), "probe")
		}
	}()
	for i := 0; i < 30; i++ {
		if it, err := m.GetConnector("grafana"); err == nil {
			_ = mustJSON(Masked(it))
		}
		_ = mustJSON(MaskedList(m.ListConnectors()))
		_ = mustJSON(m.MaskedConfig())
	}
	<-done
	it, _ := m.GetConnector("grafana")
	it.MCP.Env["X"] = "changed"
	if again, _ := m.GetConnector("grafana"); again.MCP.Env["X"] == "changed" {
		t.Fatal("GetConnector must return a copy")
	}
}

func mustJSON(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// fakeMCPServer answers initialize and tools/list like a real stdio MCP server.
const fakeMCPServer = `read init
echo 'starting fake server' >&2
echo '{"jsonrpc":"2.0","method":"notifications/message","params":{}}'
echo '{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2025-03-26","serverInfo":{"name":"fake-grafana","version":"0.4.2"},"capabilities":{}}}'
read initialized
read list
echo '{"jsonrpc":"2.0","id":2,"result":{"tools":[{"name":"search_dashboards"},{"name":"query_prometheus"},{"name":"list_alert_rules"}]}}'
sleep 5`

func TestProbeMCPPerformsTheHandshake(t *testing.T) {
	res, err := ProbeMCP(context.Background(), types.MCPConfig{Command: "sh", Args: []string{"-c", fakeMCPServer}, Transport: "stdio"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Server != "fake-grafana" || res.Version != "0.4.2" || res.Tools != 3 || res.ToolNames[1] != "query_prometheus" {
		t.Fatalf("probe = %+v", res)
	}

	_, err = ProbeMCP(context.Background(), types.MCPConfig{Command: "sh", Args: []string{"-c", "echo 'GRAFANA_URL is required' >&2; exit 1"}})
	if err == nil || !strings.Contains(err.Error(), "GRAFANA_URL is required") {
		t.Fatalf("a crashing server explains why: %v", err)
	}
	_, err = ProbeMCP(context.Background(), types.MCPConfig{Command: "sh", Args: []string{"-c", `read x; echo '{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"bad token"}}'; sleep 5`}})
	if err == nil || !strings.Contains(err.Error(), "bad token") {
		t.Fatalf("an error reply is reported: %v", err)
	}
	if _, err := ProbeMCP(context.Background(), types.MCPConfig{Command: "definitely-not-installed-xyz"}); err == nil {
		t.Fatal("a missing command fails fast")
	}
}

func TestGrafanaCredentialCheck(t *testing.T) {
	isolate(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/search" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer glsa_good_token_1234" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()
	m := newTestManager(t, nil)

	ok := m.TestGenericConnector(context.Background(), "grafana", nil) // ping: unconfigured
	if ok.Success || !strings.Contains(ok.Message, "URL is required") {
		t.Fatalf("missing URL: %+v", ok)
	}
	it, _ := m.GetConnector("grafana")
	cp := *it
	cp.Enabled, cp.BaseURL, cp.APIToken = true, srv.URL, "glsa_wrong_token_0000"
	_, _ = m.UpdateConnector(cp)
	if res := m.TestGenericConnector(context.Background(), "grafana", nil); res.Success || !strings.Contains(res.Message, "rejected the token") {
		t.Fatalf("wrong token: %+v", res)
	}
	cp.APIToken = "glsa_good_token_1234"
	_, _ = m.UpdateConnector(cp)
	if res := m.TestGenericConnector(context.Background(), "grafana", nil); !res.Success {
		t.Fatalf("good token: %+v", res)
	}
	if got, _ := m.GetConnector("grafana"); got.Status != "connected" {
		t.Fatalf("status after a good check = %s", got.Status)
	}
}

func TestProbeStopsServersStartedByALauncher(t *testing.T) {
	// Like npx: the launcher starts the real server as a child that inherits its pipes.
	launcher := `sh -c 'read init; echo "{\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"serverInfo\":{\"name\":\"child\"}}}"; read a; read b; echo "{\"jsonrpc\":\"2.0\",\"id\":2,\"result\":{\"tools\":[]}}"; sleep 30' & wait`
	done := make(chan error, 1)
	go func() {
		_, err := ProbeMCP(context.Background(), types.MCPConfig{Command: "sh", Args: []string{"-c", launcher}})
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the probe must not wait for the server's child to exit")
	}
}

func TestRetiredGitHubLaunchersMoveToTheCurrentToolsets(t *testing.T) {
	isolate(t, "github-mcp-server")
	m := newTestManager(t, &types.ConnectorsConfig{Items: []*types.ConnectorItem{
		{ID: "github", Name: "GitHub Issues & PRs", Enabled: true, ConfigMode: "hybrid", Status: "configured",
			MCP: &types.MCPConfig{Enabled: true, Command: "github-mcp-server", Args: []string{"stdio", "--read-only", "--toolsets", githubToolsetsV1},
				Transport: "stdio", Env: map[string]string{"GITHUB_PERSONAL_ACCESS_TOKEN": "ghp_v1token1234"}}},
	}})
	gh, _ := m.GetConnector("github")
	args := strings.Join(gh.MCP.Args, " ")
	if !strings.Contains(args, "--read-only") || !strings.Contains(args, githubToolsets) {
		t.Fatalf("the first launcher should move to the current toolsets: %+v", gh.MCP)
	}
	for _, ts := range []string{"actions", "pull_requests", "context", "code_security"} {
		if !strings.Contains(githubToolsets, ts) {
			t.Errorf("toolset %s missing", ts)
		}
	}
	if gh.MCP.Env["GITHUB_PERSONAL_ACCESS_TOKEN"] != "ghp_v1token1234" {
		t.Fatal("the token survives the migration")
	}
}

func TestGitHubEnterpriseHost(t *testing.T) {
	for _, c := range []struct{ base, host, api string }{
		{"", "", "https://api.github.com"},
		{"https://api.github.com", "", "https://api.github.com"},
		{"https://github.acme.co.id/api/v3", "https://github.acme.co.id", "https://github.acme.co.id/api/v3"},
		{"https://api.acme.ghe.com", "https://acme.ghe.com", "https://api.acme.ghe.com"},
		{"http://127.0.0.1:8080", "http://127.0.0.1:8080", "http://127.0.0.1:8080/api/v3"},
	} {
		if got := githubHost(c.base); got != c.host {
			t.Errorf("githubHost(%q) = %q, want %q", c.base, got, c.host)
		}
		if got := githubAPIBase(c.base); got != c.api {
			t.Errorf("githubAPIBase(%q) = %q, want %q", c.base, got, c.api)
		}
	}

	isolate(t, "github-mcp-server")
	m := newTestManager(t, nil)
	it, _ := m.GetConnector("github")
	cp := *it
	cp.APIToken, cp.BaseURL = "ghp_enterprise_1234", "https://github.acme.co.id/api/v3"
	m.mu.RLock()
	mcp, ok := m.runtimeMCPLocked(&cp)
	m.mu.RUnlock()
	if !ok || mcp.Env["GITHUB_HOST"] != "https://github.acme.co.id" {
		t.Fatalf("an Enterprise server gets GITHUB_HOST: ok=%v env=%v", ok, mcp.Env["GITHUB_HOST"])
	}
}

func TestGitHubActionsAccessIsChecked(t *testing.T) {
	isolate(t)
	status := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/repos/acme/app/actions/runs" || r.Header.Get("Authorization") != "Bearer ghp_actions_1234" {
			w.WriteHeader(http.StatusTeapot)
			return
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"total_count":0,"workflow_runs":[]}`))
	}))
	defer srv.Close()
	m := newTestManager(t, nil)
	it := &types.ConnectorItem{ID: "github", BaseURL: srv.URL, TargetEntity: "acme/app", APIToken: "ghp_actions_1234"}
	if p := m.githubActionsProblem(context.Background(), it); p != "" {
		t.Fatalf("readable runs are no problem: %s", p)
	}
	status = http.StatusForbidden
	if p := m.githubActionsProblem(context.Background(), it); !strings.Contains(p, "Actions: read") {
		t.Fatalf("a token without Actions access is explained: %q", p)
	}
}
