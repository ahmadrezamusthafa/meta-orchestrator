package connectors

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

func TestProjectConnectorsAreOptInAndReadOnly(t *testing.T) {
	isolate(t, "npx")
	m := newTestManager(t, nil)
	readOnly := map[string]func(types.MCPConfig) bool{
		"clickup": func(c types.MCPConfig) bool { return c.Env["CLICKUP_MCP_MODE"] == "read" },
		"asana":   func(c types.MCPConfig) bool { return c.Env["READ_ONLY_MODE"] == "true" },
		"monday":  func(c types.MCPConfig) bool { return strings.Contains(strings.Join(c.Args, " "), "--read-only true") },
		"trello":  func(c types.MCPConfig) bool { return true }, // read-only through the token's scope
	}
	for id, isReadOnly := range readOnly {
		it, err := m.GetConnector(id)
		if err != nil {
			t.Fatalf("%s missing: %v", id, err)
		}
		if it.Enabled || it.Category != types.ConnectorCategoryProjectManagement || it.AgentAccess == "" || it.MCP == nil {
			t.Errorf("%s must be an opt-in project-management connector with a described access: %+v", id, it)
			continue
		}
		if !isReadOnly(*it.MCP) {
			t.Errorf("%s server must start read-only: %+v", id, it.MCP)
		}
	}
}

func TestUnpublishedLinearAndNotionServersMigrate(t *testing.T) {
	isolate(t, "npx")
	m := newTestManager(t, &types.ConnectorsConfig{Items: []*types.ConnectorItem{
		{ID: "notion", Name: "Notion Workspace", Enabled: true, ConfigMode: "hybrid", Status: "configured",
			MCP: &types.MCPConfig{Enabled: true, Command: "npx", Args: []string{"-y", "@modelcontextprotocol/server-notion"}, Transport: "stdio",
				Env: map[string]string{"NOTION_API_KEY": "ntn_legacy_token_5678"}}},
		{ID: "linear", Name: "Linear App", Enabled: true, ConfigMode: "hybrid", Status: "configured", APIToken: "lin_api_secret_4321",
			MCP: &types.MCPConfig{Enabled: true, Command: "npx", Args: []string{"-y", "@modelcontextprotocol/server-linear"}, Transport: "stdio"}},
	}})
	no, _ := m.GetConnector("notion")
	if !strings.Contains(strings.Join(no.MCP.Args, " "), "@notionhq/notion-mcp-server") || no.MCP.Env["NOTION_TOKEN"] != "ntn_legacy_token_5678" {
		t.Fatalf("notion should move to the official server and keep its token: %+v", no.MCP)
	}
	li, _ := m.GetConnector("linear")
	if !strings.Contains(strings.Join(li.MCP.Args, " "), "@tacticlaunch/mcp-linear") {
		t.Fatalf("linear should move to a published MCP server: %+v", li.MCP)
	}
	servers, _ := m.AgentServers()
	if env := servers["linear"].(map[string]interface{})["env"].(map[string]string); env["LINEAR_API_TOKEN"] != "lin_api_secret_4321" {
		t.Fatalf("linear env = %v", env)
	}
	if env := servers["notion"].(map[string]interface{})["env"].(map[string]string); env["NOTION_TOKEN"] != "ntn_legacy_token_5678" {
		t.Fatalf("notion env = %v", env)
	}
}

func TestProjectConnectorsNeedTheirSettingsToRun(t *testing.T) {
	isolate(t, "npx")
	m := newTestManager(t, nil)
	set := func(id string, f func(*types.ConnectorItem)) {
		it, _ := m.GetConnector(id)
		cp := *it
		cp.Enabled, cp.Status = true, "configured"
		f(&cp)
		if _, err := m.UpdateConnector(cp); err != nil {
			t.Fatal(err)
		}
	}
	set("clickup", func(it *types.ConnectorItem) { it.APIToken = "pk_clickup_secret_1111" })  // no workspace yet
	set("trello", func(it *types.ConnectorItem) { it.APIToken = "trello_token_secret_2222" }) // no API key yet
	set("monday", func(it *types.ConnectorItem) { it.APIToken = "monday_secret_token_3333" })
	set("asana", func(it *types.ConnectorItem) { it.APIToken = "asana_secret_token_4444" })

	servers, _ := m.AgentServers()
	for _, id := range []string{"clickup", "trello"} {
		if _, ok := servers[id]; ok {
			t.Errorf("%s without its required setting would fail every call: leave it out", id)
		}
	}
	if env := servers["monday"].(map[string]interface{})["env"].(map[string]string); env["MONDAY_TOKEN"] != "monday_secret_token_3333" {
		t.Fatalf("monday env = %v", env)
	}
	if env := servers["asana"].(map[string]interface{})["env"].(map[string]string); env["ASANA_ACCESS_TOKEN"] != "asana_secret_token_4444" || env["READ_ONLY_MODE"] != "true" {
		t.Fatalf("asana env = %v", env)
	}

	set("clickup", func(it *types.ConnectorItem) { it.TargetEntity = "9012345" })
	set("trello", func(it *types.ConnectorItem) { it.Username, it.TargetEntity = "trellokey123", "AbCd1234" })
	servers, _ = m.AgentServers()
	cu := servers["clickup"].(map[string]interface{})["env"].(map[string]string)
	if cu["CLICKUP_API_KEY"] != "pk_clickup_secret_1111" || cu["CLICKUP_TEAM_ID"] != "9012345" || cu["CLICKUP_MCP_MODE"] != "read" {
		t.Fatalf("clickup env = %v", cu)
	}
	tr := servers["trello"].(map[string]interface{})["env"].(map[string]string)
	if tr["TRELLO_API_KEY"] != "trellokey123" || tr["TRELLO_TOKEN"] != "trello_token_secret_2222" || tr["TRELLO_BOARD_ID"] != "AbCd1234" {
		t.Fatalf("trello env = %v", tr)
	}
	for _, id := range []string{"clickup", "trello", "monday", "asana"} {
		it, _ := m.GetConnector(id)
		if shown := mustJSON(Masked(it)); strings.Contains(shown, "secret") {
			t.Errorf("%s leaks its token to the browser: %s", id, shown)
		}
	}
}

// fakeAPIs answers the "who am I" calls of the project-management services, whatever their host.
func fakeAPIs(t *testing.T, m *Manager) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		switch r.Host + r.URL.Path {
		case "api.clickup.com/api/v2/user":
			if auth != "pk_good" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(`{"user":{"username":"dev"}}`))
		case "api.clickup.com/api/v2/team":
			_, _ = w.Write([]byte(`{"teams":[{"id":"9012345","name":"Acme"}]}`))
		case "api.trello.com/1/members/me":
			if !strings.Contains(auth, `oauth_token="trello_good"`) || r.URL.Query().Get("token") != "" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(`{"username":"dev"}`))
		case "api.trello.com/1/tokens/trello_good":
			_, _ = w.Write([]byte(`{"permissions":[{"read":true,"write":false}]}`))
		case "app.asana.com/api/1.0/users/me":
			_, _ = w.Write([]byte(`{"data":{"name":"Dev","workspaces":[{"gid":"111","name":"Acme"}]}}`))
		case "api.monday.com/v2":
			if auth != "monday_good" {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"errors":[{"message":"Not authenticated"}]}`))
				return
			}
			_, _ = w.Write([]byte(`{"data":{"me":{"name":"Dev"},"account":{"name":"Acme"}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	target, _ := url.Parse(srv.URL)
	m.client = &http.Client{Transport: rewriteHost{target: target}}
}

type rewriteHost struct{ target *url.URL }

func (h rewriteHost) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Host = r.URL.Host
	r.URL.Scheme, r.URL.Host = h.target.Scheme, h.target.Host
	return http.DefaultTransport.RoundTrip(r)
}

func TestProjectCredentialChecks(t *testing.T) {
	isolate(t)
	m := newTestManager(t, nil)
	fakeAPIs(t, m)
	check := func(id string, f func(*types.ConnectorItem)) types.TestConnectorResponse {
		it, _ := m.GetConnector(id)
		cp := *it
		cp.Enabled = true
		f(&cp)
		_, _ = m.UpdateConnector(cp)
		return m.TestGenericConnector(context.Background(), id, nil)
	}

	if res := check("clickup", func(it *types.ConnectorItem) { it.APIToken = "pk_bad" }); res.Success || !strings.Contains(res.Message, "rejected the token") {
		t.Fatalf("clickup bad token: %+v", res)
	}
	if res := check("clickup", func(it *types.ConnectorItem) { it.APIToken = "pk_good" }); res.Success || !strings.Contains(res.Message, "9012345 (Acme)") {
		t.Fatalf("clickup without a workspace lists the ones it reaches: %+v", res)
	}
	if res := check("clickup", func(it *types.ConnectorItem) { it.TargetEntity = "9012345" }); !res.Success || res.ConnectedAs != "dev" {
		t.Fatalf("clickup good: %+v", res)
	}

	if res := check("trello", func(it *types.ConnectorItem) { it.APIToken = "trello_good" }); res.Success || !strings.Contains(res.Message, "API key is required") {
		t.Fatalf("trello without a key: %+v", res)
	}
	if res := check("trello", func(it *types.ConnectorItem) { it.Username = "key123" }); !res.Success || !strings.Contains(res.ServerInfo+res.Message, "read-only token") {
		t.Fatalf("trello good: %+v", res)
	}

	if res := check("asana", func(it *types.ConnectorItem) { it.APIToken, it.TargetEntity = "asana_good", "Other" }); res.Success || !strings.Contains(res.Message, "It reaches: Acme") {
		t.Fatalf("asana unknown workspace: %+v", res)
	}
	if res := check("asana", func(it *types.ConnectorItem) { it.TargetEntity = "111" }); !res.Success || res.ConnectedAs != "Dev" {
		t.Fatalf("asana good: %+v", res)
	}

	if res := check("monday", func(it *types.ConnectorItem) { it.APIToken = "monday_bad" }); res.Success || !strings.Contains(res.Message, "rejected the token") {
		t.Fatalf("monday bad token: %+v", res)
	}
	if res := check("monday", func(it *types.ConnectorItem) { it.APIToken = "monday_good" }); !res.Success {
		t.Fatalf("monday good: %+v", res)
	}
	if got, _ := m.GetConnector("monday"); got.Status != "connected" {
		t.Fatalf("status after a good check = %s", got.Status)
	}
}
