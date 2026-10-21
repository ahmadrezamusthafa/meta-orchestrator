package connectors

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

func grafanaWith(t *testing.T, m *Manager, f func(*types.ConnectorItem)) *types.ConnectorItem {
	t.Helper()
	it, _ := m.GetConnector("grafana")
	cp := *it
	cp.ExtraSettings = map[string]interface{}{}
	f(&cp)
	if _, err := m.UpdateConnector(cp); err != nil {
		t.Fatal(err)
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.items["grafana"]
}

func TestGrafanaArgsAreNeverNull(t *testing.T) {
	isolate(t, "mcp-grafana")
	m := newTestManager(t, nil)
	it, _ := m.GetConnector("grafana")
	raw, _ := json.Marshal(it.MCP)
	if strings.Contains(string(raw), `"args":null`) {
		t.Fatalf("the MCP tab joins args: they must be a list, got %s", raw)
	}
	if got := strings.Join(it.MCP.Args, " "); got != "--disable-write --disable-admin" {
		t.Errorf("args = %q, want the read-only flags", got)
	}
}

func TestGrafanaRuntimeByAuthMode(t *testing.T) {
	isolate(t, "mcp-grafana")
	m := newTestManager(t, nil)
	run := func(it *types.ConnectorItem) (types.MCPConfig, bool) {
		m.mu.RLock()
		defer m.mu.RUnlock()
		return m.runtimeMCPLocked(it)
	}

	it := grafanaWith(t, m, func(it *types.ConnectorItem) {
		it.Enabled, it.BaseURL, it.Username, it.APIToken = true, "127.0.0.1:3000", "admin", "admin-pass"
		it.ExtraSettings["auth_mode"], it.ExtraSettings["org_id"], it.ExtraSettings["tls_skip_verify"] = "basic", float64(2), true
	})
	mcp, ok := run(it)
	if !ok || mcp.Env["GRAFANA_URL"] != "http://127.0.0.1:3000" || mcp.Env["GRAFANA_USERNAME"] != "admin" || mcp.Env["GRAFANA_PASSWORD"] != "admin-pass" ||
		mcp.Env["GRAFANA_ORG_ID"] != "2" || mcp.Env["GRAFANA_SERVICE_ACCOUNT_TOKEN"] != "" {
		t.Fatalf("basic auth server = %v %+v", ok, mcp.Env)
	}
	if args := strings.Join(mcp.Args, " "); args != "--disable-write --disable-admin --tls-skip-verify" {
		t.Errorf("an admin login still runs read-only: args = %q", args)
	}

	it = grafanaWith(t, m, func(it *types.ConnectorItem) {
		it.Enabled, it.BaseURL, it.Username, it.APIToken = true, "http://localhost:3000", "", "-"
		it.ExtraSettings["auth_mode"] = "anonymous"
	})
	if mcp, ok = run(it); !ok || len(mcp.Env) != 1 || mcp.Env["GRAFANA_URL"] != "http://localhost:3000" {
		t.Fatalf("anonymous server = %v %+v", ok, mcp.Env)
	}

	it = grafanaWith(t, m, func(it *types.ConnectorItem) {
		it.Enabled, it.BaseURL, it.APIToken = true, "grafana.example.com", ""
		it.ExtraSettings["auth_mode"] = "token"
	})
	it.APIToken = ""
	if _, ok = run(it); ok {
		t.Fatal("token mode without a token cannot run")
	}
	t.Setenv("GRAFANA_SERVICE_ACCOUNT_TOKEN", "glsa_from_env_1234")
	if mcp, ok = run(it); !ok || mcp.Env["GRAFANA_URL"] != "https://grafana.example.com" || mcp.Env["GRAFANA_SERVICE_ACCOUNT_TOKEN"] != "glsa_from_env_1234" {
		t.Fatalf("token server = %v %+v", ok, mcp.Env)
	}
}

func TestGrafanaDockerReachesTheHostsLoopback(t *testing.T) {
	if got := grafanaURLFor("http://127.0.0.1:3000", true); got != "http://host.docker.internal:3000" {
		t.Errorf("docker url = %q", got)
	}
	if got := grafanaURLFor("http://127.0.0.1:3000", false); got != "http://127.0.0.1:3000" {
		t.Errorf("native url = %q", got)
	}
	if got := grafanaURLFor("https://grafana.example.com", true); got != "https://grafana.example.com" {
		t.Errorf("remote url = %q", got)
	}
}

func TestGrafanaSavedLaunchersMoveToReadOnly(t *testing.T) {
	isolate(t, "mcp-grafana")
	m := newTestManager(t, &types.ConnectorsConfig{Items: []*types.ConnectorItem{{ID: "grafana", Name: "Grafana", ConfigMode: "mcp",
		MCP: &types.MCPConfig{Enabled: true, Command: "mcp-grafana", Transport: "stdio"}}}})
	if it, _ := m.GetConnector("grafana"); !containsArg(it.MCP.Args, "--disable-write") {
		t.Fatalf("an earlier built-in launcher is upgraded: %+v", it.MCP)
	}
}

func TestCheckGrafana(t *testing.T) {
	isolate(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, basic := r.BasicAuth()
		authed := (basic && user == "admin" && pass == "admin-pass") || r.Header.Get("Authorization") == "Bearer glsa_ok_token_123"
		if !authed {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path == "/api/user" {
			_, _ = w.Write([]byte(`{"login":"admin","isGrafanaAdmin":true}`))
			return
		}
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()
	m := newTestManager(t, nil)
	check := func(f func(*types.ConnectorItem)) (string, string) {
		it := grafanaWith(t, m, func(it *types.ConnectorItem) { it.BaseURL = srv.URL; f(it) })
		who, _, problem := m.checkGrafana(context.Background(), it)
		return who, problem
	}

	if who, p := check(func(it *types.ConnectorItem) {
		it.ExtraSettings["auth_mode"], it.Username, it.APIToken = "basic", "admin", "admin-pass"
	}); p != "" || !strings.Contains(who, "Grafana admin") {
		t.Errorf("basic: who=%q problem=%q", who, p)
	}
	if _, p := check(func(it *types.ConnectorItem) {
		it.ExtraSettings["auth_mode"], it.Username, it.APIToken = "basic", "admin", "wrong-pass"
	}); !strings.Contains(p, "username or password") {
		t.Errorf("wrong password: %q", p)
	}
	if who, p := check(func(it *types.ConnectorItem) {
		it.ExtraSettings["auth_mode"], it.Username, it.APIToken = "token", "", "glsa_ok_token_123"
	}); p != "" || who != "service account" {
		t.Errorf("token: who=%q problem=%q", who, p)
	}
	if _, p := check(func(it *types.ConnectorItem) { it.ExtraSettings["auth_mode"] = "anonymous" }); !strings.Contains(p, "auth.anonymous") {
		t.Errorf("anonymous refused: %q", p)
	}
}
