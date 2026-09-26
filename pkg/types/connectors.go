package types

import "time"

// ConnectorType defines the type of external connector.
type ConnectorType string

const (
	ConnectorTypeJira       ConnectorType = "jira"
	ConnectorTypeConfluence ConnectorType = "confluence"
	ConnectorTypeGitHub     ConnectorType = "github"
	ConnectorTypeSlack      ConnectorType = "slack"
)

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
	ID            string                 `json:"id"`             // e.g. "jira", "confluence", "github", "slack", "linear", "notion"
	Name          string                 `json:"name"`           // e.g. "Atlassian JIRA"
	Category      ConnectorCategory      `json:"category"`       // e.g. "issue_tracker"
	CategoryLabel string                 `json:"category_label"` // e.g. "Issue Tracking & Agile"
	Description   string                 `json:"description"`
	Icon          string                 `json:"icon"`           // icon identifier e.g. "jira", "confluence", "github", "slack", "linear", "notion", "webhook"
	Color         string                 `json:"color"`          // UI accent color
	Enabled       bool                   `json:"enabled"`
	Status        string                 `json:"status"`         // "connected", "configured", "disabled", "unconfigured", "error"
	BaseURL       string                 `json:"base_url"`
	Username      string                 `json:"username"`
	APIToken      string                 `json:"api_token"`
	TargetEntity  string                 `json:"target_entity"`  // ProjectKey, SpaceKey, Channel, Repo
	TargetLabel   string                 `json:"target_label"`   // e.g. "Default Project Key", "Documentation Space"
	Capabilities  []string               `json:"capabilities"`   // e.g. ["Ticket Auto-Detect", "Issue Import", "Status Sync"]
	ExtraSettings map[string]interface{} `json:"extra_settings,omitempty"`
	LastTestedAt  time.Time              `json:"last_tested_at,omitempty"`
	LatencyMs     int64                  `json:"latency_ms,omitempty"`
	ErrorMessage  string                 `json:"error_message,omitempty"`
}

// ToggleConnectorRequest represents a quick toggle enable/disable command.
type ToggleConnectorRequest struct {
	Enabled bool `json:"enabled"`
}

// ConnectorsConfig represents all integrated third-party connectors.
type ConnectorsConfig struct {
	Jira       JiraConfig       `json:"jira"`
	Confluence ConfluenceConfig `json:"confluence"`
	Items      []*ConnectorItem `json:"items,omitempty"`
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
