package types

import "time"

// ConnectorType defines the type of external connector.
type ConnectorType string

const (
	ConnectorTypeJira       ConnectorType = "jira"
	ConnectorTypeConfluence ConnectorType = "confluence"
	ConnectorTypeGitHub     ConnectorType = "github"
	ConnectorTypeBitbucket  ConnectorType = "bitbucket"
	ConnectorTypeSlack      ConnectorType = "slack"
)

// MCPConfig defines Model Context Protocol server configuration for a connector.
type MCPConfig struct {
	Enabled     bool              `json:"enabled"`
	Command     string            `json:"command"`                // e.g. "npx", "uvx", "docker", "/path/to/binary"
	Args        []string          `json:"args"`                   // e.g. ["-y", "@modelcontextprotocol/server-github"]
	Env         map[string]string `json:"env,omitempty"`          // Environment variables (e.g. API tokens)
	Transport   string            `json:"transport"`              // "stdio", "sse", "streamable_http"
	EndpointURL string            `json:"endpoint_url,omitempty"` // For SSE/HTTP transports
}

// JiraConfig holds settings and credentials for JIRA integration.
type JiraConfig struct {
	Enabled        bool      `json:"enabled"`
	BaseURL        string    `json:"base_url"` // e.g. https://company.atlassian.net
	Username       string    `json:"username"` // Email or username
	APIToken       string    `json:"api_token"`
	ProjectKey     string    `json:"project_key"` // e.g. "PROJ", "TASK"
	JQLFilter      string    `json:"jql_filter"`  // Default query for syncing
	AutoDetectKeys bool      `json:"auto_detect_keys"`
	AutoSyncStatus bool      `json:"auto_sync_status"` // Push stage transitions to JIRA
	LastTestedAt   time.Time `json:"last_tested_at,omitempty"`
	Status         string    `json:"status"` // "connected", "configured", "error", "unconfigured"
	ErrorMessage   string    `json:"error_message,omitempty"`
}

// ConfluenceConfig holds settings and credentials for Confluence integration.
type ConfluenceConfig struct {
	Enabled             bool      `json:"enabled"`
	BaseURL             string    `json:"base_url"` // e.g. https://company.atlassian.net/wiki
	Username            string    `json:"username"`
	APIToken            string    `json:"api_token"`
	SpaceKey            string    `json:"space_key"` // e.g. "ARCH", "ENG", "SDLC"
	ParentPageID        string    `json:"parent_page_id,omitempty"`
	AutoPublishTechDocs bool      `json:"auto_publish_tech_docs"` // Publish RFC on generation
	AutoPublishPRD      bool      `json:"auto_publish_prd"`
	LastTestedAt        time.Time `json:"last_tested_at,omitempty"`
	Status              string    `json:"status"` // "connected", "configured", "error", "unconfigured"
	ErrorMessage        string    `json:"error_message,omitempty"`
}

// ConnectorCategory categorizes connectors for catalog discovery and filtering.
type ConnectorCategory string

const (
	ConnectorCategoryIssueTracker  ConnectorCategory = "issue_tracker"
	ConnectorCategoryDocumentation ConnectorCategory = "documentation"
	ConnectorCategoryChatOps       ConnectorCategory = "chatops"
	ConnectorCategoryVCS           ConnectorCategory = "vcs"
	ConnectorCategoryCustom        ConnectorCategory = "custom"
)

// ConnectorItem represents any third-party connector in the modular catalog.
type ConnectorItem struct {
	ID            string                 `json:"id"`             // e.g. "jira", "confluence", "github", "gitlab", "bitbucket", "slack", "linear", "notion"
	Name          string                 `json:"name"`           // e.g. "Atlassian JIRA"
	Category      ConnectorCategory      `json:"category"`       // e.g. "issue_tracker"
	CategoryLabel string                 `json:"category_label"` // e.g. "Issue Tracking & Agile"
	Description   string                 `json:"description"`
	Icon          string                 `json:"icon"`  // icon identifier e.g. "jira", "confluence", "github", "gitlab", "bitbucket", "slack", "linear", "notion", "webhook"
	Color         string                 `json:"color"` // UI accent color
	Enabled       bool                   `json:"enabled"`
	ConfigMode    string                 `json:"config_mode"` // "rest", "mcp", "hybrid"
	Status        string                 `json:"status"`      // "connected", "configured", "disabled", "unconfigured", "error"
	BaseURL       string                 `json:"base_url"`
	Username      string                 `json:"username"`
	APIToken      string                 `json:"api_token"`
	TargetEntity  string                 `json:"target_entity"` // ProjectKey, SpaceKey, Channel, Repo
	TargetLabel   string                 `json:"target_label"`  // e.g. "Default Project Key", "Documentation Space"
	Capabilities  []string               `json:"capabilities"`  // e.g. ["Ticket Auto-Detect", "Issue Import", "Status Sync"]
	ExtraSettings map[string]interface{} `json:"extra_settings,omitempty"`
	MCP           *MCPConfig             `json:"mcp,omitempty"` // MCP server integration
	LastTestedAt  time.Time              `json:"last_tested_at,omitempty"`
	LatencyMs     int64                  `json:"latency_ms,omitempty"`
	ErrorMessage  string                 `json:"error_message,omitempty"`
	HasEnvAuth    bool                   `json:"has_env_auth,omitempty"`
	EnvAuthSource string                 `json:"env_auth_source,omitempty"`
}

// ToggleConnectorRequest represents a quick toggle enable/disable command.
type ToggleConnectorRequest struct {
	Enabled bool `json:"enabled"`
}

// ConnectorPingConfig controls automatic periodic pinging of external connectors.
type ConnectorPingConfig struct {
	Enabled         bool `json:"enabled"`
	IntervalSeconds int  `json:"interval_seconds"` // Default e.g. 30
}

// ConnectorStatusEvent is emitted over WebSocket when periodic ping or manual test completes.
type ConnectorStatusEvent struct {
	ConnectorID  string        `json:"connector_id"`
	Status       string        `json:"status"`
	LatencyMs    int64         `json:"latency_ms"`
	LastTestedAt time.Time     `json:"last_tested_at"`
	ErrorMessage string        `json:"error_message,omitempty"`
	Item         ConnectorItem `json:"item"`
}

// PingAllSummary represents the results of a full connectors ping cycle.
type PingAllSummary struct {
	Timestamp   time.Time                        `json:"timestamp"`
	TotalPinged int                              `json:"total_pinged"`
	Results     map[string]TestConnectorResponse `json:"results"`
}

// ConnectorsConfig represents all integrated third-party connectors.
type ConnectorsConfig struct {
	Jira       JiraConfig          `json:"jira"`
	Confluence ConfluenceConfig    `json:"confluence"`
	Items      []*ConnectorItem    `json:"items,omitempty"`
	Ping       ConnectorPingConfig `json:"ping"`
	JiraSync   *JiraSyncConfig     `json:"jira_sync,omitempty"` // nil → DefaultJiraSyncConfig
}

// JiraSyncConfig holds the rules that keep the Kanban board in sync with JIRA.
type JiraSyncConfig struct {
	Enabled         bool              `json:"enabled"`
	JQL             string            `json:"jql"`              // issues that belong on the board
	IntervalSeconds int               `json:"interval_seconds"` // background sync period
	MaxIssues       int               `json:"max_issues"`
	WorkflowID      string            `json:"workflow_id"`
	SelectedMethod  string            `json:"selected_method"`
	AssignedRepos   []string          `json:"assigned_repos"`
	DefaultStageID  string            `json:"default_stage_id"`
	StatusStageMap  map[string]string `json:"status_stage_map,omitempty"` // JIRA status (case-insensitive) → starting stage
	UpdateExisting  bool              `json:"update_existing"`            // refresh title/description/status of linked tasks
	// ExcludeStatuses drops issues in these JIRA statuses (case-insensitive) on top of the enforced
	// statusCategory != Done, for workflows whose "finished" statuses sit outside the Done category.
	ExcludeStatuses []string `json:"exclude_statuses"`
}

// DefaultExcludedJiraStatuses are status names treated as finished work.
var DefaultExcludedJiraStatuses = []string{"Done", "Finish", "Finished", "Closed", "Resolved", "Cancelled", "Canceled", "Won't Do"}

// DefaultJiraSyncConfig syncs open issues assigned to the connected user into the first stage.
func DefaultJiraSyncConfig() JiraSyncConfig {
	return JiraSyncConfig{
		Enabled:         true,
		JQL:             "assignee = currentUser() AND statusCategory != Done ORDER BY updated DESC",
		IntervalSeconds: 300,
		MaxIssues:       50,
		WorkflowID:      "general_ai_sdlc",
		SelectedMethod:  "BMAD",
		DefaultStageID:  "prd_discovery",
		StatusStageMap:  map[string]string{},
		UpdateExisting:  true,
		ExcludeStatuses: append([]string(nil), DefaultExcludedJiraStatuses...),
	}
}

// JiraSyncStatus reports the outcome of the most recent board sync.
type JiraSyncStatus struct {
	Connected   bool      `json:"connected"` // live JIRA credentials are configured
	Running     bool      `json:"running"`
	LastRunAt   time.Time `json:"last_run_at,omitempty"`
	LastError   string    `json:"last_error,omitempty"`
	Fetched     int       `json:"fetched"`
	Created     int       `json:"created"`
	Updated     int       `json:"updated"`
	Skipped     int       `json:"skipped"`   // issues the operator removed from the board
	Dismissed   int       `json:"dismissed"` // total dismissed issue keys
	NextRunAt   time.Time `json:"next_run_at,omitempty"`
	LinkedTasks int       `json:"linked_tasks"`
}

// JiraSyncSettings is the payload served by the board sync settings endpoint.
type JiraSyncSettings struct {
	Config JiraSyncConfig `json:"config"`
	Status JiraSyncStatus `json:"status"`
}

// JiraIssueDTO represents an issue fetched or imported from JIRA.
type JiraIssueDTO struct {
	Key         string `json:"key"` // e.g. PROJ-1042
	Summary     string `json:"summary"`
	Description string `json:"description"`
	Status      string `json:"status"` // "To Do", "In Progress", "In Review", "Done"
	Priority    string `json:"priority"`
	IssueType   string `json:"issue_type"` // "Story", "Bug", "Task", "Epic"
	URL         string `json:"url"`
	Reporter    string `json:"reporter,omitempty"`
	Assignee    string `json:"assignee,omitempty"`
	Created     string `json:"created,omitempty"`
	ParentKey   string `json:"parent_key,omitempty"`   // direct parent (epic for stories, story for sub-tasks)
	EpicKey     string `json:"epic_key,omitempty"`     // owning epic; the issue's own key when it is an epic
	EpicSummary string `json:"epic_summary,omitempty"` // owning epic's summary
}

// ImportJiraIssueRequest represents parameters to import a JIRA ticket into a Kanban task.
type ImportJiraIssueRequest struct {
	IssueKey       string            `json:"issue_key"`
	WorkflowID     string            `json:"workflow_id"`
	SelectedMethod string            `json:"selected_method"`
	AssignedRepos  []string          `json:"assigned_repos"`
	StartStageID   string            `json:"start_stage_id,omitempty"`
	ActiveSlice    *StageSlice       `json:"active_slice,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

// ConfluencePublishRequest represents parameters to publish a document to Confluence.
type ConfluencePublishRequest struct {
	TaskID          string `json:"task_id"`
	DocType         string `json:"doc_type"` // "TECH_DOC_RFC", "PRD", "ARCHITECTURE", "ATDD"
	Title           string `json:"title"`
	ContentMarkdown string `json:"content_markdown"`
	SpaceKey        string `json:"space_key,omitempty"`
	ParentPageID    string `json:"parent_page_id,omitempty"`
}

// ConfluencePublishResponse represents the outcome of publishing to Confluence.
type ConfluencePublishResponse struct {
	Success     bool      `json:"success"`
	PageID      string    `json:"page_id"`
	PageTitle   string    `json:"page_title"`
	PageURL     string    `json:"page_url"`
	SpaceKey    string    `json:"space_key"`
	PublishedAt time.Time `json:"published_at"`
	Version     int       `json:"version"`
}

// TestConnectorRequest holds parameters to test connector connectivity.
type TestConnectorRequest struct {
	Type       ConnectorType     `json:"type"`
	Jira       *JiraConfig       `json:"jira,omitempty"`
	Confluence *ConfluenceConfig `json:"confluence,omitempty"`
}

// TestConnectorResponse holds diagnostic results of connection testing.
type TestConnectorResponse struct {
	Success      bool   `json:"success"`
	LatencyMs    int64  `json:"latency_ms"`
	Message      string `json:"message"`
	ConnectedAs  string `json:"connected_as,omitempty"`
	ServerInfo   string `json:"server_info,omitempty"`
	TargetEntity string `json:"target_entity,omitempty"`
}
