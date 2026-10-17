package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/stack"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/toolchain"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// ConnectorSuggestion is a connector the scope's repositories would benefit from.
type ConnectorSuggestion struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Reasons []string `json:"reasons"`
	Status  string   `json:"status"`
	Enabled bool     `json:"enabled"`
	// Ready means agents get it in their turns (enabled, connected or configured, credentials set).
	Ready  bool   `json:"ready"`
	Access string `json:"access,omitempty"`
}

// connectorRules map what a repository uses to the connector that serves it.
var connectorRules = []struct {
	connector string
	match     func(stack.Profile) string // the reason, or "" when it does not apply
}{
	{"github", func(p stack.Profile) string {
		switch {
		case hasHost(p, "github.com"):
			return "hosted on GitHub (pull requests, CI runs)"
		case p.Has("github-actions"):
			return "uses GitHub Actions (CI runs and logs)"
		}
		return ""
	}},
	{"gitlab", func(p stack.Profile) string { return when(hasHost(p, "gitlab"), "hosted on GitLab") }},
	{"bitbucket", func(p stack.Profile) string {
		return when(hasHost(p, "bitbucket.org"), "hosted on Bitbucket (pull requests, pipelines)")
	}},
	{"grafana", func(p stack.Profile) string {
		switch {
		case p.Has("grafana"):
			return "ships Grafana dashboards (check them against live data)"
		case p.Has("opentelemetry"):
			return "instrumented with OpenTelemetry (verify spans and metrics arrive)"
		}
		return ""
	}},
	{"azure", func(p stack.Profile) string {
		return when(p.Has("azure-container-apps"), "deploys to Azure Container Apps (compare live state with the IaC)")
	}},
	{"cloudflare", func(p stack.Profile) string {
		return when(p.Has("cloudflare"), "uses Cloudflare (Workers logs and analytics)")
	}},
	{"clickup", tracker("clickup", "links ClickUp tasks (read the acceptance criteria and comments)")},
	{"trello", tracker("trello", "links Trello cards (read the card, checklists and comments)")},
	{"asana", tracker("asana", "links Asana tasks (read the task and its comments)")},
	{"monday", tracker("monday", "links monday.com boards (read the item and its updates)")},
	{"linear", tracker("linear", "links Linear issues (read the issue and its comments)")},
	{"notion", tracker("notion", "links Notion pages (read the specs they hold)")},
}

// tracker matches a repository that links the project-management tool id (stack.Profile.Trackers).
func tracker(id, reason string) func(stack.Profile) string {
	return func(p stack.Profile) string {
		for _, t := range p.Trackers {
			if t == id {
				return reason
			}
		}
		return ""
	}
}

// npxConnectors start their MCP server through npx, so they need Node.js.
var npxConnectors = map[string]bool{"azure": true, "cloudflare": true, "clickup": true, "trello": true, "asana": true,
	"monday": true, "linear": true, "notion": true}

func hasHost(p stack.Profile, host string) bool {
	for _, h := range p.Hosts {
		if strings.Contains(h, host) {
			return true
		}
	}
	return false
}

func when(ok bool, reason string) string {
	if ok {
		return reason
	}
	return ""
}

// connectorSuggestions lists the connectors the named repositories would use, with whether
// agents already get them.
func (r *Router) connectorSuggestions(repos []string) []ConnectorSuggestion {
	mgr := r.cfg.ConnectorsManager
	if mgr == nil {
		return []ConnectorSuggestion{}
	}
	ready := map[string]types.AgentConnector{}
	_, agentList := mgr.AgentServers()
	for _, c := range agentList {
		ready[c.ID] = c
	}
	out := []ConnectorSuggestion{}
	index := map[string]int{}
	for _, repo := range r.repoStacks(repos) {
		for _, rule := range connectorRules {
			why := rule.match(repo.Profile)
			if why == "" {
				continue
			}
			reason := repo.Name + ": " + why
			if i, ok := index[rule.connector]; ok {
				out[i].Reasons = append(out[i].Reasons, reason)
				continue
			}
			item, err := mgr.GetConnector(rule.connector)
			if err != nil {
				continue
			}
			_, isReady := ready[rule.connector]
			index[rule.connector] = len(out)
			out = append(out, ConnectorSuggestion{ID: item.ID, Name: item.Name, Reasons: []string{reason}, Status: item.Status,
				Enabled: item.Enabled, Ready: isReady, Access: item.AgentAccess})
		}
	}
	return out
}

// handleConnectorSuggestions: GET /api/v1/connectors/suggestions[?task=|?project=].
func (r *Router) handleConnectorSuggestions(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		r.writeError(w, http.StatusMethodNotAllowed, "GET required")
		return
	}
	repos, err := r.scopeRepos(req)
	if err != nil {
		r.writeError(w, http.StatusNotFound, err.Error())
		return
	}
	r.writeJSON(w, http.StatusOK, r.connectorSuggestions(repos))
}

// agentMCPConfig writes the connected connectors' MCP servers for one agent turn. The file is
// private to this user and deleted by cleanup when the turn ends.
func (r *Router) agentMCPConfig(taskID, turnID string) (string, func()) {
	if r.cfg.ConnectorsManager == nil {
		return "", func() {}
	}
	path, _, cleanup, err := r.cfg.ConnectorsManager.WriteAgentMCPConfig(taskID + "-" + turnID)
	if err != nil {
		r.addEntry(taskID, types.ConsoleEntry{Kind: types.ConsoleKindSystem, TurnID: turnID,
			Content: "⚠ Connectors are not available this turn: " + err.Error()})
		return "", func() {}
	}
	return path, cleanup
}

// connectorsBrief tells the agent which connected services it can query this turn, and which
// ones its repositories would use but the operator has not connected.
func (r *Router) connectorsBrief(t *types.Task) string {
	if r.cfg.ConnectorsManager == nil {
		return ""
	}
	_, connected := r.cfg.ConnectorsManager.AgentServers()
	var missing []string
	for _, s := range r.connectorSuggestions(t.AssignedRepos) {
		if !s.Ready {
			missing = append(missing, s.Name)
		}
	}
	if len(connected) == 0 && len(missing) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n## Connected services\n")
	if len(connected) > 0 {
		b.WriteString("These services are available as MCP tools in this session — use them to read live state (pull requests, CI runs, " +
			"tickets, dashboards, cloud resources) instead of guessing:\n")
		for _, c := range connected {
			fmt.Fprintf(&b, "- %s (`mcp__%s__*`)", c.Name, c.ID)
			if c.Access != "" {
				b.WriteString(": " + c.Access)
			}
			b.WriteString("\n")
		}
		b.WriteString("Never change shared environments through them; tool calls that write still ask the operator.\n")
	}
	if len(missing) > 0 {
		fmt.Fprintf(&b, "Not connected, though these repositories use them: %s. If you need their live data, say so in your summary "+
			"so the operator can connect them on the Connectors page.\n", strings.Join(missing, ", "))
	}
	return b.String()
}

// connectorToolNeeds recommends the native MCP servers and CLIs of enabled connectors, so they
// run without Docker and can sign in.
func (r *Router) connectorToolNeeds() []toolchain.Requirement {
	if r.cfg.ConnectorsManager == nil {
		return nil
	}
	needs := map[string]toolchain.Requirement{
		"github":  {Tool: "github-mcp-server", Reasons: []string{"GitHub connector: runs GitHub's MCP server without Docker"}},
		"grafana": {Tool: "mcp-grafana", Reasons: []string{"Grafana connector: runs Grafana's MCP server without Docker"}},
		"azure":   {Tool: "azure-cli", Reasons: []string{"Azure connector: signs in with `az login`"}},
	}
	var out []toolchain.Requirement
	var npx []string
	for _, it := range r.cfg.ConnectorsManager.ListConnectors() {
		if !it.Enabled || (it.Status != "connected" && it.Status != "configured") {
			continue // only connectors the operator actually set up
		}
		if req, ok := needs[it.ID]; ok {
			out = append(out, req)
		}
		if npxConnectors[it.ID] {
			npx = append(npx, it.Name)
		}
	}
	if len(npx) > 0 {
		out = append(out, toolchain.Requirement{Tool: "node", Reasons: []string{"Started through npx: " + strings.Join(npx, ", ")}})
	}
	return out
}
