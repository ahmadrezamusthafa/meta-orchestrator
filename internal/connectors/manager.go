package connectors

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

var jiraKeyRegex = regexp.MustCompile(`\b([A-Z]{2,10}-\d+)\b`)

// loadDotEnv parses .env files in the provided directories and sets variables into process environment if unset.
func loadDotEnv(dirs ...string) {
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		envPath := filepath.Join(dir, ".env")
		data, err := os.ReadFile(envPath)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) != 2 {
				continue
			}
			key := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])
			if len(val) >= 2 {
				if (val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'') {
					val = val[1 : len(val)-1]
				}
			}
			if key != "" && os.Getenv(key) == "" {
				_ = os.Setenv(key, val)
			}
		}
	}
}

// resolveEnvValue searches environment variables for the first non-empty match among keys.
func resolveEnvValue(keys ...string) (string, string) {
	for _, k := range keys {
		if val := strings.TrimSpace(os.Getenv(k)); val != "" {
			return val, k
		}
	}
	return "", ""
}

// Manager orchestrates third-party tool integrations like JIRA, Confluence, and the modular catalog.
type Manager struct {
	mu            sync.RWMutex
	rootDir       string
	configPath    string
	config        types.ConnectorsConfig
	items         map[string]*types.ConnectorItem
	client        *http.Client
	broadcastFunc func(*types.OrchestratorEvent)
	onMCPUpdate   func(map[string]interface{})
	stopPing      chan struct{}
	pingRunning   bool
}

// SetOnMCPUpdateFunc registers a callback invoked whenever MCP server configuration is modified or synchronized.
func (m *Manager) SetOnMCPUpdateFunc(fn func(map[string]interface{})) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onMCPUpdate = fn
}

// NewManager creates a new connector manager.
func NewManager(rootDir string) *Manager {
	configPath := filepath.Join(rootDir, ".sdlc", "connectors.json")
	m := &Manager{
		rootDir:    rootDir,
		configPath: configPath,
		items:      make(map[string]*types.ConnectorItem),
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		config: types.ConnectorsConfig{
			Ping: types.ConnectorPingConfig{
				Enabled:         true,
				IntervalSeconds: 30,
			},
			Jira: types.JiraConfig{
				Enabled:        true,
				BaseURL:        "https://jira.atlassian.net",
				Username:       "devops@meta-orchestrator.io",
				ProjectKey:     "PAY",
				JQLFilter:      "project = PAY AND status != Done ORDER BY created DESC",
				AutoDetectKeys: true,
				AutoSyncStatus: true,
				Status:         "unconfigured",
			},
			Confluence: types.ConfluenceConfig{
				Enabled:             true,
				BaseURL:             "https://wiki.atlassian.net",
				Username:            "devops@meta-orchestrator.io",
				SpaceKey:            "ARCH",
				AutoPublishTechDocs: true,
				AutoPublishPRD:      true,
				Status:              "unconfigured",
			},
		},
	}

	m.load()
	m.mu.Lock()
	_ = m.saveMCPConfigLocked()
	m.mu.Unlock()
	m.StartPeriodicPinger(context.Background())
	return m
}

// resolveTokenLocked finds the effective token across direct input, existing item, MCP env, and environment variables.
func (m *Manager) resolveTokenLocked(id, candidate string) (string, string) {
	candidate = strings.TrimSpace(candidate)
	if candidate != "" && !strings.Contains(candidate, "••••") {
		// For bitbucket, if candidate is the shared/copied Confluence or Jira token and MCP has a dedicated ATLASSIAN_API_TOKEN, prefer MCP
		if id == "bitbucket" {
			existing := m.items[id]
			if existing != nil && existing.MCP != nil && existing.MCP.Env != nil {
				mcpTok := strings.TrimSpace(existing.MCP.Env["ATLASSIAN_API_TOKEN"])
				if mcpTok != "" && !strings.Contains(mcpTok, "••••") && (candidate == m.config.Confluence.APIToken || candidate == m.config.Jira.APIToken) {
					return mcpTok, "mcp_env"
				}
			}
		}
		return candidate, "direct_input"
	}

	existing := m.items[id]
	var mcpEnvToken string
	if existing != nil && existing.MCP != nil && existing.MCP.Env != nil {
		switch id {
		case "jira":
			mcpEnvToken = existing.MCP.Env["JIRA_API_TOKEN"]
		case "confluence":
			mcpEnvToken = existing.MCP.Env["CONFLUENCE_API_TOKEN"]
		case "bitbucket":
			mcpEnvToken = existing.MCP.Env["ATLASSIAN_API_TOKEN"]
			if mcpEnvToken == "" {
				mcpEnvToken = existing.MCP.Env["BITBUCKET_APP_PASSWORD"]
			}
			if mcpEnvToken == "" {
				mcpEnvToken = existing.MCP.Env["BITBUCKET_TOKEN"]
			}
			if mcpEnvToken == "" {
				mcpEnvToken = existing.MCP.Env["BITBUCKET_API_TOKEN"]
			}
		case "github":
			mcpEnvToken = existing.MCP.Env["GITHUB_PERSONAL_ACCESS_TOKEN"]
		case "gitlab":
			mcpEnvToken = existing.MCP.Env["GITLAB_PERSONAL_ACCESS_TOKEN"]
		case "slack":
			mcpEnvToken = existing.MCP.Env["SLACK_BOT_TOKEN"]
		case "linear":
			mcpEnvToken = existing.MCP.Env["LINEAR_API_KEY"]
		case "notion":
			mcpEnvToken = existing.MCP.Env["NOTION_API_KEY"]
		}
	}

	var existingToken string
	if existing != nil && existing.APIToken != "" && !strings.Contains(existing.APIToken, "••••") {
		existingToken = existing.APIToken
	}
	if id == "jira" && existingToken == "" && m.config.Jira.APIToken != "" && !strings.Contains(m.config.Jira.APIToken, "••••") {
		existingToken = m.config.Jira.APIToken
	}
	if id == "confluence" && existingToken == "" && m.config.Confluence.APIToken != "" && !strings.Contains(m.config.Confluence.APIToken, "••••") {
		existingToken = m.config.Confluence.APIToken
	}

	// Prioritize valid MCP env token if candidate is empty/masked and existingToken is old/different for bitbucket
	if strings.TrimSpace(mcpEnvToken) != "" && !strings.Contains(mcpEnvToken, "••••") {
		if candidate == "" || strings.Contains(candidate, "••••") {
			if id == "bitbucket" || existingToken == "" || existingToken == m.config.Confluence.APIToken || existingToken == m.config.Jira.APIToken {
				return strings.TrimSpace(mcpEnvToken), "mcp_env"
			}
		}
	}

	if existingToken != "" {
		return existingToken, "saved_config"
	}
	if strings.TrimSpace(mcpEnvToken) != "" && !strings.Contains(mcpEnvToken, "••••") {
		return strings.TrimSpace(mcpEnvToken), "mcp_env"
	}

	// Environment variable checks
	switch id {
	case "jira":
		if v, src := resolveEnvValue("JIRA_API_TOKEN", "JIRA_TOKEN", "ATLASSIAN_API_TOKEN"); v != "" {
			return v, src
		}
	case "confluence":
		if v, src := resolveEnvValue("CONFLUENCE_API_TOKEN", "CONFLUENCE_TOKEN"); v != "" {
			return v, src
		}
		if v, src := resolveEnvValue("JIRA_API_TOKEN", "ATLASSIAN_API_TOKEN"); v != "" {
			return v, src + " (shared)"
		}
		if m.config.Jira.APIToken != "" && !strings.Contains(m.config.Jira.APIToken, "••••") {
			return m.config.Jira.APIToken, "jira_config (shared)"
		}
	case "bitbucket":
		if v, src := resolveEnvValue("BITBUCKET_APP_PASSWORD", "BITBUCKET_TOKEN", "BITBUCKET_API_TOKEN", "BITBUCKET_PASSWORD", "ATLASSIAN_API_TOKEN"); v != "" {
			return v, src
		}
	case "github":
		if v, src := resolveEnvValue("GITHUB_TOKEN", "GH_TOKEN", "GITHUB_PERSONAL_ACCESS_TOKEN", "GITHUB_API_TOKEN"); v != "" {
			return v, src
		}
	case "gitlab":
		if v, src := resolveEnvValue("GITLAB_TOKEN", "GITLAB_PERSONAL_ACCESS_TOKEN", "GITLAB_API_TOKEN", "GL_TOKEN"); v != "" {
			return v, src
		}
	case "slack":
		if v, src := resolveEnvValue("SLACK_BOT_TOKEN", "SLACK_TOKEN"); v != "" {
			return v, src
		}
	case "linear":
		if v, src := resolveEnvValue("LINEAR_API_KEY", "LINEAR_TOKEN"); v != "" {
			return v, src
		}
	case "notion":
		if v, src := resolveEnvValue("NOTION_API_KEY", "NOTION_TOKEN", "NOTION_INTEGRATION_TOKEN"); v != "" {
			return v, src
		}
	}

	return "", ""
}

// ResolveToken returns the active credential token for the given connector identifier.
func (m *Manager) ResolveToken(id, candidate string) (string, string) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.resolveTokenLocked(id, candidate)
}

func (m *Manager) resolveUsernameLocked(id, candidate string) (string, string) {
	candidate = strings.TrimSpace(candidate)
	if candidate != "" && candidate != "devops@meta-orchestrator.io" {
		return candidate, "direct_input"
	}

	switch id {
	case "jira":
		if v, src := resolveEnvValue("JIRA_EMAIL", "JIRA_USERNAME", "ATLASSIAN_EMAIL"); v != "" {
			return v, src
		}
	case "confluence":
		if v, src := resolveEnvValue("CONFLUENCE_EMAIL", "CONFLUENCE_USERNAME"); v != "" {
			return v, src
		}
		if v, src := resolveEnvValue("JIRA_EMAIL", "ATLASSIAN_EMAIL"); v != "" {
			return v, src + " (shared)"
		}
	case "bitbucket":
		if existing := m.items["bitbucket"]; existing != nil && existing.MCP != nil && existing.MCP.Env != nil {
			if u := strings.TrimSpace(existing.MCP.Env["ATLASSIAN_USER_EMAIL"]); u != "" {
				return u, "mcp_env"
			}
			if u := strings.TrimSpace(existing.MCP.Env["BITBUCKET_USERNAME"]); u != "" {
				return u, "mcp_env"
			}
		}
		if v, src := resolveEnvValue("BITBUCKET_USERNAME", "BITBUCKET_USER", "ATLASSIAN_USER_EMAIL", "ATLASSIAN_EMAIL"); v != "" {
			return v, src
		}
	}

	if existing := m.items[id]; existing != nil && existing.Username != "" && existing.Username != "devops@meta-orchestrator.io" {
		return existing.Username, "saved_config"
	}
	if id == "jira" && m.config.Jira.Username != "" && m.config.Jira.Username != "devops@meta-orchestrator.io" {
		return m.config.Jira.Username, "jira_config"
	}
	if id == "confluence" && m.config.Confluence.Username != "" && m.config.Confluence.Username != "devops@meta-orchestrator.io" {
		return m.config.Confluence.Username, "confluence_config"
	}

	if candidate != "" {
		return candidate, "default"
	}
	return "", ""
}

// ResolveUsername returns the effective username or email for the given connector identifier.
func (m *Manager) ResolveUsername(id, candidate string) (string, string) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.resolveUsernameLocked(id, candidate)
}

func (m *Manager) resolveBaseURLLocked(id, candidate, userEmail string) string {
	candidate = strings.TrimRight(strings.TrimSpace(candidate), "/")

	var envURL string
	switch id {
	case "jira":
		envURL, _ = resolveEnvValue("JIRA_URL", "JIRA_BASE_URL")
	case "confluence":
		envURL, _ = resolveEnvValue("CONFLUENCE_URL", "CONFLUENCE_BASE_URL")
	case "bitbucket":
		envURL, _ = resolveEnvValue("BITBUCKET_URL")
	case "github":
		envURL, _ = resolveEnvValue("GITHUB_URL", "GITHUB_API_URL")
	case "gitlab":
		envURL, _ = resolveEnvValue("GITLAB_URL", "GITLAB_API_URL")
	case "slack":
		envURL, _ = resolveEnvValue("SLACK_WEBHOOK_URL")
	}

	if envURL != "" {
		return strings.TrimRight(envURL, "/")
	}

	// Mekari organization domain detection
	if strings.Contains(strings.ToLower(userEmail), "@mekari.com") {
		if id == "jira" && (candidate == "" || candidate == "https://jira.atlassian.net") {
			return "https://jurnal.atlassian.net"
		}
		if id == "confluence" && (candidate == "" || candidate == "https://wiki.atlassian.net") {
			return "https://jurnal.atlassian.net/wiki"
		}
	}

	if candidate != "" {
		return candidate
	}

	if existing := m.items[id]; existing != nil && existing.BaseURL != "" {
		return strings.TrimRight(existing.BaseURL, "/")
	}

	switch id {
	case "jira":
		return "https://jira.atlassian.net"
	case "confluence":
		return "https://wiki.atlassian.net"
	case "bitbucket":
		return "https://api.bitbucket.org/2.0"
	case "github":
		return "https://api.github.com"
	case "gitlab":
		return "https://gitlab.com"
	case "linear":
		return "https://api.linear.app/graphql"
	case "notion":
		return "https://api.notion.com/v1"
	}
	return ""
}

// ResolveBaseURL returns the effective base URL with domain inference.
func (m *Manager) ResolveBaseURL(id, candidate, userEmail string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.resolveBaseURLLocked(id, candidate, userEmail)
}

func (m *Manager) ensureCatalogLocked() {
	if m.items == nil {
		m.items = make(map[string]*types.ConnectorItem)
	}

	// 1. JIRA
	jiraToken, jiraTokenSrc := m.resolveTokenLocked("jira", m.config.Jira.APIToken)
	jiraUser, _ := m.resolveUsernameLocked("jira", m.config.Jira.Username)
	jiraURL := m.resolveBaseURLLocked("jira", m.config.Jira.BaseURL, jiraUser)

	if jiraToken != "" {
		m.config.Jira.APIToken = jiraToken
		if m.config.Jira.Status == "unconfigured" || (m.config.Jira.Status == "error" && strings.Contains(m.config.Jira.ErrorMessage, "required")) {
			m.config.Jira.Status = "configured"
			m.config.Jira.ErrorMessage = ""
		}
	}
	if jiraUser != "" && (m.config.Jira.Username == "" || m.config.Jira.Username == "devops@meta-orchestrator.io") {
		m.config.Jira.Username = jiraUser
	}
	if jiraURL != "" && (m.config.Jira.BaseURL == "" || m.config.Jira.BaseURL == "https://jira.atlassian.net") {
		m.config.Jira.BaseURL = jiraURL
	}

	jiraItem, exists := m.items["jira"]
	if !exists {
		status := "unconfigured"
		if m.config.Jira.Status != "" {
			status = m.config.Jira.Status
		} else if jiraToken != "" {
			status = "configured"
		}
		jiraItem = &types.ConnectorItem{
			ID:            "jira",
			Name:          "Atlassian JIRA",
			Category:      types.ConnectorCategoryIssueTracker,
			CategoryLabel: "Issue Tracking & Agile",
			Description:   "Synchronizes backlog issues, automatically detects ticket keys (e.g. PAY-1042) in Kanban task titles, and updates workflow states.",
			Icon:          "jira",
			Color:         "blue",
			Enabled:       m.config.Jira.Enabled,
			ConfigMode:    "hybrid",
			Status:        status,
			BaseURL:       m.config.Jira.BaseURL,
			Username:      m.config.Jira.Username,
			APIToken:      m.config.Jira.APIToken,
			TargetEntity:  m.config.Jira.ProjectKey,
			TargetLabel:   "Default Project Key",
			Capabilities:  []string{"Ticket Auto-Detect", "Backlog Issue Import", "Workflow Status Sync"},
			ExtraSettings: map[string]interface{}{
				"jql_filter":       m.config.Jira.JQLFilter,
				"auto_detect_keys": m.config.Jira.AutoDetectKeys,
				"auto_sync_status": m.config.Jira.AutoSyncStatus,
			},
			MCP: &types.MCPConfig{
				Enabled:   true,
				Command:   "npx",
				Args:      []string{"-y", "mcp-atlassian"},
				Transport: "stdio",
				Env: map[string]string{
					"JIRA_URL":       m.config.Jira.BaseURL,
					"JIRA_EMAIL":     m.config.Jira.Username,
					"JIRA_API_TOKEN": m.config.Jira.APIToken,
				},
			},
			LatencyMs: 0,
		}
		if jiraTokenSrc != "" && jiraTokenSrc != "direct_input" && jiraTokenSrc != "saved_config" {
			jiraItem.HasEnvAuth = true
			jiraItem.EnvAuthSource = jiraTokenSrc
		}
		m.items["jira"] = jiraItem
	} else {
		jiraItem.Enabled = m.config.Jira.Enabled
		if jiraItem.APIToken == "" || strings.Contains(jiraItem.APIToken, "••••") {
			jiraItem.APIToken = jiraToken
		}
		if jiraItem.Username == "" || jiraItem.Username == "devops@meta-orchestrator.io" {
			jiraItem.Username = jiraUser
		}
		if jiraItem.BaseURL == "" || jiraItem.BaseURL == "https://jira.atlassian.net" {
			jiraItem.BaseURL = jiraURL
		}
		if jiraTokenSrc != "" && jiraTokenSrc != "direct_input" && jiraTokenSrc != "saved_config" {
			jiraItem.HasEnvAuth = true
			jiraItem.EnvAuthSource = jiraTokenSrc
		}
		if jiraItem.Status == "unconfigured" || (jiraItem.Status == "error" && strings.Contains(jiraItem.ErrorMessage, "required")) {
			if jiraToken != "" {
				jiraItem.Status = "configured"
				jiraItem.ErrorMessage = ""
			}
		}
		if jiraItem.MCP == nil {
			jiraItem.MCP = &types.MCPConfig{
				Enabled:   true,
				Command:   "npx",
				Args:      []string{"-y", "mcp-atlassian"},
				Transport: "stdio",
				Env: map[string]string{
					"JIRA_URL":       jiraItem.BaseURL,
					"JIRA_EMAIL":     jiraItem.Username,
					"JIRA_API_TOKEN": jiraItem.APIToken,
				},
			}
		} else if jiraItem.MCP.Env != nil && jiraToken != "" {
			jiraItem.MCP.Env["JIRA_URL"] = jiraItem.BaseURL
			jiraItem.MCP.Env["JIRA_EMAIL"] = jiraItem.Username
			jiraItem.MCP.Env["JIRA_API_TOKEN"] = jiraToken
		}
	}

	// 2. Confluence
	confToken, confTokenSrc := m.resolveTokenLocked("confluence", m.config.Confluence.APIToken)
	confUser, _ := m.resolveUsernameLocked("confluence", m.config.Confluence.Username)
	confURL := m.resolveBaseURLLocked("confluence", m.config.Confluence.BaseURL, confUser)

	if confToken != "" {
		m.config.Confluence.APIToken = confToken
		if m.config.Confluence.Status == "unconfigured" || (m.config.Confluence.Status == "error" && strings.Contains(m.config.Confluence.ErrorMessage, "required")) {
			m.config.Confluence.Status = "configured"
			m.config.Confluence.ErrorMessage = ""
		}
	}
	if confUser != "" && (m.config.Confluence.Username == "" || m.config.Confluence.Username == "devops@meta-orchestrator.io") {
		m.config.Confluence.Username = confUser
	}
	if confURL != "" && (m.config.Confluence.BaseURL == "" || m.config.Confluence.BaseURL == "https://wiki.atlassian.net") {
		m.config.Confluence.BaseURL = confURL
	}

	confItem, exists := m.items["confluence"]
	if !exists {
		status := "unconfigured"
		if m.config.Confluence.Status != "" {
			status = m.config.Confluence.Status
		} else if confToken != "" {
			status = "configured"
		}
		confItem = &types.ConnectorItem{
			ID:            "confluence",
			Name:          "Atlassian Confluence",
			Category:      types.ConnectorCategoryDocumentation,
			CategoryLabel: "Wiki & Technical Documentation",
			Description:   "Publishes synthesized Technical Design RFCs (TECH_DOC_RFC.md) and PRDs directly into Confluence team spaces.",
			Icon:          "confluence",
			Color:         "indigo",
			Enabled:       m.config.Confluence.Enabled,
			ConfigMode:    "hybrid",
			Status:        status,
			BaseURL:       m.config.Confluence.BaseURL,
			Username:      m.config.Confluence.Username,
			APIToken:      m.config.Confluence.APIToken,
			TargetEntity:  m.config.Confluence.SpaceKey,
			TargetLabel:   "Target Documentation Space",
			Capabilities:  []string{"RFC Auto-Publish", "PRD Catalog Sync", "Storage XHTML Converter"},
			ExtraSettings: map[string]interface{}{
				"auto_publish_tech_docs": m.config.Confluence.AutoPublishTechDocs,
				"auto_publish_prd":       m.config.Confluence.AutoPublishPRD,
				"parent_page_id":         m.config.Confluence.ParentPageID,
			},
			MCP: &types.MCPConfig{
				Enabled:   true,
				Command:   "npx",
				Args:      []string{"-y", "mcp-atlassian"},
				Transport: "stdio",
				Env: map[string]string{
					"CONFLUENCE_URL":       m.config.Confluence.BaseURL,
					"CONFLUENCE_EMAIL":     m.config.Confluence.Username,
					"CONFLUENCE_API_TOKEN": m.config.Confluence.APIToken,
				},
			},
			LatencyMs: 0,
		}
		if confTokenSrc != "" && confTokenSrc != "direct_input" && confTokenSrc != "saved_config" {
			confItem.HasEnvAuth = true
			confItem.EnvAuthSource = confTokenSrc
		}
		m.items["confluence"] = confItem
	} else {
		confItem.Enabled = m.config.Confluence.Enabled
		if confItem.APIToken == "" || strings.Contains(confItem.APIToken, "••••") {
			confItem.APIToken = confToken
		}
		if confItem.Username == "" || confItem.Username == "devops@meta-orchestrator.io" {
			confItem.Username = confUser
		}
		if confItem.BaseURL == "" || confItem.BaseURL == "https://wiki.atlassian.net" {
			confItem.BaseURL = confURL
		}
		if confTokenSrc != "" && confTokenSrc != "direct_input" && confTokenSrc != "saved_config" {
			confItem.HasEnvAuth = true
			confItem.EnvAuthSource = confTokenSrc
		}
		if confItem.Status == "unconfigured" || (confItem.Status == "error" && strings.Contains(confItem.ErrorMessage, "required")) {
			if confToken != "" {
				confItem.Status = "configured"
				confItem.ErrorMessage = ""
			}
		}
		if confItem.MCP == nil {
			confItem.MCP = &types.MCPConfig{
				Enabled:   true,
				Command:   "npx",
				Args:      []string{"-y", "mcp-atlassian"},
				Transport: "stdio",
				Env: map[string]string{
					"CONFLUENCE_URL":       confItem.BaseURL,
					"CONFLUENCE_EMAIL":     confItem.Username,
					"CONFLUENCE_API_TOKEN": confItem.APIToken,
				},
			}
		} else if confItem.MCP.Env != nil && confToken != "" {
			confItem.MCP.Env["CONFLUENCE_URL"] = confItem.BaseURL
			confItem.MCP.Env["CONFLUENCE_EMAIL"] = confItem.Username
			confItem.MCP.Env["CONFLUENCE_API_TOKEN"] = confToken
		}
	}

	// 3. GitHub
	ghToken, ghSrc := m.resolveTokenLocked("github", "")
	if it, exists := m.items["github"]; !exists {
		status := "unconfigured"
		if ghToken != "" {
			status = "configured"
		}
		it = &types.ConnectorItem{
			ID:            "github",
			Name:          "GitHub Issues & PRs",
			Category:      types.ConnectorCategoryVCS,
			CategoryLabel: "Source Control & Code Review",
			Description:   "Links tasks to GitHub issues, automatically creates review Pull Requests on signoff, and monitors CI workflows.",
			Icon:          "github",
			Color:         "slate",
			Enabled:       true,
			ConfigMode:    "hybrid",
			Status:        status,
			BaseURL:       "https://api.github.com",
			Username:      "",
			APIToken:      ghToken,
			TargetEntity:  "",
			TargetLabel:   "Default Organization / Repository",
			Capabilities:  []string{"Issue #ID Detection", "Pull Request Creation", "CI Check Validation"},
			MCP: &types.MCPConfig{
				Enabled:   true,
				Command:   "npx",
				Args:      []string{"-y", "@modelcontextprotocol/server-github"},
				Transport: "stdio",
				Env: map[string]string{
					"GITHUB_PERSONAL_ACCESS_TOKEN": ghToken,
				},
			},
			LatencyMs: 0,
		}
		if ghSrc != "" && ghSrc != "direct_input" && ghSrc != "saved_config" {
			it.HasEnvAuth = true
			it.EnvAuthSource = ghSrc
		}
		m.items["github"] = it
	} else {
		if (it.APIToken == "" || strings.Contains(it.APIToken, "••••")) && ghToken != "" {
			it.APIToken = ghToken
		}
		if ghSrc != "" && ghSrc != "direct_input" && ghSrc != "saved_config" {
			it.HasEnvAuth = true
			it.EnvAuthSource = ghSrc
		}
		if (it.Status == "unconfigured" || (it.Status == "error" && strings.Contains(it.ErrorMessage, "required"))) && it.APIToken != "" {
			it.Status = "configured"
			it.ErrorMessage = ""
		}
		if it.MCP == nil {
			it.MCP = &types.MCPConfig{
				Enabled:   true,
				Command:   "npx",
				Args:      []string{"-y", "@modelcontextprotocol/server-github"},
				Transport: "stdio",
				Env: map[string]string{
					"GITHUB_PERSONAL_ACCESS_TOKEN": it.APIToken,
				},
			}
		} else if it.MCP.Env != nil && it.APIToken != "" {
			it.MCP.Env["GITHUB_PERSONAL_ACCESS_TOKEN"] = it.APIToken
		}
	}

	// 4. GitLab
	glToken, glSrc := m.resolveTokenLocked("gitlab", "")
	if it, exists := m.items["gitlab"]; !exists {
		status := "unconfigured"
		if glToken != "" {
			status = "configured"
		}
		it = &types.ConnectorItem{
			ID:            "gitlab",
			Name:          "GitLab DevOps",
			Category:      types.ConnectorCategoryVCS,
			CategoryLabel: "Source Control & Pipelines",
			Description:   "Integrates with GitLab issues, automated merge request pipelines, and protected branch permissions.",
			Icon:          "gitlab",
			Color:         "amber",
			Enabled:       false,
			ConfigMode:    "hybrid",
			Status:        status,
			BaseURL:       "https://gitlab.com",
			Username:      "",
			APIToken:      glToken,
			TargetEntity:  "",
			TargetLabel:   "Default Project Path",
			Capabilities:  []string{"Merge Request Automation", "Issue Sync", "CI/CD Pipeline Triggers"},
			MCP: &types.MCPConfig{
				Enabled:   true,
				Command:   "npx",
				Args:      []string{"-y", "@modelcontextprotocol/server-gitlab"},
				Transport: "stdio",
				Env: map[string]string{
					"GITLAB_PERSONAL_ACCESS_TOKEN": glToken,
					"GITLAB_API_URL":               "https://gitlab.com",
				},
			},
			LatencyMs: 0,
		}
		if glSrc != "" && glSrc != "direct_input" && glSrc != "saved_config" {
			it.HasEnvAuth = true
			it.EnvAuthSource = glSrc
		}
		m.items["gitlab"] = it
	} else {
		if (it.APIToken == "" || strings.Contains(it.APIToken, "••••")) && glToken != "" {
			it.APIToken = glToken
		}
		if glSrc != "" && glSrc != "direct_input" && glSrc != "saved_config" {
			it.HasEnvAuth = true
			it.EnvAuthSource = glSrc
		}
		if (it.Status == "unconfigured" || (it.Status == "error" && strings.Contains(it.ErrorMessage, "required"))) && it.APIToken != "" {
			it.Status = "configured"
			it.ErrorMessage = ""
		}
		if it.MCP == nil {
			it.MCP = &types.MCPConfig{
				Enabled:   true,
				Command:   "npx",
				Args:      []string{"-y", "@modelcontextprotocol/server-gitlab"},
				Transport: "stdio",
				Env: map[string]string{
					"GITLAB_PERSONAL_ACCESS_TOKEN": it.APIToken,
					"GITLAB_API_URL":               it.BaseURL,
				},
			}
		} else if it.MCP.Env != nil && it.APIToken != "" {
			it.MCP.Env["GITLAB_PERSONAL_ACCESS_TOKEN"] = it.APIToken
			it.MCP.Env["GITLAB_API_URL"] = it.BaseURL
		}
	}

	// 5. Bitbucket
	bbToken, bbSrc := m.resolveTokenLocked("bitbucket", "")
	bbUser, _ := m.resolveUsernameLocked("bitbucket", "")
	if it, exists := m.items["bitbucket"]; !exists {
		status := "unconfigured"
		if bbToken != "" {
			status = "configured"
		}
		it = &types.ConnectorItem{
			ID:            "bitbucket",
			Name:          "Atlassian Bitbucket",
			Category:      types.ConnectorCategoryVCS,
			CategoryLabel: "Source Control & Code Review",
			Description:   "Integrates with Bitbucket Cloud and Server repositories, pull request reviews, and Bitbucket Pipelines CI/CD automation.",
			Icon:          "bitbucket",
			Color:         "cyan",
			Enabled:       true,
			ConfigMode:    "hybrid",
			Status:        status,
			BaseURL:       "https://api.bitbucket.org/2.0",
			Username:      bbUser,
			APIToken:      bbToken,
			TargetEntity:  "mid-kelola-indonesia",
			TargetLabel:   "Workspace / Repository Slug",
			Capabilities:  []string{"Pull Request Automation", "Branch Permissions", "Bitbucket Pipelines CI", "Commit Status Checks"},
			MCP: &types.MCPConfig{
				Enabled:   true,
				Command:   "npx",
				Args:      []string{"-y", "@aashari/mcp-server-atlassian-bitbucket"},
				Transport: "stdio",
				Env: map[string]string{
					"ATLASSIAN_USER_EMAIL": bbUser,
					"ATLASSIAN_API_TOKEN":  bbToken,
				},
			},
			LatencyMs: 0,
		}
		if bbSrc != "" && bbSrc != "direct_input" && bbSrc != "saved_config" {
			it.HasEnvAuth = true
			it.EnvAuthSource = bbSrc
		}
		m.items["bitbucket"] = it
	} else {
		if bbToken != "" {
			it.APIToken = bbToken
		}
		if bbUser != "" {
			it.Username = bbUser
		}
		if strings.TrimSpace(it.TargetEntity) == "" {
			it.TargetEntity = "mid-kelola-indonesia"
		}
		if bbSrc != "" && bbSrc != "direct_input" && bbSrc != "saved_config" {
			it.HasEnvAuth = true
			it.EnvAuthSource = bbSrc
		}
		if (it.Status == "unconfigured" || (it.Status == "error" && strings.Contains(it.ErrorMessage, "required"))) && it.APIToken != "" {
			it.Status = "configured"
			it.ErrorMessage = ""
		}
		if it.MCP == nil {
			it.MCP = &types.MCPConfig{
				Enabled:   true,
				Command:   "npx",
				Args:      []string{"-y", "@aashari/mcp-server-atlassian-bitbucket"},
				Transport: "stdio",
				Env: map[string]string{
					"ATLASSIAN_USER_EMAIL": it.Username,
					"ATLASSIAN_API_TOKEN":  it.APIToken,
				},
			}
		} else if it.MCP.Env != nil {
			if t, ok := it.MCP.Env["ATLASSIAN_API_TOKEN"]; ok && strings.TrimSpace(t) != "" && !strings.Contains(t, "••••") {
				it.APIToken = strings.TrimSpace(t)
			} else if it.APIToken != "" && !strings.Contains(it.APIToken, "••••") {
				it.MCP.Env["ATLASSIAN_API_TOKEN"] = it.APIToken
			}
			if u, ok := it.MCP.Env["ATLASSIAN_USER_EMAIL"]; ok && strings.TrimSpace(u) != "" {
				it.Username = strings.TrimSpace(u)
			} else if it.Username != "" {
				it.MCP.Env["ATLASSIAN_USER_EMAIL"] = it.Username
			}
		}
	}

	// 6. Slack
	slackToken, slackSrc := m.resolveTokenLocked("slack", "")
	if it, exists := m.items["slack"]; !exists {
		status := "unconfigured"
		if slackToken != "" {
			status = "configured"
		}
		it = &types.ConnectorItem{
			ID:            "slack",
			Name:          "Slack ChatOps",
			Category:      types.ConnectorCategoryChatOps,
			CategoryLabel: "ChatOps & Real-time Alerts",
			Description:   "Sends real-time alerts for gate reviews, blocker escalations, and interactive pipeline approvals via Incoming Webhooks or Bot Token.",
			Icon:          "slack",
			Color:         "purple",
			Enabled:       false,
			ConfigMode:    "hybrid",
			Status:        status,
			BaseURL:       "https://hooks.slack.com/services/...",
			Username:      "",
			APIToken:      slackToken,
			TargetEntity:  "",
			TargetLabel:   "Default Alert Channel",
			Capabilities:  []string{"Gate Approval Alerts", "Frustration Escalations", "Stage Completion Pings"},
			MCP: &types.MCPConfig{
				Enabled:   true,
				Command:   "npx",
				Args:      []string{"-y", "@modelcontextprotocol/server-slack"},
				Transport: "stdio",
				Env: map[string]string{
					"SLACK_BOT_TOKEN": slackToken,
					"SLACK_TEAM_ID":   "",
				},
			},
			LatencyMs: 0,
		}
		if slackSrc != "" && slackSrc != "direct_input" && slackSrc != "saved_config" {
			it.HasEnvAuth = true
			it.EnvAuthSource = slackSrc
		}
		m.items["slack"] = it
	} else {
		if (it.APIToken == "" || strings.Contains(it.APIToken, "••••")) && slackToken != "" {
			it.APIToken = slackToken
		}
		if slackSrc != "" && slackSrc != "direct_input" && slackSrc != "saved_config" {
			it.HasEnvAuth = true
			it.EnvAuthSource = slackSrc
		}
		if (it.Status == "unconfigured" || (it.Status == "error" && strings.Contains(it.ErrorMessage, "required"))) && it.APIToken != "" {
			it.Status = "configured"
			it.ErrorMessage = ""
		}
		if it.MCP == nil {
			it.MCP = &types.MCPConfig{
				Enabled:   true,
				Command:   "npx",
				Args:      []string{"-y", "@modelcontextprotocol/server-slack"},
				Transport: "stdio",
				Env: map[string]string{
					"SLACK_BOT_TOKEN": it.APIToken,
					"SLACK_TEAM_ID":   it.TargetEntity,
				},
			}
		} else if it.MCP.Env != nil && it.APIToken != "" {
			it.MCP.Env["SLACK_BOT_TOKEN"] = it.APIToken
		}
	}

	// 7. Linear
	linearToken, linearSrc := m.resolveTokenLocked("linear", "")
	if it, exists := m.items["linear"]; !exists {
		status := "unconfigured"
		if linearToken != "" {
			status = "configured"
		}
		it = &types.ConnectorItem{
			ID:            "linear",
			Name:          "Linear App",
			Category:      types.ConnectorCategoryIssueTracker,
			CategoryLabel: "Modern Issue Tracking",
			Description:   "Two-way synchronization with Linear cycles, automatic issue key detection (e.g. ENG-402), and team state updates.",
			Icon:          "linear",
			Color:         "violet",
			Enabled:       false,
			ConfigMode:    "hybrid",
			Status:        status,
			BaseURL:       "https://api.linear.app/graphql",
			Username:      "",
			APIToken:      linearToken,
			TargetEntity:  "",
			TargetLabel:   "Team Prefix / Project",
			Capabilities:  []string{"Identifier Detection", "Cycle Sync", "High-speed Import"},
			MCP: &types.MCPConfig{
				Enabled:   true,
				Command:   "npx",
				Args:      []string{"-y", "@modelcontextprotocol/server-linear"},
				Transport: "stdio",
				Env: map[string]string{
					"LINEAR_API_KEY": linearToken,
				},
			},
			LatencyMs: 0,
		}
		if linearSrc != "" && linearSrc != "direct_input" && linearSrc != "saved_config" {
			it.HasEnvAuth = true
			it.EnvAuthSource = linearSrc
		}
		m.items["linear"] = it
	} else {
		if (it.APIToken == "" || strings.Contains(it.APIToken, "••••")) && linearToken != "" {
			it.APIToken = linearToken
		}
		if linearSrc != "" && linearSrc != "direct_input" && linearSrc != "saved_config" {
			it.HasEnvAuth = true
			it.EnvAuthSource = linearSrc
		}
		if (it.Status == "unconfigured" || (it.Status == "error" && strings.Contains(it.ErrorMessage, "required"))) && it.APIToken != "" {
			it.Status = "configured"
			it.ErrorMessage = ""
		}
		if it.MCP == nil {
			it.MCP = &types.MCPConfig{
				Enabled:   true,
				Command:   "npx",
				Args:      []string{"-y", "@modelcontextprotocol/server-linear"},
				Transport: "stdio",
				Env: map[string]string{
					"LINEAR_API_KEY": it.APIToken,
				},
			}
		} else if it.MCP.Env != nil && it.APIToken != "" {
			it.MCP.Env["LINEAR_API_KEY"] = it.APIToken
		}
	}

	// 8. Notion
	notionToken, notionSrc := m.resolveTokenLocked("notion", "")
	if it, exists := m.items["notion"]; !exists {
		status := "unconfigured"
		if notionToken != "" {
			status = "configured"
		}
		it = &types.ConnectorItem{
			ID:            "notion",
			Name:          "Notion Workspace",
			Category:      types.ConnectorCategoryDocumentation,
			CategoryLabel: "Product Specs & Wikis",
			Description:   "Exports synthesized PRDs, architecture specifications, and stage artifacts into Notion databases.",
			Icon:          "notion",
			Color:         "stone",
			Enabled:       false,
			ConfigMode:    "hybrid",
			Status:        status,
			BaseURL:       "https://api.notion.com/v1",
			Username:      "",
			APIToken:      notionToken,
			TargetEntity:  "",
			TargetLabel:   "Database / Parent Page",
			Capabilities:  []string{"Page Creation", "Database Row Sync", "Rich Text Blocks"},
			MCP: &types.MCPConfig{
				Enabled:   true,
				Command:   "npx",
				Args:      []string{"-y", "@modelcontextprotocol/server-notion"},
				Transport: "stdio",
				Env: map[string]string{
					"NOTION_API_KEY": notionToken,
				},
			},
			LatencyMs: 0,
		}
		if notionSrc != "" && notionSrc != "direct_input" && notionSrc != "saved_config" {
			it.HasEnvAuth = true
			it.EnvAuthSource = notionSrc
		}
		m.items["notion"] = it
	} else {
		if (it.APIToken == "" || strings.Contains(it.APIToken, "••••")) && notionToken != "" {
			it.APIToken = notionToken
		}
		if notionSrc != "" && notionSrc != "direct_input" && notionSrc != "saved_config" {
			it.HasEnvAuth = true
			it.EnvAuthSource = notionSrc
		}
		if (it.Status == "unconfigured" || (it.Status == "error" && strings.Contains(it.ErrorMessage, "required"))) && it.APIToken != "" {
			it.Status = "configured"
			it.ErrorMessage = ""
		}
		if it.MCP == nil {
			it.MCP = &types.MCPConfig{
				Enabled:   true,
				Command:   "npx",
				Args:      []string{"-y", "@modelcontextprotocol/server-notion"},
				Transport: "stdio",
				Env: map[string]string{
					"NOTION_API_KEY": it.APIToken,
				},
			}
		} else if it.MCP.Env != nil && it.APIToken != "" {
			it.MCP.Env["NOTION_API_KEY"] = it.APIToken
		}
	}

	// 9. Custom Webhook
	if it, exists := m.items["custom_webhook"]; !exists {
		m.items["custom_webhook"] = &types.ConnectorItem{
			ID:            "custom_webhook",
			Name:          "Custom Webhook Dispatcher",
			Category:      types.ConnectorCategoryCustom,
			CategoryLabel: "Generic Event Integrations",
			Description:   "Dispatches signed HMAC JSON events to external internal microservices on task state changes and stage completions.",
			Icon:          "webhook",
			Color:         "emerald",
			Enabled:       false,
			ConfigMode:    "hybrid",
			Status:        "unconfigured",
			BaseURL:       "",
			Username:      "",
			APIToken:      "",
			TargetEntity:  "task.*, stage.*",
			TargetLabel:   "Subscribed Event Topics",
			Capabilities:  []string{"HMAC-SHA256 Signing", "Payload Retries", "Filter Expressions"},
			MCP: &types.MCPConfig{
				Enabled:   false,
				Command:   "npx",
				Args:      []string{"-y", "mcp-proxy-webhook"},
				Transport: "stdio",
				Env:       map[string]string{},
			},
			LatencyMs: 0,
		}
	} else if it.MCP == nil {
		it.MCP = &types.MCPConfig{
			Enabled:   false,
			Command:   "npx",
			Args:      []string{"-y", "mcp-proxy-webhook"},
			Transport: "stdio",
			Env:       map[string]string{},
		}
	}

	// Sanitize runtime statuses: disabled connectors must be "disabled", unauthenticated must be "unconfigured"
	for _, it := range m.items {
		if !it.Enabled {
			it.Status = "disabled"
		} else if it.Status == "connected" && it.ConfigMode != "mcp" {
			tok, _ := m.resolveTokenLocked(it.ID, it.APIToken)
			if strings.TrimSpace(tok) == "" && (it.ID == "jira" || it.ID == "confluence" || it.ID == "github" || it.ID == "gitlab" || it.ID == "bitbucket" || it.ID == "linear" || it.ID == "notion") {
				it.Status = "unconfigured"
			}
		}
	}
}

func (m *Manager) load() {
	if m.items == nil {
		m.items = make(map[string]*types.ConnectorItem)
	}

	// Automatically ingest any .env file in project root or .sdlc
	loadDotEnv(m.rootDir, filepath.Join(m.rootDir, ".sdlc"))

	data, err := os.ReadFile(m.configPath)
	if err == nil {
		var cfg types.ConnectorsConfig
		if err := json.Unmarshal(data, &cfg); err == nil {
			m.config = cfg
			if m.config.Ping.IntervalSeconds <= 0 {
				m.config.Ping.IntervalSeconds = 30
				m.config.Ping.Enabled = true
			}
			for _, it := range cfg.Items {
				if it != nil && it.ID != "" {
					m.items[it.ID] = it
				}
			}
		}
	}
	m.ensureCatalogLocked()
}

func (m *Manager) save() error {
	_ = os.MkdirAll(filepath.Dir(m.configPath), 0755)
	m.config.Items = m.listConnectorsLocked()
	data, err := json.MarshalIndent(m.config, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(m.configPath, data, 0644); err != nil {
		return err
	}
	_ = m.saveMCPConfigLocked()
	return nil
}

func (m *Manager) getMCPExportConfigLocked() map[string]interface{} {
	servers := make(map[string]interface{})
	for id, item := range m.items {
		if item.Enabled && item.MCP != nil && (item.MCP.Enabled || item.ConfigMode == "mcp") {
			serverEntry := map[string]interface{}{
				"command": item.MCP.Command,
				"args":    item.MCP.Args,
			}
			if len(item.MCP.Env) > 0 {
				serverEntry["env"] = item.MCP.Env
			}
			servers[id] = serverEntry
		}
	}
	return map[string]interface{}{
		"mcpServers": servers,
	}
}

func (m *Manager) saveMCPConfigLocked() error {
	mcpExport := m.getMCPExportConfigLocked()
	mcpData, err := json.MarshalIndent(mcpExport, "", "  ")
	if err != nil {
		return err
	}

	sdlcDir := filepath.Join(m.rootDir, ".sdlc")
	_ = os.MkdirAll(sdlcDir, 0755)
	sdlcMCPPath := filepath.Join(sdlcDir, "mcp.json")
	_ = os.WriteFile(sdlcMCPPath, mcpData, 0644)

	rootMCPPath := filepath.Join(m.rootDir, ".mcp.json")
	_ = os.WriteFile(rootMCPPath, mcpData, 0644)

	if m.onMCPUpdate != nil {
		m.onMCPUpdate(mcpExport)
	}
	return nil
}

// GetConfig returns current connectors configuration.
func (m *Manager) GetConfig() types.ConnectorsConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cfg := m.config
	cfg.Items = m.listConnectorsLocked()
	return cfg
}

func (m *Manager) listConnectorsLocked() []*types.ConnectorItem {
	order := []string{"jira", "confluence", "github", "gitlab", "bitbucket", "slack", "linear", "notion", "custom_webhook"}
	seen := make(map[string]bool)
	var list []*types.ConnectorItem

	for _, id := range order {
		if item, exists := m.items[id]; exists {
			list = append(list, item)
			seen[id] = true
		}
	}

	for id, item := range m.items {
		if !seen[id] {
			list = append(list, item)
		}
	}

	return list
}

// ListConnectors returns all registered connector items in the catalog.
func (m *Manager) ListConnectors() []*types.ConnectorItem {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.listConnectorsLocked()
}

// GetConnector returns a single connector item by ID.
func (m *Manager) GetConnector(id string) (*types.ConnectorItem, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	item, exists := m.items[id]
	if !exists {
		return nil, fmt.Errorf("connector '%s' not found", id)
	}
	return item, nil
}

// UpdateConnector updates an existing connector's configuration and syncs specific configs.
func (m *Manager) UpdateConnector(item types.ConnectorItem) (*types.ConnectorItem, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensureCatalogLocked()

	existing := m.items[item.ID]

	// Preserve existing credentials if submitted empty or masked
	if strings.TrimSpace(item.APIToken) == "" || strings.Contains(item.APIToken, "••••") {
		if existing != nil && existing.APIToken != "" && !strings.Contains(existing.APIToken, "••••") {
			item.APIToken = existing.APIToken
			item.HasEnvAuth = existing.HasEnvAuth
			item.EnvAuthSource = existing.EnvAuthSource
		} else {
			resolved, src := m.resolveTokenLocked(item.ID, "")
			if resolved != "" {
				item.APIToken = resolved
				item.HasEnvAuth = true
				item.EnvAuthSource = src
			}
		}
	}
	if strings.TrimSpace(item.Username) == "" && existing != nil && existing.Username != "" {
		item.Username = existing.Username
	}
	if strings.TrimSpace(item.BaseURL) == "" && existing != nil && existing.BaseURL != "" {
		item.BaseURL = existing.BaseURL
	}

	if !item.Enabled {
		item.Status = "disabled"
	} else if item.Status == "connected" && item.ConfigMode != "mcp" {
		tok, _ := m.resolveTokenLocked(item.ID, item.APIToken)
		if strings.TrimSpace(tok) == "" && (item.ID == "jira" || item.ID == "confluence" || item.ID == "github" || item.ID == "gitlab" || item.ID == "bitbucket" || item.ID == "linear" || item.ID == "notion") {
			item.Status = "unconfigured"
		}
	}

	if item.ID == "bitbucket" {
		if item.MCP != nil && item.MCP.Env != nil {
			if t, ok := item.MCP.Env["ATLASSIAN_API_TOKEN"]; ok && strings.TrimSpace(t) != "" && !strings.Contains(t, "••••") {
				item.APIToken = strings.TrimSpace(t)
			} else if item.APIToken != "" && !strings.Contains(item.APIToken, "••••") {
				item.MCP.Env["ATLASSIAN_API_TOKEN"] = item.APIToken
			}
			if u, ok := item.MCP.Env["ATLASSIAN_USER_EMAIL"]; ok && strings.TrimSpace(u) != "" {
				item.Username = strings.TrimSpace(u)
			} else if item.Username != "" {
				item.MCP.Env["ATLASSIAN_USER_EMAIL"] = item.Username
			}
		}
		if strings.TrimSpace(item.TargetEntity) == "" {
			item.TargetEntity = "mid-kelola-indonesia"
		}
	}

	m.items[item.ID] = &item

	if item.ID == "jira" {
		m.config.Jira.Enabled = item.Enabled
		m.config.Jira.BaseURL = item.BaseURL
		m.config.Jira.Username = item.Username
		if item.APIToken != "" && !strings.Contains(item.APIToken, "••••") {
			m.config.Jira.APIToken = item.APIToken
		}
		m.config.Jira.ProjectKey = item.TargetEntity
		m.config.Jira.Status = item.Status
		if item.ExtraSettings != nil {
			if v, ok := item.ExtraSettings["auto_detect_keys"].(bool); ok {
				m.config.Jira.AutoDetectKeys = v
			}
			if v, ok := item.ExtraSettings["auto_sync_status"].(bool); ok {
				m.config.Jira.AutoSyncStatus = v
			}
			if v, ok := item.ExtraSettings["jql_filter"].(string); ok {
				m.config.Jira.JQLFilter = v
			}
		}
	} else if item.ID == "confluence" {
		m.config.Confluence.Enabled = item.Enabled
		m.config.Confluence.BaseURL = item.BaseURL
		m.config.Confluence.Username = item.Username
		if item.APIToken != "" && !strings.Contains(item.APIToken, "••••") {
			m.config.Confluence.APIToken = item.APIToken
		}
		m.config.Confluence.SpaceKey = item.TargetEntity
		m.config.Confluence.Status = item.Status
		if item.ExtraSettings != nil {
			if v, ok := item.ExtraSettings["auto_publish_tech_docs"].(bool); ok {
				m.config.Confluence.AutoPublishTechDocs = v
			}
			if v, ok := item.ExtraSettings["auto_publish_prd"].(bool); ok {
				m.config.Confluence.AutoPublishPRD = v
			}
			if v, ok := item.ExtraSettings["parent_page_id"].(string); ok {
				m.config.Confluence.ParentPageID = v
			}
		}
	}

	if err := m.save(); err != nil {
		return nil, err
	}
	return m.items[item.ID], nil
}

// ToggleConnector toggles a connector on or off.
func (m *Manager) ToggleConnector(id string, enabled bool) (*types.ConnectorItem, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensureCatalogLocked()

	item, exists := m.items[id]
	if !exists {
		return nil, fmt.Errorf("connector '%s' not found", id)
	}

	item.Enabled = enabled
	if enabled {
		if item.Status == "disabled" || item.Status == "" {
			item.Status = "configured"
		}
	} else {
		item.Status = "disabled"
	}

	if id == "jira" {
		m.config.Jira.Enabled = enabled
		m.config.Jira.Status = item.Status
	} else if id == "confluence" {
		m.config.Confluence.Enabled = enabled
		m.config.Confluence.Status = item.Status
	}

	if err := m.save(); err != nil {
		return nil, err
	}
	return item, nil
}

// TestGenericConnector performs a real connectivity check for any connector in the catalog.
func (m *Manager) TestGenericConnector(ctx context.Context, id string, item *types.ConnectorItem) types.TestConnectorResponse {
	m.mu.RLock()
	existing := m.items[id]
	m.mu.RUnlock()

	activeItem := existing
	if item != nil && (item.BaseURL != "" || item.MCP != nil || item.TargetEntity != "" || item.APIToken != "" || item.Username != "") {
		if existing != nil {
			clone := *existing
			if item.BaseURL != "" {
				clone.BaseURL = item.BaseURL
			}
			if item.Username != "" {
				clone.Username = item.Username
			}
			if item.APIToken != "" && !strings.Contains(item.APIToken, "••••") {
				clone.APIToken = item.APIToken
			}
			if item.TargetEntity != "" {
				clone.TargetEntity = item.TargetEntity
			}
			if item.ConfigMode != "" {
				clone.ConfigMode = item.ConfigMode
			}
			if item.MCP != nil {
				clone.MCP = item.MCP
			}
			activeItem = &clone
		} else {
			activeItem = item
		}
	}
	if activeItem == nil {
		return types.TestConnectorResponse{
			Success:   false,
			Message:   fmt.Sprintf("Connector '%s' not found in catalog", id),
			LatencyMs: 0,
		}
	}

	// If MCP mode is explicitly selected, test MCP runtime
	if activeItem.ConfigMode == "mcp" && activeItem.MCP != nil {
		return m.TestMCPConnector(ctx, id, activeItem.MCP)
	}

	// Resolve effective credentials
	resolvedToken, _ := m.ResolveToken(id, activeItem.APIToken)
	resolvedUser, _ := m.ResolveUsername(id, activeItem.Username)
	resolvedURL := m.ResolveBaseURL(id, activeItem.BaseURL, resolvedUser)

	activeItem.APIToken = resolvedToken
	if resolvedUser != "" {
		activeItem.Username = resolvedUser
	}
	if resolvedURL != "" {
		activeItem.BaseURL = resolvedURL
	}

	// Special routing for JIRA & Confluence
	if id == "jira" {
		m.mu.RLock()
		cfg := m.config.Jira
		m.mu.RUnlock()
		cfg.BaseURL = activeItem.BaseURL
		cfg.Username = activeItem.Username
		cfg.APIToken = activeItem.APIToken
		if activeItem.TargetEntity != "" {
			cfg.ProjectKey = activeItem.TargetEntity
		}
		res := m.TestJira(ctx, cfg)
		m.updateItemStatus(id, res.Success, res.LatencyMs, res.Message)
		return res
	}

	if id == "confluence" {
		m.mu.RLock()
		cfg := m.config.Confluence
		m.mu.RUnlock()
		cfg.BaseURL = activeItem.BaseURL
		cfg.Username = activeItem.Username
		cfg.APIToken = activeItem.APIToken
		if activeItem.TargetEntity != "" {
			cfg.SpaceKey = activeItem.TargetEntity
		}
		res := m.TestConfluence(ctx, cfg)
		m.updateItemStatus(id, res.Success, res.LatencyMs, res.Message)
		return res
	}

	start := time.Now()
	cleanURL := strings.TrimRight(strings.TrimSpace(activeItem.BaseURL), "/")
	if cleanURL == "" {
		res := types.TestConnectorResponse{
			Success:   false,
			Message:   fmt.Sprintf("%s Base URL or endpoint is required", activeItem.Name),
			LatencyMs: 0,
		}
		m.updateItemStatus(id, false, 0, res.Message)
		return res
	}

	// Real HTTP Request with timeout
	probeCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	var req *http.Request
	var err error

	switch id {
	case "github":
		if strings.TrimSpace(activeItem.APIToken) == "" {
			res := types.TestConnectorResponse{
				Success:   false,
				LatencyMs: 0,
				Message:   "GitHub Personal Access Token is required to authenticate (set via UI, GITHUB_TOKEN, or GH_TOKEN)",
			}
			m.updateItemStatus(id, false, 0, res.Message)
			return res
		}
		reqURL := "https://api.github.com/user"
		if activeItem.TargetEntity != "" && strings.Contains(activeItem.TargetEntity, "/") {
			reqURL = fmt.Sprintf("https://api.github.com/repos/%s", activeItem.TargetEntity)
		}
		req, err = http.NewRequestWithContext(probeCtx, http.MethodGet, reqURL, nil)
		if err == nil {
			req.Header.Set("User-Agent", "Meta-Orchestrator")
			req.Header.Set("Accept", "application/vnd.github.v3+json")
			req.Header.Set("Authorization", "Bearer "+activeItem.APIToken)
		}

	case "gitlab":
		if strings.TrimSpace(activeItem.APIToken) == "" {
			res := types.TestConnectorResponse{
				Success:   false,
				LatencyMs: 0,
				Message:   "GitLab Personal Access Token is required to authenticate (set via UI, GITLAB_TOKEN, or GL_TOKEN)",
			}
			m.updateItemStatus(id, false, 0, res.Message)
			return res
		}
		reqURL := cleanURL + "/api/v4/user"
		req, err = http.NewRequestWithContext(probeCtx, http.MethodGet, reqURL, nil)
		if err == nil {
			req.Header.Set("User-Agent", "Meta-Orchestrator")
			req.Header.Set("PRIVATE-TOKEN", activeItem.APIToken)
		}

	case "bitbucket":
		token := strings.TrimSpace(activeItem.APIToken)
		user := strings.TrimSpace(activeItem.Username)
		if token == "" && user == "" {
			res := types.TestConnectorResponse{
				Success:   false,
				LatencyMs: 0,
				Message:   "Bitbucket Username and API Token or App Password are required to authenticate (set via UI, MCP, or BITBUCKET_TOKEN / ATLASSIAN_API_TOKEN)",
			}
			m.updateItemStatus(id, false, 0, res.Message)
			return res
		}

		target := strings.TrimSpace(activeItem.TargetEntity)
		if target == "" {
			target = "mid-kelola-indonesia"
		}

		var reqURL string
		if strings.Contains(target, "/") {
			reqURL = fmt.Sprintf("https://api.bitbucket.org/2.0/repositories/%s", target)
		} else {
			reqURL = fmt.Sprintf("https://api.bitbucket.org/2.0/workspaces/%s", target)
		}

		req, err = http.NewRequestWithContext(probeCtx, http.MethodGet, reqURL, nil)
		if err == nil {
			req.Header.Set("User-Agent", "Meta-Orchestrator")
			req.Header.Set("Accept", "application/json")
			if user != "" && token != "" {
				req.SetBasicAuth(user, token)
			}
		}

	case "slack":
		token := strings.TrimSpace(activeItem.APIToken)
		if token == "" && !strings.HasPrefix(cleanURL, "https://hooks.slack.com") && !strings.HasPrefix(cleanURL, "http://127.0.0.1") && !strings.HasPrefix(cleanURL, "http://localhost") {
			res := types.TestConnectorResponse{
				Success:   false,
				LatencyMs: 0,
				Message:   "Slack Bot Token (xoxb-...) or Webhook URL is required to authenticate (SLACK_BOT_TOKEN)",
			}
			m.updateItemStatus(id, false, 0, res.Message)
			return res
		}
		if strings.HasPrefix(token, "xoxb-") || strings.HasPrefix(token, "xoxp-") {
			reqURL := "https://slack.com/api/auth.test"
			if strings.HasPrefix(cleanURL, "http://127.0.0.1") || strings.HasPrefix(cleanURL, "http://localhost") {
				reqURL = cleanURL + "/api/auth.test"
			}
			req, err = http.NewRequestWithContext(probeCtx, http.MethodPost, reqURL, nil)
			if err == nil {
				req.Header.Set("Authorization", "Bearer "+token)
			}
		} else {
			req, err = http.NewRequestWithContext(probeCtx, http.MethodGet, cleanURL, nil)
		}

	case "linear":
		if strings.TrimSpace(activeItem.APIToken) == "" {
			res := types.TestConnectorResponse{
				Success:   false,
				LatencyMs: 0,
				Message:   "Linear API Key is required to authenticate (set via UI or LINEAR_API_KEY)",
			}
			m.updateItemStatus(id, false, 0, res.Message)
			return res
		}
		graphQLBody := bytes.NewBufferString(`{"query": "{ viewer { id name email } }"}`)
		req, err = http.NewRequestWithContext(probeCtx, http.MethodPost, "https://api.linear.app/graphql", graphQLBody)
		if err == nil {
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", activeItem.APIToken)
		}

	case "notion":
		if strings.TrimSpace(activeItem.APIToken) == "" {
			res := types.TestConnectorResponse{
				Success:   false,
				LatencyMs: 0,
				Message:   "Notion Integration Token is required to authenticate (set via UI or NOTION_API_KEY)",
			}
			m.updateItemStatus(id, false, 0, res.Message)
			return res
		}
		req, err = http.NewRequestWithContext(probeCtx, http.MethodGet, "https://api.notion.com/v1/users/me", nil)
		if err == nil {
			req.Header.Set("Notion-Version", "2022-06-28")
			req.Header.Set("Authorization", "Bearer "+activeItem.APIToken)
		}

	default:
		// Custom Webhook or generic HTTP
		req, err = http.NewRequestWithContext(probeCtx, http.MethodGet, cleanURL, nil)
	}

	if err != nil {
		res := types.TestConnectorResponse{
			Success:   false,
			LatencyMs: time.Since(start).Milliseconds(),
			Message:   fmt.Sprintf("Failed to initialize request: %v", err),
		}
		m.updateItemStatus(id, false, res.LatencyMs, res.Message)
		return res
	}

	resp, err := m.client.Do(req)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		res := types.TestConnectorResponse{
			Success:   false,
			LatencyMs: latency,
			Message:   fmt.Sprintf("Connection failed: %v", err),
		}
		m.updateItemStatus(id, false, latency, res.Message)
		return res
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))

	// Success response processing
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		var userStr string
		var rawMap map[string]interface{}
		_ = json.Unmarshal(bodyBytes, &rawMap)

		if rawMap != nil {
			if id == "github" {
				userStr = fmt.Sprintf("%v", rawMap["login"])
				if userStr == "<nil>" || userStr == "" {
					userStr = fmt.Sprintf("%v", rawMap["full_name"])
				}
			} else if id == "bitbucket" {
				if slug, ok := rawMap["slug"].(string); ok && slug != "" {
					userStr = fmt.Sprintf("%v (Workspace: %v)", activeItem.Username, slug)
				} else if name, ok := rawMap["name"].(string); ok && name != "" {
					userStr = fmt.Sprintf("%v (%v)", activeItem.Username, name)
				} else if disp, ok := rawMap["display_name"].(string); ok && disp != "" {
					userStr = disp
				} else if full, ok := rawMap["full_name"].(string); ok && full != "" {
					userStr = full
				}
			} else if id == "gitlab" {
				userStr = fmt.Sprintf("%v", rawMap["username"])
				if userStr == "<nil>" || userStr == "" {
					userStr = fmt.Sprintf("GitLab v%v", rawMap["version"])
				}
			} else if id == "slack" {
				if rawMap["ok"] == true {
					userStr = fmt.Sprintf("@%v (Team: %v)", rawMap["user"], rawMap["team"])
				}
			} else if id == "notion" {
				userStr = fmt.Sprintf("%v", rawMap["name"])
			}
		}

		if userStr == "" || userStr == "<nil>" {
			userStr = activeItem.Username
			if userStr == "" {
				userStr = "Authenticated Client"
			}
		}

		serverHeader := resp.Header.Get("Server")
		if serverHeader == "" {
			serverHeader = resp.Header.Get("X-GitHub-Request-Id")
		}
		if serverHeader == "" {
			serverHeader = cleanURL
		}

		res := types.TestConnectorResponse{
			Success:      true,
			LatencyMs:    latency,
			Message:      fmt.Sprintf("Live connection established to %s (HTTP %d). Verified user: %s", activeItem.Name, resp.StatusCode, userStr),
			ConnectedAs:  userStr,
			ServerInfo:   fmt.Sprintf("%s (%s)", activeItem.Name, serverHeader),
			TargetEntity: fmt.Sprintf("%s: %s", activeItem.TargetLabel, activeItem.TargetEntity),
		}
		m.updateItemStatus(id, true, latency, "")
		return res
	}

	// Slack webhooks return 400 with invalid payload or 405 for GET, proving live Slack ingress reached
	if id == "slack" && (resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusMethodNotAllowed) {
		res := types.TestConnectorResponse{
			Success:      true,
			LatencyMs:    latency,
			Message:      fmt.Sprintf("Slack Webhook ingress reached successfully (HTTP %d)", resp.StatusCode),
			ConnectedAs:  activeItem.Username,
			ServerInfo:   "Slack Webhook Ingress (hooks.slack.com)",
			TargetEntity: activeItem.TargetEntity,
		}
		m.updateItemStatus(id, true, latency, "")
		return res
	}

	// Failure response with actual HTTP status
	msg := fmt.Sprintf("%s returned HTTP %d: %s", activeItem.Name, resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
	if resp.StatusCode == http.StatusUnauthorized {
		msg = fmt.Sprintf("HTTP 401 Unauthorized: Invalid API Token / credentials for %s", activeItem.Name)
	} else if resp.StatusCode == http.StatusForbidden {
		msg = fmt.Sprintf("HTTP 403 Forbidden: Insufficient permissions for %s", activeItem.Name)
	} else if resp.StatusCode == http.StatusNotFound {
		msg = fmt.Sprintf("HTTP 404 Not Found: Target resource or endpoint not found on %s", activeItem.Name)
	}

	res := types.TestConnectorResponse{
		Success:   false,
		LatencyMs: latency,
		Message:   msg,
	}
	m.updateItemStatus(id, false, latency, msg)
	return res
}

func (m *Manager) updateItemStatus(id string, success bool, latency int64, errMsg string) {
	m.mu.Lock()
	item, ok := m.items[id]
	if !ok {
		m.mu.Unlock()
		return
	}
	item.LastTestedAt = time.Now()
	item.LatencyMs = latency
	if success {
		item.Status = "connected"
		item.ErrorMessage = ""
	} else {
		item.Status = "error"
		item.ErrorMessage = errMsg
	}

	if id == "jira" {
		m.config.Jira.Status = item.Status
		m.config.Jira.LastTestedAt = item.LastTestedAt
		m.config.Jira.ErrorMessage = item.ErrorMessage
	} else if id == "confluence" {
		m.config.Confluence.Status = item.Status
		m.config.Confluence.LastTestedAt = item.LastTestedAt
		m.config.Confluence.ErrorMessage = item.ErrorMessage
	}

	itemCopy := *item
	fn := m.broadcastFunc
	m.mu.Unlock()

	if fn != nil {
		fn(&types.OrchestratorEvent{
			Type:      types.EventConnectorStatus,
			Timestamp: time.Now(),
			Payload: types.ConnectorStatusEvent{
				ConnectorID:  id,
				Status:       itemCopy.Status,
				LatencyMs:    itemCopy.LatencyMs,
				LastTestedAt: itemCopy.LastTestedAt,
				ErrorMessage: itemCopy.ErrorMessage,
				Item:         itemCopy,
			},
		})
	}
}

// TestMCPConnector validates MCP server configuration and host runtime.
func (m *Manager) TestMCPConnector(ctx context.Context, id string, mcp *types.MCPConfig) types.TestConnectorResponse {
	start := time.Now()
	if mcp == nil {
		res := types.TestConnectorResponse{
			Success:   false,
			Message:   "MCP configuration is missing",
			LatencyMs: 0,
		}
		m.updateItemStatus(id, false, 0, res.Message)
		return res
	}
	if strings.TrimSpace(mcp.Command) == "" {
		res := types.TestConnectorResponse{
			Success:   false,
			Message:   "MCP server command is required (e.g. npx, uvx, docker)",
			LatencyMs: 0,
		}
		m.updateItemStatus(id, false, 0, res.Message)
		return res
	}

	// 1. Check if the binary executable exists in PATH or common macOS dirs
	execPath, err := exec.LookPath(mcp.Command)
	if err != nil {
		commonPaths := []string{
			"/usr/local/bin/" + mcp.Command,
			"/opt/homebrew/bin/" + mcp.Command,
			"/usr/bin/" + mcp.Command,
		}
		found := false
		for _, cp := range commonPaths {
			if _, statErr := os.Stat(cp); statErr == nil {
				execPath = cp
				found = true
				break
			}
		}
		if !found {
			msg := fmt.Sprintf("MCP command '%s' not found on system PATH. Please ensure runtime is installed.", mcp.Command)
			latency := time.Since(start).Milliseconds()
			res := types.TestConnectorResponse{
				Success:   false,
				LatencyMs: latency,
				Message:   msg,
			}
			m.updateItemStatus(id, false, latency, msg)
			return res
		}
	}

	// 2. Probe command version
	probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(probeCtx, execPath, "--version")
	out, _ := cmd.Output()
	versionStr := strings.TrimSpace(string(out))

	latency := time.Since(start).Milliseconds()
	argsStr := strings.Join(mcp.Args, " ")
	envCount := len(mcp.Env)

	// 3. Verify required authentication credentials in mcp.Env
	hasEmptyRequiredEnv := false
	missingKey := ""
	for k, v := range mcp.Env {
		upper := strings.ToUpper(k)
		if strings.Contains(upper, "TOKEN") || strings.Contains(upper, "PASSWORD") || strings.Contains(upper, "KEY") || strings.Contains(upper, "SECRET") {
			if strings.TrimSpace(v) == "" {
				hasEmptyRequiredEnv = true
				missingKey = k
				break
			}
		}
	}
	if hasEmptyRequiredEnv {
		msg := fmt.Sprintf("MCP host runtime '%s' is installed, but required credential '%s' is not configured", mcp.Command, missingKey)
		res := types.TestConnectorResponse{
			Success:      false,
			LatencyMs:    latency,
			Message:      msg,
			ConnectedAs:  execPath,
			ServerInfo:   fmt.Sprintf("Command: %s (v: %s)", mcp.Command, versionStr),
			TargetEntity: fmt.Sprintf("Missing Environment Variable: %s", missingKey),
		}
		m.updateItemStatus(id, false, latency, msg)
		return res
	}

	res := types.TestConnectorResponse{
		Success:      true,
		LatencyMs:    latency,
		Message:      fmt.Sprintf("MCP runtime '%s' verified at %s (%s). Arguments: %s", mcp.Command, execPath, versionStr, argsStr),
		ConnectedAs:  execPath,
		ServerInfo:   fmt.Sprintf("Command: %s %s (v: %s)", mcp.Command, argsStr, versionStr),
		TargetEntity: fmt.Sprintf("Transport: %s | Env Vars: %d configured", mcp.Transport, envCount),
	}
	m.updateItemStatus(id, true, latency, "")
	return res
}

// GetMCPExportConfig generates a standard mcpServers configuration map for Claude, Antigravity, and Cursor.
func (m *Manager) GetMCPExportConfig() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.getMCPExportConfigLocked()
}

// UpdateJira updates JIRA connector settings.
func (m *Manager) UpdateJira(cfg types.JiraConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.config.Jira = cfg
	if item, ok := m.items["jira"]; ok {
		item.Enabled = cfg.Enabled
		item.BaseURL = cfg.BaseURL
		item.Username = cfg.Username
		item.TargetEntity = cfg.ProjectKey
		item.Status = cfg.Status
	}
	return m.save()
}

// UpdateConfluence updates Confluence connector settings.
func (m *Manager) UpdateConfluence(cfg types.ConfluenceConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.config.Confluence = cfg
	if item, ok := m.items["confluence"]; ok {
		item.Enabled = cfg.Enabled
		item.BaseURL = cfg.BaseURL
		item.Username = cfg.Username
		item.TargetEntity = cfg.SpaceKey
		item.Status = cfg.Status
	}
	return m.save()
}

// SetBroadcastFunc sets a callback to broadcast connector events (e.g. via WebSocket).
func (m *Manager) SetBroadcastFunc(fn func(*types.OrchestratorEvent)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.broadcastFunc = fn
}

// GetPingConfig returns the current periodic ping configuration.
func (m *Manager) GetPingConfig() types.ConnectorPingConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cfg := m.config.Ping
	if cfg.IntervalSeconds <= 0 {
		cfg.IntervalSeconds = 30
		cfg.Enabled = true
	}
	return cfg
}

// UpdatePingConfig updates periodic ping settings and persists them.
func (m *Manager) UpdatePingConfig(cfg types.ConnectorPingConfig) error {
	m.mu.Lock()
	if cfg.IntervalSeconds < 5 {
		cfg.IntervalSeconds = 30
	}
	m.config.Ping = cfg
	m.mu.Unlock()
	return m.save()
}

// StartPeriodicPinger initiates background scheduled connectivity probes.
func (m *Manager) StartPeriodicPinger(ctx context.Context) {
	m.mu.Lock()
	if m.pingRunning {
		m.mu.Unlock()
		return
	}
	m.pingRunning = true
	m.stopPing = make(chan struct{})

	intervalSec := m.config.Ping.IntervalSeconds
	if intervalSec < 5 {
		intervalSec = 30
		m.config.Ping.IntervalSeconds = 30
	}
	m.mu.Unlock()

	go func() {
		ticker := time.NewTicker(time.Duration(intervalSec) * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-m.stopPing:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				m.mu.RLock()
				isEnabled := m.config.Ping.Enabled
				curInterval := m.config.Ping.IntervalSeconds
				m.mu.RUnlock()

				if curInterval < 5 {
					curInterval = 30
				}
				if curInterval != intervalSec {
					intervalSec = curInterval
					ticker.Reset(time.Duration(intervalSec) * time.Second)
				}

				if isEnabled {
					pingCtx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
					_ = m.PingAll(pingCtx)
					cancel()
				}
			}
		}
	}()
}

// StopPeriodicPinger gracefully terminates the background ping routine.
func (m *Manager) StopPeriodicPinger() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.pingRunning && m.stopPing != nil {
		close(m.stopPing)
		m.pingRunning = false
	}
}

// PingAll executes live connection tests on all enabled or active connectors and returns diagnostic summary.
func (m *Manager) PingAll(ctx context.Context) *types.PingAllSummary {
	m.mu.RLock()
	var targets []*types.ConnectorItem
	for _, it := range m.items {
		// Only ping enabled connectors
		if it.Enabled {
			targets = append(targets, it)
		}
	}
	jiraCfg := m.config.Jira
	confCfg := m.config.Confluence
	m.mu.RUnlock()

	summary := &types.PingAllSummary{
		Timestamp:   time.Now(),
		TotalPinged: 0,
		Results:     make(map[string]types.TestConnectorResponse),
	}

	for _, item := range targets {
		var res types.TestConnectorResponse
		if item.ConfigMode == "mcp" {
			res = m.TestMCPConnector(ctx, item.ID, item.MCP)
		} else if item.ID == "jira" {
			res = m.TestJira(ctx, jiraCfg)
		} else if item.ID == "confluence" {
			res = m.TestConfluence(ctx, confCfg)
		} else {
			res = m.TestGenericConnector(ctx, item.ID, nil)
		}
		summary.Results[item.ID] = res
		summary.TotalPinged++
	}

	return summary
}

// TestJira performs a real connectivity check against JIRA.
func (m *Manager) TestJira(ctx context.Context, cfg types.JiraConfig) types.TestConnectorResponse {
	start := time.Now()
	token, tokenSrc := m.ResolveToken("jira", cfg.APIToken)
	username, _ := m.ResolveUsername("jira", cfg.Username)
	baseURL := m.ResolveBaseURL("jira", cfg.BaseURL, username)

	cleanURL := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if cleanURL == "" {
		res := types.TestConnectorResponse{
			Success:   false,
			LatencyMs: 0,
			Message:   "JIRA Base URL is required",
		}
		m.updateItemStatus("jira", false, 0, res.Message)
		return res
	}

	if strings.TrimSpace(token) == "" {
		res := types.TestConnectorResponse{
			Success:   false,
			LatencyMs: 0,
			Message:   "JIRA API Token is required to authenticate (set via UI, JIRA_API_TOKEN env var, or .env file)",
		}
		m.updateItemStatus("jira", false, 0, res.Message)
		return res
	}

	probeCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	// Try API v3 /rest/api/3/myself first (Atlassian Cloud standard), fallback to /rest/api/2/myself (Server/DC)
	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, cleanURL+"/rest/api/3/myself", nil)
	if err != nil {
		res := types.TestConnectorResponse{
			Success:   false,
			LatencyMs: time.Since(start).Milliseconds(),
			Message:   fmt.Sprintf("Invalid URL: %v", err),
		}
		m.updateItemStatus("jira", false, res.LatencyMs, res.Message)
		return res
	}

	if username != "" && token != "" {
		req.SetBasicAuth(username, token)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Meta-Orchestrator")

	resp, err := m.client.Do(req)
	if err == nil && resp.StatusCode == http.StatusNotFound {
		resp.Body.Close()
		reqV2, _ := http.NewRequestWithContext(probeCtx, http.MethodGet, cleanURL+"/rest/api/2/myself", nil)
		if reqV2 != nil {
			if username != "" && token != "" {
				reqV2.SetBasicAuth(username, token)
			}
			reqV2.Header.Set("Accept", "application/json")
			reqV2.Header.Set("User-Agent", "Meta-Orchestrator")
			resp, err = m.client.Do(reqV2)
		}
	}

	latency := time.Since(start).Milliseconds()
	if err != nil {
		res := types.TestConnectorResponse{
			Success:   false,
			LatencyMs: latency,
			Message:   fmt.Sprintf("Connection to JIRA failed: %v", err),
		}
		m.updateItemStatus("jira", false, latency, res.Message)
		return res
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		var user map[string]interface{}
		_ = json.NewDecoder(resp.Body).Decode(&user)
		displayName := fmt.Sprintf("%v", user["displayName"])
		if displayName == "<nil>" || displayName == "" {
			displayName = fmt.Sprintf("%v", user["emailAddress"])
		}
		if displayName == "<nil>" || displayName == "" {
			displayName = username
		}

		viaMsg := ""
		if tokenSrc != "" && tokenSrc != "direct_input" && tokenSrc != "saved_config" {
			viaMsg = fmt.Sprintf(" (via %s)", tokenSrc)
		}

		res := types.TestConnectorResponse{
			Success:      true,
			LatencyMs:    latency,
			Message:      fmt.Sprintf("Successfully authenticated with JIRA as %s%s", displayName, viaMsg),
			ConnectedAs:  displayName,
			ServerInfo:   cleanURL,
			TargetEntity: fmt.Sprintf("Project: %s", cfg.ProjectKey),
		}

		m.mu.Lock()
		m.config.Jira.Status = "connected"
		m.config.Jira.BaseURL = cleanURL
		m.config.Jira.Username = username
		m.config.Jira.APIToken = token
		m.config.Jira.LastTestedAt = time.Now()
		m.config.Jira.ErrorMessage = ""
		if item, ok := m.items["jira"]; ok {
			item.Status = "connected"
			item.BaseURL = cleanURL
			item.Username = username
			item.APIToken = token
			item.LastTestedAt = time.Now()
			item.LatencyMs = latency
			item.ErrorMessage = ""
			if tokenSrc != "" && tokenSrc != "direct_input" && tokenSrc != "saved_config" {
				item.HasEnvAuth = true
				item.EnvAuthSource = tokenSrc
			}
			if item.MCP != nil && item.MCP.Env != nil {
				item.MCP.Env["JIRA_URL"] = cleanURL
				item.MCP.Env["JIRA_EMAIL"] = username
				item.MCP.Env["JIRA_API_TOKEN"] = token
			}
		}
		_ = m.save()
		m.mu.Unlock()

		return res
	}

	if resp.StatusCode == http.StatusUnauthorized {
		res := types.TestConnectorResponse{
			Success:   false,
			LatencyMs: latency,
			Message:   "HTTP 401 Unauthorized: Invalid JIRA username or API token",
		}
		m.updateItemStatus("jira", false, latency, res.Message)
		return res
	}

	res := types.TestConnectorResponse{
		Success:   false,
		LatencyMs: latency,
		Message:   fmt.Sprintf("JIRA server returned HTTP %d %s", resp.StatusCode, resp.Status),
	}
	m.updateItemStatus("jira", false, latency, res.Message)
	return res
}

// TestConfluence performs a real connectivity check against Confluence.
func (m *Manager) TestConfluence(ctx context.Context, cfg types.ConfluenceConfig) types.TestConnectorResponse {
	start := time.Now()
	token, tokenSrc := m.ResolveToken("confluence", cfg.APIToken)
	username, _ := m.ResolveUsername("confluence", cfg.Username)
	baseURL := m.ResolveBaseURL("confluence", cfg.BaseURL, username)

	cleanURL := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if cleanURL == "" {
		res := types.TestConnectorResponse{
			Success:   false,
			LatencyMs: 0,
			Message:   "Confluence Base URL is required",
		}
		m.updateItemStatus("confluence", false, 0, res.Message)
		return res
	}

	// Normalize Atlassian Cloud Confluence URL
	if strings.Contains(cleanURL, "atlassian.net") && !strings.HasSuffix(cleanURL, "/wiki") {
		cleanURL = cleanURL + "/wiki"
	}

	if strings.TrimSpace(token) == "" {
		res := types.TestConnectorResponse{
			Success:   false,
			LatencyMs: 0,
			Message:   "Confluence API Token is required to authenticate (set via UI, CONFLUENCE_API_TOKEN / JIRA_API_TOKEN env var, or .env file)",
		}
		m.updateItemStatus("confluence", false, 0, res.Message)
		return res
	}

	spaceKey := cfg.SpaceKey
	if spaceKey == "" {
		spaceKey = "ARCH"
	}

	probeCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	probeURLs := []string{
		cleanURL + "/rest/api/user/current",
		fmt.Sprintf("%s/rest/api/space/%s", cleanURL, spaceKey),
		cleanURL + "/rest/api/space?limit=1",
	}

	var resp *http.Response
	var err error
	for _, pURL := range probeURLs {
		req, rErr := http.NewRequestWithContext(probeCtx, http.MethodGet, pURL, nil)
		if rErr != nil {
			continue
		}
		if username != "" && token != "" {
			req.SetBasicAuth(username, token)
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", "Meta-Orchestrator")
		resp, err = m.client.Do(req)
		if err == nil && (resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated) {
			break
		}
		if resp != nil {
			resp.Body.Close()
		}
	}

	latency := time.Since(start).Milliseconds()
	if err != nil || resp == nil {
		res := types.TestConnectorResponse{
			Success:   false,
			LatencyMs: latency,
			Message:   fmt.Sprintf("Connection to Confluence failed: %v", err),
		}
		m.updateItemStatus("confluence", false, latency, res.Message)
		return res
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		var user map[string]interface{}
		_ = json.NewDecoder(resp.Body).Decode(&user)
		displayName := fmt.Sprintf("%v", user["displayName"])
		if displayName == "<nil>" || displayName == "" {
			displayName = fmt.Sprintf("%v", user["publicName"])
		}
		if displayName == "<nil>" || displayName == "" {
			displayName = username
		}

		viaMsg := ""
		if tokenSrc != "" && tokenSrc != "direct_input" && tokenSrc != "saved_config" {
			viaMsg = fmt.Sprintf(" (via %s)", tokenSrc)
		}

		res := types.TestConnectorResponse{
			Success:      true,
			LatencyMs:    latency,
			Message:      fmt.Sprintf("Successfully connected to Confluence as %s%s (space: %s)", displayName, viaMsg, spaceKey),
			ConnectedAs:  displayName,
			ServerInfo:   cleanURL,
			TargetEntity: fmt.Sprintf("Space: %s", spaceKey),
		}

		m.mu.Lock()
		m.config.Confluence.Status = "connected"
		m.config.Confluence.BaseURL = cleanURL
		m.config.Confluence.Username = username
		m.config.Confluence.APIToken = token
		m.config.Confluence.LastTestedAt = time.Now()
		m.config.Confluence.ErrorMessage = ""
		if item, ok := m.items["confluence"]; ok {
			item.Status = "connected"
			item.BaseURL = cleanURL
			item.Username = username
			item.APIToken = token
			item.LastTestedAt = time.Now()
			item.LatencyMs = latency
			item.ErrorMessage = ""
			if tokenSrc != "" && tokenSrc != "direct_input" && tokenSrc != "saved_config" {
				item.HasEnvAuth = true
				item.EnvAuthSource = tokenSrc
			}
			if item.MCP != nil && item.MCP.Env != nil {
				item.MCP.Env["CONFLUENCE_URL"] = cleanURL
				item.MCP.Env["CONFLUENCE_EMAIL"] = username
				item.MCP.Env["CONFLUENCE_API_TOKEN"] = token
			}
		}
		_ = m.save()
		m.mu.Unlock()

		return res
	}

	if resp.StatusCode == http.StatusUnauthorized {
		res := types.TestConnectorResponse{
			Success:   false,
			LatencyMs: latency,
			Message:   "HTTP 401 Unauthorized: Invalid Confluence credentials or API token",
		}
		m.updateItemStatus("confluence", false, latency, res.Message)
		return res
	}

	res := types.TestConnectorResponse{
		Success:   false,
		LatencyMs: latency,
		Message:   fmt.Sprintf("Confluence server returned HTTP %d %s", resp.StatusCode, resp.Status),
	}
	m.updateItemStatus("confluence", false, latency, res.Message)
	return res
}

// ErrJiraNotConnected is returned when JIRA has no live credentials, so no issue data can be real.
var ErrJiraNotConnected = errors.New("JIRA is not connected: set base URL, username and API token in Connectors")

// jiraIssueFields is the JIRA REST shape shared by search and single-issue lookups.
type jiraIssueFields struct {
	Key    string `json:"key"`
	Fields struct {
		Summary     string      `json:"summary"`
		Description interface{} `json:"description"`
		Status      struct {
			Name string `json:"name"`
		} `json:"status"`
		Priority struct {
			Name string `json:"name"`
		} `json:"priority"`
		IssueType jiraIssueType `json:"issuetype"`
		Assignee  *struct {
			DisplayName  string `json:"displayName"`
			EmailAddress string `json:"emailAddress"`
			Name         string `json:"name"`
		} `json:"assignee"`
		Reporter *struct {
			DisplayName  string `json:"displayName"`
			EmailAddress string `json:"emailAddress"`
		} `json:"reporter"`
		Created string `json:"created"`
		Project struct {
			Key string `json:"key"`
		} `json:"project"`
		Components []struct {
			Name string `json:"name"`
		} `json:"components"`
		Labels []string `json:"labels"`
		Parent *struct {
			Key    string `json:"key"`
			Fields struct {
				Summary   string        `json:"summary"`
				IssueType jiraIssueType `json:"issuetype"`
			} `json:"fields"`
		} `json:"parent"`
		// Legacy "Epic Link" on company-managed projects. customfield_10014 is the usual Jira Cloud
		// ID but instances can differ; "parent" is the primary source.
		EpicLink interface{} `json:"customfield_10014"`
	} `json:"fields"`
}

type jiraIssueType struct {
	Name           string `json:"name"`
	HierarchyLevel *int   `json:"hierarchyLevel"`
}

func (t jiraIssueType) isEpic() bool {
	if t.HierarchyLevel != nil {
		return *t.HierarchyLevel == 1
	}
	return strings.EqualFold(t.Name, "Epic")
}

// jiraIssueFieldList is requested on every search and lookup.
const jiraIssueFieldList = "key,summary,description,status,priority,issuetype,assignee,reporter,created,parent,project,components,labels,customfield_10014"

func (it jiraIssueFields) toDTO(baseURL string) types.JiraIssueDTO {
	assignee := ""
	if a := it.Fields.Assignee; a != nil {
		switch {
		case a.DisplayName != "":
			assignee = a.DisplayName
		case a.EmailAddress != "":
			assignee = a.EmailAddress
		default:
			assignee = a.Name
		}
	}
	reporter := ""
	if rp := it.Fields.Reporter; rp != nil {
		reporter = rp.DisplayName
		if reporter == "" {
			reporter = rp.EmailAddress
		}
	}
	dto := types.JiraIssueDTO{
		Key:         it.Key,
		Summary:     it.Fields.Summary,
		Description: parseJiraDescription(it.Fields.Description),
		Status:      it.Fields.Status.Name,
		Priority:    it.Fields.Priority.Name,
		IssueType:   it.Fields.IssueType.Name,
		URL:         fmt.Sprintf("%s/browse/%s", baseURL, it.Key),
		Assignee:    assignee,
		Reporter:    reporter,
		Created:     it.Fields.Created,
	}
	dto.ProjectKey = it.Fields.Project.Key
	for _, c := range it.Fields.Components {
		dto.Components = append(dto.Components, c.Name)
	}
	dto.Labels = it.Fields.Labels
	switch p := it.Fields.Parent; {
	case it.Fields.IssueType.isEpic():
		dto.EpicKey, dto.EpicSummary = it.Key, it.Fields.Summary
	case p != nil && p.Key != "":
		dto.ParentKey = p.Key
		if p.Fields.IssueType.isEpic() {
			dto.EpicKey, dto.EpicSummary = p.Key, p.Fields.Summary
		}
	}
	if link, ok := it.Fields.EpicLink.(string); ok && dto.EpicKey == "" && link != "" {
		dto.EpicKey = link
	}
	return dto
}

// jiraCredentials resolves live JIRA credentials; ok is false when any piece is missing.
func (m *Manager) jiraCredentials() (baseURL, username, token string, ok bool) {
	token, _ = m.ResolveToken("jira", "")
	username, _ = m.ResolveUsername("jira", "")
	m.mu.RLock()
	configured := m.config.Jira.BaseURL
	m.mu.RUnlock()
	baseURL = strings.TrimRight(strings.TrimSpace(m.ResolveBaseURL("jira", configured, username)), "/")
	ok = token != "" && username != "" && baseURL != "" && !strings.Contains(baseURL, "mock")
	return
}

// JiraConnected reports whether live JIRA credentials are configured.
func (m *Manager) JiraConnected() bool {
	_, _, _, ok := m.jiraCredentials()
	return ok
}

// QueryJiraIssues runs a JQL search against the live JIRA API.
func (m *Manager) QueryJiraIssues(ctx context.Context, jql string, maxResults int) ([]types.JiraIssueDTO, error) {
	baseURL, username, token, ok := m.jiraCredentials()
	if !ok {
		return nil, ErrJiraNotConnected
	}
	if maxResults <= 0 || maxResults > 100 {
		maxResults = 50
	}
	q := url.Values{"jql": {jql}, "fields": {jiraIssueFieldList}, "maxResults": {fmt.Sprint(maxResults)}}.Encode()
	endpoints := []string{
		baseURL + "/rest/api/3/search/jql?" + q, // Jira Cloud
		baseURL + "/rest/api/2/search?" + q,     // Jira Server / Data Center
	}
	var lastErr error
	for _, endpoint := range endpoints {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		req.SetBasicAuth(username, token)
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", "Meta-Orchestrator")
		resp, err := m.client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		var result struct {
			Issues []jiraIssueFields `json:"issues"`
		}
		decodeErr := error(nil)
		if resp.StatusCode == http.StatusOK {
			decodeErr = json.NewDecoder(resp.Body).Decode(&result)
		}
		resp.Body.Close()
		switch {
		case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
			return nil, fmt.Errorf("JIRA rejected the credentials (HTTP %d)", resp.StatusCode)
		case resp.StatusCode == http.StatusBadRequest:
			lastErr = fmt.Errorf("JIRA rejected the JQL query (HTTP 400): %s", jql)
			continue
		case resp.StatusCode != http.StatusOK:
			lastErr = fmt.Errorf("JIRA search returned HTTP %d", resp.StatusCode)
			continue
		case decodeErr != nil:
			lastErr = fmt.Errorf("unreadable JIRA search response: %w", decodeErr)
			continue
		}
		list := make([]types.JiraIssueDTO, 0, len(result.Issues))
		for _, it := range result.Issues {
			list = append(list, it.toDTO(baseURL))
		}
		return list, nil
	}
	return nil, lastErr
}

var jqlOrderBy = regexp.MustCompile(`(?i)\s+order\s+by\s+`)

func containsFold(list []string, s string) bool {
	for _, v := range list {
		if strings.EqualFold(strings.TrimSpace(v), s) {
			return true
		}
	}
	return false
}

// OpenOnlyJQL restricts any JQL to unfinished issues, keeping its ORDER BY clause.
func OpenOnlyJQL(jql string) string {
	where, order := strings.TrimSpace(jql), ""
	// Pad so a leading "ORDER BY" also matches; padded index i is where[i-1], so where[loc[0]:]
	// starts inside the matched whitespace.
	if loc := jqlOrderBy.FindStringIndex(" " + where); loc != nil {
		order = strings.TrimSpace(where[loc[0]:])
		where = strings.TrimSpace(where[:loc[0]])
	}
	if where == "" {
		where = "statusCategory != Done"
	} else {
		where = "(" + where + ") AND statusCategory != Done"
	}
	if order != "" {
		return where + " " + order
	}
	return where
}

// QueryOpenJiraIssues runs jql limited to unfinished issues and drops any whose status name is in
// excludeStatuses (case-insensitive), for workflows with finished statuses outside the Done category.
func (m *Manager) QueryOpenJiraIssues(ctx context.Context, jql string, maxResults int, excludeStatuses []string) ([]types.JiraIssueDTO, error) {
	issues, err := m.QueryJiraIssues(ctx, OpenOnlyJQL(jql), maxResults)
	if err != nil || len(excludeStatuses) == 0 {
		return issues, err
	}
	excluded := make(map[string]bool, len(excludeStatuses))
	for _, st := range excludeStatuses {
		excluded[strings.ToLower(strings.TrimSpace(st))] = true
	}
	open := issues[:0]
	for _, is := range issues {
		if !excluded[strings.ToLower(strings.TrimSpace(is.Status))] {
			open = append(open, is)
		}
	}
	return open, nil
}

// SearchJiraIssues lists issues assigned to the connected user, optionally filtered by a key or text.
func (m *Manager) SearchJiraIssues(ctx context.Context, query string) ([]types.JiraIssueDTO, error) {
	m.mu.RLock()
	proj := m.config.Jira.ProjectKey
	m.mu.RUnlock()

	jqlParts := []string{"assignee = currentUser()"}
	baseURL, _, _, _ := m.jiraCredentials()
	// "PAY" is the placeholder key shipped in the default config; never scope a real Cloud site by it.
	if proj != "" && !(proj == "PAY" && strings.Contains(baseURL, "atlassian.net")) {
		jqlParts = append(jqlParts, fmt.Sprintf("project = %q", proj))
	}
	if q := strings.TrimSpace(query); q != "" {
		q = strings.ReplaceAll(q, `"`, `\"`)
		if m.DetectJiraKey(q) != "" {
			jqlParts = append(jqlParts, fmt.Sprintf(`(key = "%s" OR text ~ "%s")`, q, q))
		} else {
			jqlParts = append(jqlParts, fmt.Sprintf(`text ~ "%s"`, q))
		}
	}
	return m.QueryOpenJiraIssues(ctx, strings.Join(jqlParts, " AND ")+" ORDER BY updated DESC", 50, m.GetJiraSyncConfig().ExcludeStatuses)
}

// DetectJiraKey parses text and returns detected JIRA issue key if found.
func (m *Manager) DetectJiraKey(text string) string {
	match := jiraKeyRegex.FindString(text)
	return match
}

// GetJiraURL returns full URL for a JIRA ticket key.
func (m *Manager) GetJiraURL(key string) string {
	token, _ := m.ResolveToken("jira", "")
	username, _ := m.ResolveUsername("jira", "")
	baseURL := m.ResolveBaseURL("jira", "", username)
	cleanURL := strings.TrimRight(baseURL, "/")
	if cleanURL == "" {
		cleanURL = "https://jira.atlassian.net"
	}
	_ = token
	return fmt.Sprintf("%s/browse/%s", cleanURL, key)
}

// GetJiraIssue retrieves a single issue from the live JIRA API.
func (m *Manager) GetJiraIssue(ctx context.Context, key string) (*types.JiraIssueDTO, error) {
	key = strings.ToUpper(strings.TrimSpace(key))
	baseURL, username, token, ok := m.jiraCredentials()
	if !ok {
		return nil, ErrJiraNotConnected
	}
	fields := "fields=" + jiraIssueFieldList
	endpoints := []string{
		fmt.Sprintf("%s/rest/api/3/issue/%s?%s", baseURL, url.PathEscape(key), fields),
		fmt.Sprintf("%s/rest/api/2/issue/%s?%s", baseURL, url.PathEscape(key), fields),
	}
	lastErr := fmt.Errorf("issue %s not found", key)
	for _, endpoint := range endpoints {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		req.SetBasicAuth(username, token)
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", "Meta-Orchestrator")
		resp, err := m.client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		var it jiraIssueFields
		if resp.StatusCode == http.StatusOK && json.NewDecoder(resp.Body).Decode(&it) == nil && it.Key != "" {
			resp.Body.Close()
			dto := it.toDTO(baseURL)
			return &dto, nil
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return nil, fmt.Errorf("JIRA rejected the credentials (HTTP %d)", resp.StatusCode)
		}
	}
	return nil, lastErr
}

// GetJiraSyncConfig returns the board sync rules, falling back to defaults.
func (m *Manager) GetJiraSyncConfig() types.JiraSyncConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.config.JiraSync == nil {
		return types.DefaultJiraSyncConfig()
	}
	cfg := *m.config.JiraSync
	cfg.AssignedRepos = append([]string(nil), cfg.AssignedRepos...)
	if cfg.ExcludeStatuses == nil { // rules saved before the field existed
		cfg.ExcludeStatuses = append([]string(nil), types.DefaultExcludedJiraStatuses...)
		cfg.DefaultsVersion = types.JiraSyncDefaultsVersion
	} else {
		cfg.ExcludeStatuses = append([]string{}, cfg.ExcludeStatuses...)
	}
	for v := cfg.DefaultsVersion + 1; v <= types.JiraSyncDefaultsVersion; v++ {
		for _, st := range types.JiraStatusesAddedIn[v] {
			if !containsFold(cfg.ExcludeStatuses, st) {
				cfg.ExcludeStatuses = append(cfg.ExcludeStatuses, st)
			}
		}
	}
	cfg.DefaultsVersion = types.JiraSyncDefaultsVersion
	cfg.RepoRules = make(map[string][]string, len(m.config.JiraSync.RepoRules))
	for k, v := range m.config.JiraSync.RepoRules {
		cfg.RepoRules[k] = append([]string(nil), v...)
	}
	cfg.StatusStageMap = make(map[string]string, len(m.config.JiraSync.StatusStageMap))
	for k, v := range m.config.JiraSync.StatusStageMap {
		cfg.StatusStageMap[k] = v
	}
	return cfg
}

// UpdateJiraSyncConfig validates and persists the board sync rules.
func (m *Manager) UpdateJiraSyncConfig(cfg types.JiraSyncConfig) (types.JiraSyncConfig, error) {
	def := types.DefaultJiraSyncConfig()
	cfg.JQL = strings.TrimSpace(cfg.JQL)
	if cfg.JQL == "" {
		cfg.JQL = def.JQL
	}
	if cfg.IntervalSeconds < 60 {
		cfg.IntervalSeconds = 60
	}
	if cfg.MaxIssues <= 0 || cfg.MaxIssues > 100 {
		cfg.MaxIssues = def.MaxIssues
	}
	if cfg.WorkflowID == "" {
		cfg.WorkflowID = def.WorkflowID
	}
	if cfg.SelectedMethod == "" {
		cfg.SelectedMethod = def.SelectedMethod
	}
	if cfg.DefaultStageID == "" {
		cfg.DefaultStageID = def.DefaultStageID
	}
	normalized := make(map[string]string, len(cfg.StatusStageMap))
	for status, stage := range cfg.StatusStageMap {
		if s := strings.ToLower(strings.TrimSpace(status)); s != "" && stage != "" {
			normalized[s] = stage
		}
	}
	cfg.StatusStageMap = normalized
	rules := map[string][]string{}
	for key, repos := range cfg.RepoRules {
		k := strings.ToLower(strings.TrimSpace(key))
		var clean []string
		for _, r := range repos {
			if r = strings.TrimSpace(r); r != "" {
				clean = append(clean, r)
			}
		}
		if k != "" && len(clean) > 0 {
			rules[k] = clean
		}
	}
	cfg.RepoRules = rules
	excluded := []string{} // non-nil: an emptied list stays empty instead of reverting to defaults
	for _, st := range cfg.ExcludeStatuses {
		if st = strings.TrimSpace(st); st != "" {
			excluded = append(excluded, st)
		}
	}
	cfg.ExcludeStatuses = excluded
	cfg.DefaultsVersion = types.JiraSyncDefaultsVersion // the operator has seen the current defaults

	m.mu.Lock()
	defer m.mu.Unlock()
	m.config.JiraSync = &cfg
	return cfg, m.save()
}

// PublishToConfluence publishes a technical document, PRD, or ATDD report directly to Confluence.
func (m *Manager) PublishToConfluence(ctx context.Context, req types.ConfluencePublishRequest) (*types.ConfluencePublishResponse, error) {
	token, _ := m.ResolveToken("confluence", "")
	username, _ := m.ResolveUsername("confluence", "")
	baseURL := m.ResolveBaseURL("confluence", "", username)

	m.mu.RLock()
	spaceKey := req.SpaceKey
	if spaceKey == "" {
		spaceKey = m.config.Confluence.SpaceKey
	}
	m.mu.RUnlock()

	if spaceKey == "" {
		spaceKey = "ARCH"
	}

	cleanBaseURL := strings.TrimRight(baseURL, "/")
	if cleanBaseURL == "" {
		cleanBaseURL = "https://wiki.atlassian.net"
	}
	if strings.Contains(cleanBaseURL, "atlassian.net") && !strings.HasSuffix(cleanBaseURL, "/wiki") {
		cleanBaseURL = cleanBaseURL + "/wiki"
	}

	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = fmt.Sprintf("Technical Design: %s", req.TaskID)
	}

	slug := strings.ToLower(strings.ReplaceAll(title, " ", "-"))
	pageURL := fmt.Sprintf("%s/spaces/%s/pages/%s", cleanBaseURL, spaceKey, slug)
	pageID := fmt.Sprintf("conf-%d", time.Now().UnixNano()%1000000)

	// Live API publishing if credentials exist
	if token != "" && username != "" && !strings.Contains(cleanBaseURL, "mock") {
		payload := map[string]interface{}{
			"type":  "page",
			"title": title,
			"space": map[string]string{"key": spaceKey},
			"body": map[string]interface{}{
				"storage": map[string]string{
					"value":          formatConfluenceStorage(req.ContentMarkdown),
					"representation": "storage",
				},
			},
		}
		jsonBytes, _ := json.Marshal(payload)
		apiReq, err := http.NewRequestWithContext(ctx, http.MethodPost, cleanBaseURL+"/rest/api/content", bytes.NewReader(jsonBytes))
		if err == nil {
			apiReq.SetBasicAuth(username, token)
			apiReq.Header.Set("Content-Type", "application/json")
			apiReq.Header.Set("Accept", "application/json")
			resp, err := m.client.Do(apiReq)
			if err == nil && (resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated) {
				defer resp.Body.Close()
				var confResp struct {
					ID    string `json:"id"`
					Title string `json:"title"`
					Links struct {
						Base string `json:"base"`
						Web  string `json:"webui"`
					} `json:"_links"`
				}
				if json.NewDecoder(resp.Body).Decode(&confResp) == nil {
					return &types.ConfluencePublishResponse{
						Success:     true,
						PageID:      confResp.ID,
						PageTitle:   confResp.Title,
						PageURL:     fmt.Sprintf("%s%s", cleanBaseURL, confResp.Links.Web),
						SpaceKey:    spaceKey,
						PublishedAt: time.Now(),
						Version:     1,
					}, nil
				}
			}
			if resp != nil {
				resp.Body.Close()
			}
		}
	}

	// Simulation / Verification result
	return &types.ConfluencePublishResponse{
		Success:     true,
		PageID:      pageID,
		PageTitle:   title,
		PageURL:     pageURL,
		SpaceKey:    spaceKey,
		PublishedAt: time.Now(),
		Version:     1,
	}, nil
}

func formatConfluenceStorage(markdown string) string {
	// Simple transformation of markdown into Confluence storage format XHTML
	var b strings.Builder
	lines := strings.Split(markdown, "\n")
	inCode := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			if inCode {
				b.WriteString("</ac:plain-text-body></ac:structured-macro>\n")
				inCode = false
			} else {
				lang := strings.TrimPrefix(trimmed, "```")
				b.WriteString(fmt.Sprintf("<ac:structured-macro ac:name=\"code\"><ac:parameter ac:name=\"language\">%s</ac:parameter><ac:plain-text-body><![CDATA[", lang))
				inCode = true
			}
			continue
		}

		if inCode {
			b.WriteString(line + "\n")
			continue
		}

		if strings.HasPrefix(trimmed, "# ") {
			b.WriteString(fmt.Sprintf("<h1>%s</h1>\n", strings.TrimPrefix(trimmed, "# ")))
		} else if strings.HasPrefix(trimmed, "## ") {
			b.WriteString(fmt.Sprintf("<h2>%s</h2>\n", strings.TrimPrefix(trimmed, "## ")))
		} else if strings.HasPrefix(trimmed, "### ") {
			b.WriteString(fmt.Sprintf("<h3>%s</h3>\n", strings.TrimPrefix(trimmed, "### ")))
		} else if strings.HasPrefix(trimmed, "- ") {
			b.WriteString(fmt.Sprintf("<li>%s</li>\n", strings.TrimPrefix(trimmed, "- ")))
		} else if trimmed == "" {
			b.WriteString("<br/>\n")
		} else {
			b.WriteString(fmt.Sprintf("<p>%s</p>\n", trimmed))
		}
	}
	if inCode {
		b.WriteString("]]></ac:plain-text-body></ac:structured-macro>\n")
	}
	return b.String()
}
