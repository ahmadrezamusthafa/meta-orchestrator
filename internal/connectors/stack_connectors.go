package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// stackConnector is a connector the supported tech stacks need, defined as data: its catalog
// entry, the ways to start its MCP server (the first whose command is installed wins), and where
// its credential comes from.
type stackConnector struct {
	item types.ConnectorItem
	// launchers are tried in order; the docker form goes last because Docker must be running.
	launchers []types.MCPConfig
	// tokenKey is the MCP env variable the credential is passed in; tokenEnv the shell variables
	// it may come from.
	tokenKey string
	tokenEnv []string
	// legacyArgs marks older built-in launchers that are migrated to the current ones.
	legacyArgs []string
	// restCheck means the catalog's REST test checks the credential (testGenericConnector); the
	// stack test only adds the MCP probe.
	restCheck bool
}

func dockerLauncher(image string, env []string, args ...string) types.MCPConfig {
	a := []string{"run", "-i", "--rm"}
	for _, e := range env {
		a = append(a, "-e", e) // the value comes from the docker CLI's own environment
	}
	a = append(append(a, image), args...)
	return types.MCPConfig{Enabled: true, Command: "docker", Args: a, Transport: "stdio"}
}

const githubToolsets = "repos,issues,pull_requests,actions"

// stackConnectors are the connectors added for the supported stacks, in catalog order. GitHub
// already has a catalog entry; it is listed here for its official MCP server.
var stackConnectors = []stackConnector{
	{
		item: types.ConnectorItem{ID: "github", AgentAccess: "Read-only: repositories, issues, pull requests and GitHub Actions runs and logs"},
		launchers: []types.MCPConfig{
			{Enabled: true, Command: "github-mcp-server", Args: []string{"stdio", "--read-only", "--toolsets", githubToolsets}, Transport: "stdio"},
			withEnv(dockerLauncher("ghcr.io/github/github-mcp-server", []string{"GITHUB_PERSONAL_ACCESS_TOKEN", "GITHUB_TOOLSETS", "GITHUB_READ_ONLY"}),
				map[string]string{"GITHUB_TOOLSETS": githubToolsets, "GITHUB_READ_ONLY": "1"}),
		},
		tokenKey:   "GITHUB_PERSONAL_ACCESS_TOKEN",
		tokenEnv:   []string{"GITHUB_TOKEN", "GH_TOKEN", "GITHUB_PERSONAL_ACCESS_TOKEN"},
		legacyArgs: []string{"@modelcontextprotocol/server-github"},
		restCheck:  true,
	},
	{
		item: types.ConnectorItem{
			ID: "grafana", Name: "Grafana", Category: types.ConnectorCategoryObservability, CategoryLabel: "Observability",
			Description: "Lets agents check dashboards, alerts and the Prometheus, Loki and Tempo data behind them — e.g. that new OpenTelemetry spans and metrics really arrive. Use a service account with the Viewer role.",
			Icon:        "grafana", Color: "orange", ConfigMode: "mcp", Status: "unconfigured",
			TargetLabel:  "Default dashboard folder (optional)",
			Capabilities: []string{"Dashboard Search", "PromQL / LogQL Queries", "Alert Rules", "Trace Lookup"},
			AgentAccess:  "Read-only when the service account has the Viewer role: dashboards, alerts, Prometheus, Loki and Tempo queries",
		},
		launchers: []types.MCPConfig{
			{Enabled: true, Command: "mcp-grafana", Args: []string{}, Transport: "stdio"},
			dockerLauncher("mcp/grafana", []string{"GRAFANA_URL", "GRAFANA_SERVICE_ACCOUNT_TOKEN"}, "-t", "stdio"),
		},
		tokenKey: "GRAFANA_SERVICE_ACCOUNT_TOKEN",
		tokenEnv: []string{"GRAFANA_SERVICE_ACCOUNT_TOKEN", "GRAFANA_API_KEY", "GRAFANA_TOKEN"},
	},
	{
		item: types.ConnectorItem{
			ID: "azure", Name: "Microsoft Azure", Category: types.ConnectorCategoryCloud, CategoryLabel: "Cloud Platforms",
			Description: "Lets agents read live Azure state — Container Apps, revisions, logs and resource settings — to compare it with the IaC. Read-only; signs in with your Azure CLI login (az login).",
			Icon:        "azure", Color: "sky", ConfigMode: "mcp", Status: "unconfigured",
			TargetLabel:  "Default subscription (optional)",
			Capabilities: []string{"Container Apps State", "Resource Inspection", "Monitor Logs"},
			AgentAccess:  "Read-only (--read-only): Azure resources and Container Apps; changes go through IaC",
		},
		launchers: []types.MCPConfig{
			{Enabled: true, Command: "npx", Args: []string{"-y", "@azure/mcp@latest", "server", "start", "--read-only"}, Transport: "stdio"},
		},
	},
	{
		item: types.ConnectorItem{
			ID: "cloudflare", Name: "Cloudflare", Category: types.ConnectorCategoryCloud, CategoryLabel: "Cloud Platforms",
			Description: "Lets agents read Workers logs and analytics through Cloudflare's Observability MCP server. Use an API token with read-only permissions.",
			Icon:        "cloudflare", Color: "orange", ConfigMode: "mcp", Status: "unconfigured",
			BaseURL:      "https://api.cloudflare.com/client/v4",
			TargetLabel:  "Default account ID (optional)",
			Capabilities: []string{"Workers Logs", "Analytics", "Token Verification"},
			AgentAccess:  "Read-only API token: Workers logs and analytics; DNS, WAF and Workers changes go through IaC",
		},
		launchers: []types.MCPConfig{
			{Enabled: true, Command: "npx", Args: []string{"-y", "mcp-remote", "https://observability.mcp.cloudflare.com/mcp",
				"--header", "Authorization: Bearer ${CLOUDFLARE_API_TOKEN}"}, Transport: "stdio"},
		},
		tokenKey: "CLOUDFLARE_API_TOKEN",
		tokenEnv: []string{"CLOUDFLARE_API_TOKEN", "CF_API_TOKEN"},
	},

	// ── Project management: agents read the ticket, its comments and its board ──
	{
		item: types.ConnectorItem{
			ID: "clickup", Name: "ClickUp", Category: types.ConnectorCategoryProjectManagement, CategoryLabel: "Project Management",
			Description: "Lets agents read ClickUp tasks with their comments, checklists, lists and docs — the acceptance criteria behind a change. Runs the server in read mode.",
			Icon:        "clickup", Color: "pink", ConfigMode: "mcp", Status: "unconfigured",
			BaseURL:      "https://api.clickup.com/api/v2",
			TargetLabel:  "Workspace (team) ID",
			Capabilities: []string{"Task Details & Comments", "Task Search", "Lists & Spaces", "Docs"},
			AgentAccess:  "Read-only (CLICKUP_MCP_MODE=read): tasks, comments, lists, spaces and docs of the workspace",
		},
		launchers: []types.MCPConfig{
			withEnv(types.MCPConfig{Enabled: true, Command: "npx", Args: []string{"-y", "@hauptsache.net/clickup-mcp"}, Transport: "stdio"},
				map[string]string{"CLICKUP_MCP_MODE": "read"}),
		},
		tokenKey: "CLICKUP_API_KEY",
		tokenEnv: []string{"CLICKUP_API_KEY", "CLICKUP_API_TOKEN", "CLICKUP_TOKEN"},
	},
	{
		item: types.ConnectorItem{
			ID: "trello", Name: "Trello", Category: types.ConnectorCategoryProjectManagement, CategoryLabel: "Project Management",
			Description: "Lets agents read Trello boards, lists, cards, checklists and comments. Authorize the token with read scope so it cannot change boards.",
			Icon:        "trello", Color: "blue", ConfigMode: "mcp", Status: "unconfigured",
			BaseURL:      "https://api.trello.com/1",
			TargetLabel:  "Default board ID (optional)",
			Capabilities: []string{"Boards & Lists", "Card Details", "Checklists", "Comments"},
			AgentAccess:  "Read-only when the token is authorized with scope=read: boards, lists, cards, checklists and comments",
		},
		launchers: []types.MCPConfig{
			{Enabled: true, Command: "npx", Args: []string{"-y", "@delorenj/mcp-server-trello"}, Transport: "stdio"},
		},
		tokenKey: "TRELLO_TOKEN",
		tokenEnv: []string{"TRELLO_TOKEN", "TRELLO_API_TOKEN"},
	},
	{
		item: types.ConnectorItem{
			ID: "asana", Name: "Asana", Category: types.ConnectorCategoryProjectManagement, CategoryLabel: "Project Management",
			Description: "Lets agents read Asana tasks, projects, sections, comments and status updates. Runs the server in read-only mode.",
			Icon:        "asana", Color: "rose", ConfigMode: "mcp", Status: "unconfigured",
			BaseURL:      "https://app.asana.com/api/1.0",
			TargetLabel:  "Default workspace (optional)",
			Capabilities: []string{"Task Details & Stories", "Task Search", "Projects & Sections", "Status Updates"},
			AgentAccess:  "Read-only (READ_ONLY_MODE): tasks, projects, sections, comments and status updates",
		},
		launchers: []types.MCPConfig{
			withEnv(types.MCPConfig{Enabled: true, Command: "npx", Args: []string{"-y", "@roychri/mcp-server-asana"}, Transport: "stdio"},
				map[string]string{"READ_ONLY_MODE": "true"}),
		},
		tokenKey: "ASANA_ACCESS_TOKEN",
		tokenEnv: []string{"ASANA_ACCESS_TOKEN", "ASANA_TOKEN", "ASANA_PAT"},
	},
	{
		item: types.ConnectorItem{
			ID: "monday", Name: "monday.com", Category: types.ConnectorCategoryProjectManagement, CategoryLabel: "Project Management",
			Description: "Lets agents read monday.com boards, items, updates and columns through monday.com's official MCP server, in read-only mode.",
			Icon:        "monday", Color: "red", ConfigMode: "mcp", Status: "unconfigured",
			BaseURL:      "https://api.monday.com/v2",
			TargetLabel:  "Default board ID (optional)",
			Capabilities: []string{"Boards & Items", "Item Updates", "Columns & Groups", "Users & Teams"},
			AgentAccess:  "Read-only (--read-only): boards, items, updates, columns and groups",
		},
		launchers: []types.MCPConfig{
			{Enabled: true, Command: "npx", Args: []string{"-y", "@mondaydotcomorg/monday-api-mcp@latest", "--read-only", "true"}, Transport: "stdio"}, // the flag takes a value
		},
		tokenKey: "MONDAY_TOKEN", // the server reads MONDAY_<ARG>, not the monday_token its README names
		tokenEnv: []string{"MONDAY_API_TOKEN", "MONDAY_TOKEN"},
	},
	// Linear and Notion already have catalog entries; these move them to working MCP servers (the
	// @modelcontextprotocol packages they named were never published).
	{
		item: types.ConnectorItem{ID: "linear", AgentAccess: "Read and write as the API key allows (create it with the Read permission only for read-only): issues, projects, cycles and comments"},
		// A stdio server with the API key: Linear's hosted server falls back to a browser sign-in
		// when the key is rejected, which an unattended agent turn cannot complete.
		launchers: []types.MCPConfig{
			{Enabled: true, Command: "npx", Args: []string{"-y", "@tacticlaunch/mcp-linear"}, Transport: "stdio"},
		},
		tokenKey:   "LINEAR_API_TOKEN",
		tokenEnv:   []string{"LINEAR_API_KEY", "LINEAR_API_TOKEN", "LINEAR_TOKEN"},
		legacyArgs: []string{"@modelcontextprotocol/server-linear"},
		restCheck:  true,
	},
	{
		item: types.ConnectorItem{ID: "notion", AgentAccess: "Only the pages and databases shared with the integration; read-only when the integration has only the Read content capability"},
		launchers: []types.MCPConfig{
			{Enabled: true, Command: "npx", Args: []string{"-y", "@notionhq/notion-mcp-server"}, Transport: "stdio"},
		},
		tokenKey:   "NOTION_TOKEN",
		tokenEnv:   []string{"NOTION_API_KEY", "NOTION_TOKEN", "NOTION_INTEGRATION_TOKEN"},
		legacyArgs: []string{"@modelcontextprotocol/server-notion"},
		restCheck:  true,
	},
}

func withEnv(m types.MCPConfig, env map[string]string) types.MCPConfig {
	m.Env = env
	return m
}

// IsStackConnector reports whether id is one of the connectors defined for the supported stacks,
// whose test also starts the MCP server.
func IsStackConnector(id string) bool {
	_, ok := lookupStack(id)
	return ok
}

func lookupStack(id string) (stackConnector, bool) {
	for _, s := range stackConnectors {
		if s.item.ID == id {
			return s, true
		}
	}
	return stackConnector{}, false
}

// lookPath is exec.LookPath; tests replace it.
var lookPath = exec.LookPath

// launcherFor picks the first launcher whose command is installed, else the first one (so the
// operator sees what to install).
func (s stackConnector) launcherFor() types.MCPConfig {
	for _, l := range s.launchers {
		if _, err := lookPath(l.Command); err == nil {
			return cloneMCP(l)
		}
	}
	return cloneMCP(s.launchers[0])
}

// isBuiltinLauncher reports whether mcp is one of our launchers (or a legacy one) rather than a
// command the operator wrote, so it may be re-resolved when tools change.
func (s stackConnector) isBuiltinLauncher(mcp *types.MCPConfig) bool {
	if mcp == nil {
		return true
	}
	joined := mcp.Command + " " + strings.Join(mcp.Args, " ")
	for _, legacy := range s.legacyArgs {
		if strings.Contains(joined, legacy) {
			return true
		}
	}
	for _, l := range s.launchers {
		if l.Command == mcp.Command && strings.Join(l.Args, " ") == strings.Join(mcp.Args, " ") {
			return true
		}
	}
	return false
}

func cloneMCP(m types.MCPConfig) types.MCPConfig {
	m.Args = append([]string(nil), m.Args...)
	env := map[string]string{}
	for k, v := range m.Env {
		env[k] = v
	}
	m.Env = env
	return m
}

// ensureStackConnectorsLocked adds the stack connectors to the catalog and moves built-in
// launchers to the best one installed — never touching a command the operator customised.
func (m *Manager) ensureStackConnectorsLocked() {
	for _, s := range stackConnectors {
		it, exists := m.items[s.item.ID]
		if !exists {
			item := s.item
			item.Enabled = false // a cloud or observability connection is always opted into
			mcp := s.launcherFor()
			item.MCP = &mcp
			m.items[item.ID] = &item
			it = &item
		}
		it.AgentAccess = s.item.AgentAccess
		if s.isBuiltinLauncher(it.MCP) {
			enabled := it.MCP == nil || it.MCP.Enabled
			var oldEnv map[string]string
			if it.MCP != nil {
				oldEnv = it.MCP.Env
			}
			mcp := s.launcherFor()
			mcp.Enabled = enabled
			if tok := carriedToken(s.tokenKey, oldEnv); tok != "" {
				mcp.Env[s.tokenKey] = tok
			}
			it.MCP = &mcp
		}
	}
}

// carriedToken is the credential an older launcher held: under tokenKey, or for a migrated
// server under the secret variable it used instead (e.g. NOTION_API_KEY → NOTION_TOKEN).
func carriedToken(tokenKey string, oldEnv map[string]string) string {
	if tokenKey == "" || oldEnv == nil {
		return ""
	}
	if v := strings.TrimSpace(oldEnv[tokenKey]); v != "" {
		return v
	}
	keys := make([]string, 0, len(oldEnv))
	for k := range oldEnv {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if v := strings.TrimSpace(oldEnv[k]); v != "" && secretKey(k) {
			return v
		}
	}
	return ""
}

// stackTokenLocked resolves a stack connector's credential: the saved token, its MCP env, then
// the shell environment (including .env files).
func (m *Manager) stackTokenLocked(s stackConnector, it *types.ConnectorItem) (string, string) {
	if it != nil && it.APIToken != "" && !masked(it.APIToken) {
		return it.APIToken, "saved_config"
	}
	if it != nil && it.MCP != nil && s.tokenKey != "" {
		if v := strings.TrimSpace(it.MCP.Env[s.tokenKey]); v != "" && !masked(v) {
			return v, "mcp_env"
		}
	}
	if v, src := resolveEnvValue(s.tokenEnv...); v != "" {
		return v, src
	}
	return "", ""
}

var envRef = regexp.MustCompile(`\$\{([A-Z0-9_]+)\}`)

// runtimeMCPLocked is the MCP server an agent turn starts for the connector, with its credential
// and settings filled in. ok is false when the connector cannot run (e.g. no credential yet).
func (m *Manager) runtimeMCPLocked(it *types.ConnectorItem) (types.MCPConfig, bool) {
	if it.MCP == nil || strings.TrimSpace(it.MCP.Command) == "" {
		return types.MCPConfig{}, false
	}
	mcp := cloneMCP(*it.MCP)
	if s, ok := lookupStack(it.ID); ok {
		if s.isBuiltinLauncher(it.MCP) {
			fresh := s.launcherFor() // a tool installed since: prefer it over docker
			for k, v := range mcp.Env {
				if _, set := fresh.Env[k]; !set {
					fresh.Env[k] = v
				}
			}
			fresh.Enabled = mcp.Enabled
			mcp = fresh
		}
		if s.tokenKey != "" {
			tok, _ := m.stackTokenLocked(s, it)
			if tok == "" {
				return types.MCPConfig{}, false
			}
			mcp.Env[s.tokenKey] = tok
		}
		if it.ID == "grafana" {
			if strings.TrimSpace(it.BaseURL) == "" {
				return types.MCPConfig{}, false
			}
			mcp.Env["GRAFANA_URL"] = strings.TrimRight(it.BaseURL, "/")
		}
		if it.ID == "azure" && it.TargetEntity != "" {
			mcp.Env["AZURE_SUBSCRIPTION_ID"] = it.TargetEntity
		}
		if !projectSettings(it, mcp.Env) {
			return types.MCPConfig{}, false
		}
	} else {
		// Catalog connectors keep credentials in their MCP env; fill a missing one from the item.
		for k, v := range mcp.Env {
			if strings.TrimSpace(v) == "" || masked(v) {
				if tok, _ := m.resolveTokenLocked(it.ID, ""); tok != "" && secretKey(k) {
					mcp.Env[k] = tok
				}
			}
		}
	}
	for k, v := range mcp.Env {
		if secretKey(k) && (strings.TrimSpace(v) == "" || masked(v)) {
			return types.MCPConfig{}, false // the server would start but fail every call
		}
	}
	for i, a := range mcp.Args {
		mcp.Args[i] = envRef.ReplaceAllStringFunc(a, func(ref string) string {
			name := envRef.FindStringSubmatch(ref)[1]
			if v, ok := mcp.Env[name]; ok {
				return v
			}
			return os.Getenv(name)
		})
	}
	return mcp, true
}

// AgentServers returns the MCP servers of the enabled, connected connectors for an agent turn,
// with their credentials, and a description of each for the agent's brief.
func (m *Manager) AgentServers() (map[string]interface{}, []types.AgentConnector) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	servers := map[string]interface{}{}
	var list []types.AgentConnector
	ids := make([]string, 0, len(m.items))
	for id := range m.items {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		it := m.items[id]
		if !it.Enabled || it.MCP == nil || !(it.MCP.Enabled || it.ConfigMode == "mcp") {
			continue
		}
		if it.Status != "connected" && it.Status != "configured" {
			continue
		}
		mcp, ok := m.runtimeMCPLocked(it)
		if !ok {
			continue
		}
		entry := map[string]interface{}{"command": mcp.Command, "args": mcp.Args}
		if len(mcp.Env) > 0 {
			entry["env"] = mcp.Env
		}
		servers[id] = entry
		list = append(list, types.AgentConnector{ID: id, Name: it.Name, Access: it.AgentAccess})
	}
	return servers, list
}

// WriteAgentMCPConfig writes the agent servers to a private file (0600 in a 0700 folder) for
// `claude --mcp-config`, and returns its path and a cleanup that deletes it. It returns an empty
// path when no connector is ready.
func (m *Manager) WriteAgentMCPConfig(name string) (string, []types.AgentConnector, func(), error) {
	servers, list := m.AgentServers()
	noop := func() {}
	if len(servers) == 0 {
		return "", nil, noop, nil
	}
	dir, err := agentMCPDir()
	if err != nil {
		return "", nil, noop, err
	}
	data, err := json.MarshalIndent(map[string]interface{}{"mcpServers": servers}, "", "  ")
	if err != nil {
		return "", nil, noop, err
	}
	path := filepath.Join(dir, safeFileName(name)+".json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", nil, noop, err
	}
	return path, list, func() { _ = os.Remove(path) }, nil
}

var unsafeName = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func safeFileName(s string) string {
	s = unsafeName.ReplaceAllString(s, "_")
	if s == "" {
		s = "turn"
	}
	return fmt.Sprintf("%s-%d", s, time.Now().UnixNano())
}

// agentMCPDir is a private folder for per-turn MCP configs; leftovers from a crash are removed
// once they are a day old.
func agentMCPDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil || base == "" {
		base = os.TempDir()
	}
	dir := filepath.Join(base, "meta-orchestrator", "agent-mcp")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	_ = os.Chmod(dir, 0o700)
	if entries, err := os.ReadDir(dir); err == nil {
		for _, e := range entries {
			if info, err := e.Info(); err == nil && time.Since(info.ModTime()) > 24*time.Hour {
				_ = os.Remove(filepath.Join(dir, e.Name()))
			}
		}
	}
	return dir, nil
}

// testStackConnector checks a stack connector's credential with its API (cheap, used by the
// periodic ping) and, when deep, starts its MCP server and lists its tools.
func (m *Manager) testStackConnector(ctx context.Context, it *types.ConnectorItem, deep bool) types.TestConnectorResponse {
	start := time.Now()
	s, _ := lookupStack(it.ID)
	m.mu.RLock()
	token, _ := m.stackTokenLocked(s, it)
	m.mu.RUnlock()
	fail := func(msg string) types.TestConnectorResponse {
		lat := time.Since(start).Milliseconds()
		m.updateItemStatus(it.ID, false, lat, msg)
		return types.TestConnectorResponse{Success: false, LatencyMs: lat, Message: msg}
	}

	var who, info string
	switch it.ID {
	case "grafana":
		base := strings.TrimRight(strings.TrimSpace(it.BaseURL), "/")
		if base == "" {
			return fail("Grafana URL is required (e.g. https://grafana.example.com)")
		}
		if token == "" {
			return fail("A Grafana service account token is required (create one with the Viewer role; or set GRAFANA_SERVICE_ACCOUNT_TOKEN)")
		}
		code, body, err := m.getJSON(ctx, base+"/api/search?limit=1", token)
		switch {
		case err != nil:
			return fail("Could not reach Grafana: " + err.Error())
		case code == http.StatusUnauthorized || code == http.StatusForbidden:
			return fail(fmt.Sprintf("Grafana rejected the token (HTTP %d) — check it is a valid service account token", code))
		case code != http.StatusOK:
			return fail(fmt.Sprintf("Grafana answered HTTP %d: %s", code, firstLine(body)))
		}
		who, info = "service account", base
	case "cloudflare":
		if token == "" {
			return fail("A Cloudflare API token is required (read-only permissions; or set CLOUDFLARE_API_TOKEN)")
		}
		code, body, err := m.getJSON(ctx, "https://api.cloudflare.com/client/v4/user/tokens/verify", token)
		if err != nil {
			return fail("Could not reach Cloudflare: " + err.Error())
		}
		var v struct {
			Success bool `json:"success"`
			Result  struct {
				Status string `json:"status"`
			} `json:"result"`
		}
		_ = json.Unmarshal([]byte(body), &v)
		if code != http.StatusOK || !v.Success || v.Result.Status != "active" {
			return fail(fmt.Sprintf("Cloudflare did not accept the token (HTTP %d, status %q)", code, v.Result.Status))
		}
		who, info = "API token", "token is active"
	case "azure":
		az, err := lookPath("az")
		if err != nil {
			return fail("The Azure CLI (az) is not installed — install it from Tool Hub → Stack toolchain, then run `az login`")
		}
		cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		out, err := exec.CommandContext(cctx, az, "account", "show", "--output", "json").Output()
		cancel()
		if err != nil {
			return fail("The Azure CLI is not signed in — run `az login` in your terminal, then test again")
		}
		var acct struct {
			Name string `json:"name"`
			User struct {
				Name string `json:"name"`
			} `json:"user"`
		}
		_ = json.Unmarshal(out, &acct)
		who, info = acct.User.Name, "subscription "+acct.Name
	case "clickup", "trello", "asana", "monday":
		var problem string
		if who, info, problem = m.checkProjectCredential(ctx, it, token); problem != "" {
			return fail(problem)
		}
	default:
		// GitHub, Linear, Notion: the REST check stays in TestGenericConnector; only the MCP
		// server is probed here.
		who, info = it.Username, strings.TrimPrefix(strings.TrimPrefix(it.BaseURL, "https://"), "http://")
	}

	msg := fmt.Sprintf("%s credentials work (%s).", it.Name, info)
	if deep {
		m.mu.RLock()
		mcp, ok := m.runtimeMCPLocked(it)
		m.mu.RUnlock()
		if !ok {
			return fail(it.Name + " credentials work, but its MCP server is missing a setting or credential")
		}
		probe, err := ProbeMCP(ctx, mcp)
		if err != nil {
			hint := ""
			if mcp.Command == "docker" {
				hint = " Is Docker running? Installing the native server from Tool Hub avoids Docker."
			}
			// The server's stderr may echo its arguments or environment: never pass a credential on.
			return fail(RedactSecrets(fmt.Sprintf("%s credentials work, but its MCP server did not start: %v.%s", it.Name, err, hint), mcp.Env))
		}
		msg = fmt.Sprintf("%s is ready for agents: %s started with %d tools (%s).", it.Name, orName(probe.Server, mcp.Command), probe.Tools,
			strings.Join(probe.ToolNames, ", "))
		info = fmt.Sprintf("%s %s · %s", orName(probe.Server, mcp.Command), probe.Version, it.AgentAccess)
	}
	lat := time.Since(start).Milliseconds()
	m.updateItemStatus(it.ID, true, lat, "")
	return types.TestConnectorResponse{Success: true, LatencyMs: lat, Message: msg, ConnectedAs: who, ServerInfo: info, TargetEntity: it.TargetEntity}
}

func orName(name, fallback string) string {
	if name != "" {
		return name
	}
	return fallback
}

func (m *Manager) getJSON(ctx context.Context, url, token string) (int, string, error) {
	return m.doJSON(ctx, http.MethodGet, url, map[string]string{"Authorization": "Bearer " + token}, "")
}

// doJSON sends a request with the given headers (and body, when set) and returns the status and
// up to 64 KiB of the answer.
func (m *Manager) doJSON(ctx context.Context, method, url string, headers map[string]string, body string) (int, string, error) {
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, rd)
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("Accept", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	var b strings.Builder
	buf := make([]byte, 4096)
	for b.Len() < 64<<10 {
		n, err := resp.Body.Read(buf)
		b.Write(buf[:n])
		if err != nil {
			break
		}
	}
	return resp.StatusCode, b.String(), nil
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	return s
}

// ── Secrets in API responses ──

const maskPrefix = "••••"

func masked(v string) bool { return strings.Contains(v, maskPrefix) }

// secretKey reports whether an env variable holds a credential.
func secretKey(k string) bool {
	u := strings.ToUpper(k)
	for _, w := range []string{"TOKEN", "PASSWORD", "SECRET", "API_KEY", "APIKEY", "PRIVATE_KEY"} {
		if strings.Contains(u, w) {
			return true
		}
	}
	return false
}

// maskKey reports whether a browser sees the variable's value masked: every credential secretKey
// knows, and variables that commonly carry one (personal access tokens, auth headers, webhook
// URLs, connection strings). It is wider than secretKey, which also gates starting a server.
func maskKey(k string) bool {
	if secretKey(k) {
		return true
	}
	u := strings.ToUpper(k)
	if u == "PAT" || strings.HasSuffix(u, "_PAT") {
		return true
	}
	for _, w := range []string{"AUTH", "CREDENTIAL", "WEBHOOK", "HEADER", "COOKIE", "DSN", "CONNECTION_STRING", "PASSWD"} {
		if strings.Contains(u, w) {
			return true
		}
	}
	return false
}

// MaskEnv returns a copy of env with every credential masked, for browsers.
func MaskEnv(env map[string]string) map[string]string {
	if env == nil {
		return nil
	}
	out := make(map[string]string, len(env))
	for k, v := range env {
		if maskKey(k) {
			v = mask(v)
		}
		out[k] = v
	}
	return out
}

// RedactSecrets replaces every credential value of env that appears in s (an argument, a
// server's stderr) by its mask.
func RedactSecrets(s string, env map[string]string) string {
	var secrets []string
	for k, v := range env {
		if v = strings.TrimSpace(v); maskKey(k) && len(v) >= 4 && !masked(v) {
			secrets = append(secrets, v)
		}
	}
	sort.Slice(secrets, func(i, j int) bool { return len(secrets[i]) > len(secrets[j]) })
	for _, v := range secrets {
		s = strings.ReplaceAll(s, v, mask(v))
	}
	return s
}

// RedactArgs is RedactSecrets for each argument.
func RedactArgs(args []string, env map[string]string) []string {
	if args == nil {
		return nil
	}
	out := make([]string, len(args))
	for i, a := range args {
		out[i] = RedactSecrets(a, env)
	}
	return out
}

// mask shows only that a secret is set and its last four characters.
func mask(v string) string {
	if v == "" || masked(v) {
		return v
	}
	if len(v) <= 8 {
		return maskPrefix
	}
	return maskPrefix + v[len(v)-4:]
}

// Masked returns a copy of the connector that is safe to send to a browser: credentials are
// replaced by ••••last4. Saving a masked value back keeps the stored secret (see UpdateConnector).
func Masked(it *types.ConnectorItem) *types.ConnectorItem {
	if it == nil {
		return nil
	}
	cp := *it
	cp.APIToken = mask(it.APIToken)
	if it.MCP != nil {
		mcp := cloneMCP(*it.MCP)
		mcp.Env = MaskEnv(mcp.Env)
		cp.MCP = &mcp
	}
	return &cp
}

// MaskedList masks every connector in list.
func MaskedList(list []*types.ConnectorItem) []*types.ConnectorItem {
	out := make([]*types.ConnectorItem, 0, len(list))
	for _, it := range list {
		out = append(out, Masked(it))
	}
	return out
}

// keepMaskedEnv restores secrets the browser sent back masked.
func keepMaskedEnv(incoming, existing *types.MCPConfig) {
	if incoming == nil || existing == nil {
		return
	}
	for k, v := range incoming.Env {
		if masked(v) {
			if old, ok := existing.Env[k]; ok {
				incoming.Env[k] = old
			} else {
				delete(incoming.Env, k)
			}
		}
	}
}
