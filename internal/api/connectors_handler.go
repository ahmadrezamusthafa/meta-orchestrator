package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/connectors"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// handleConnectors returns the full connectors configuration including the extensible catalog.
func (r *Router) handleConnectors(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		r.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	cfg := r.cfg.ConnectorsManager.GetConfig()
	items := r.cfg.ConnectorsManager.ListConnectors()

	r.writeJSON(w, http.StatusOK, map[string]interface{}{
		"jira":       cfg.Jira,
		"confluence": cfg.Confluence,
		"items":      items,
	})
}

// handleConnectorCatalog returns all connectors in the catalog.
func (r *Router) handleConnectorCatalog(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		r.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	items := r.cfg.ConnectorsManager.ListConnectors()
	r.writeJSON(w, http.StatusOK, items)
}

// handleConnectorItemAction routes actions for a specific connector item (/api/v1/connectors/items/{id}...).
func (r *Router) handleConnectorItemAction(w http.ResponseWriter, req *http.Request) {
	subPath := strings.TrimPrefix(req.URL.Path, "/api/v1/connectors/items/")
	parts := strings.Split(strings.Trim(subPath, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		r.writeError(w, http.StatusBadRequest, "Connector ID is required")
		return
	}

	id := parts[0]

	// 1. POST /api/v1/connectors/items/{id}/toggle
	if len(parts) == 2 && parts[1] == "toggle" {
		if req.Method != http.MethodPost && req.Method != http.MethodPatch {
			r.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}
		var toggleReq types.ToggleConnectorRequest
		if err := json.NewDecoder(req.Body).Decode(&toggleReq); err != nil {
			r.writeError(w, http.StatusBadRequest, "Invalid JSON payload: "+err.Error())
			return
		}
		updated, err := r.cfg.ConnectorsManager.ToggleConnector(id, toggleReq.Enabled)
		if err != nil {
			r.writeError(w, http.StatusNotFound, err.Error())
			return
		}
		r.writeJSON(w, http.StatusOK, updated)
		return
	}

	// 2. POST /api/v1/connectors/items/{id}/test
	if len(parts) == 2 && parts[1] == "test" {
		if req.Method != http.MethodPost {
			r.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}
		var item types.ConnectorItem
		_ = json.NewDecoder(req.Body).Decode(&item) // Optional override body
		res := r.cfg.ConnectorsManager.TestGenericConnector(req.Context(), id, &item)
		r.writeJSON(w, http.StatusOK, res)
		return
	}

	// 3. POST /api/v1/connectors/items/{id}/test-mcp
	if len(parts) == 2 && parts[1] == "test-mcp" {
		if req.Method != http.MethodPost {
			r.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}
		var payload struct {
			types.MCPConfig
			MCP *types.MCPConfig `json:"mcp,omitempty"`
		}
		_ = json.NewDecoder(req.Body).Decode(&payload)
		mcpCfg := payload.MCPConfig
		if payload.MCP != nil && payload.MCP.Command != "" {
			mcpCfg = *payload.MCP
		}
		if mcpCfg.Command == "" {
			item, err := r.cfg.ConnectorsManager.GetConnector(id)
			if err == nil && item.MCP != nil {
				mcpCfg = *item.MCP
			}
		}
		res := r.cfg.ConnectorsManager.TestMCPConnector(req.Context(), id, &mcpCfg)
		r.writeJSON(w, http.StatusOK, res)
		return
	}

	// 4. /api/v1/connectors/items/{id}
	switch req.Method {
	case http.MethodGet:
		item, err := r.cfg.ConnectorsManager.GetConnector(id)
		if err != nil {
			r.writeError(w, http.StatusNotFound, err.Error())
			return
		}
		r.writeJSON(w, http.StatusOK, item)

	case http.MethodPut, http.MethodPost:
		var item types.ConnectorItem
		if err := json.NewDecoder(req.Body).Decode(&item); err != nil {
			r.writeError(w, http.StatusBadRequest, "Invalid JSON payload: "+err.Error())
			return
		}
		item.ID = id
		updated, err := r.cfg.ConnectorsManager.UpdateConnector(item)
		if err != nil {
			r.writeError(w, http.StatusInternalServerError, "Failed to update connector: "+err.Error())
			return
		}
		r.writeJSON(w, http.StatusOK, updated)

	default:
		r.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// handleConnectorJira updates JIRA configuration.
func (r *Router) handleConnectorJira(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPut && req.Method != http.MethodPost {
		r.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var jiraCfg types.JiraConfig
	if err := json.NewDecoder(req.Body).Decode(&jiraCfg); err != nil {
		r.writeError(w, http.StatusBadRequest, "Invalid JSON payload: "+err.Error())
		return
	}

	if err := r.cfg.ConnectorsManager.UpdateJira(jiraCfg); err != nil {
		r.writeError(w, http.StatusInternalServerError, "Failed to save JIRA configuration: "+err.Error())
		return
	}

	r.writeJSON(w, http.StatusOK, r.cfg.ConnectorsManager.GetConfig().Jira)
}

// handleConnectorConfluence updates Confluence configuration.
func (r *Router) handleConnectorConfluence(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPut && req.Method != http.MethodPost {
		r.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var confCfg types.ConfluenceConfig
	if err := json.NewDecoder(req.Body).Decode(&confCfg); err != nil {
		r.writeError(w, http.StatusBadRequest, "Invalid JSON payload: "+err.Error())
		return
	}

	if err := r.cfg.ConnectorsManager.UpdateConfluence(confCfg); err != nil {
		r.writeError(w, http.StatusInternalServerError, "Failed to save Confluence configuration: "+err.Error())
		return
	}

	r.writeJSON(w, http.StatusOK, r.cfg.ConnectorsManager.GetConfig().Confluence)
}

// handleConnectorTest tests connectivity to JIRA or Confluence.
func (r *Router) handleConnectorTest(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		r.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var testReq types.TestConnectorRequest
	if err := json.NewDecoder(req.Body).Decode(&testReq); err != nil {
		r.writeError(w, http.StatusBadRequest, "Invalid JSON payload: "+err.Error())
		return
	}

	target := strings.ToLower(strings.TrimSpace(string(testReq.Type)))
	var result types.TestConnectorResponse

	switch target {
	case "jira":
		jiraCfg := r.cfg.ConnectorsManager.GetConfig().Jira
		if testReq.Jira != nil {
			jiraCfg = *testReq.Jira
		}
		result = r.cfg.ConnectorsManager.TestJira(req.Context(), jiraCfg)
	case "confluence":
		confCfg := r.cfg.ConnectorsManager.GetConfig().Confluence
		if testReq.Confluence != nil {
			confCfg = *testReq.Confluence
		}
		result = r.cfg.ConnectorsManager.TestConfluence(req.Context(), confCfg)
	default:
		r.writeError(w, http.StatusBadRequest, "Unsupported connector type. Use 'jira' or 'confluence'")
		return
	}

	r.writeJSON(w, http.StatusOK, result)
}

// handleJiraIssues searches or lists available JIRA issues for import.
func (r *Router) handleJiraIssues(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		r.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	query := req.URL.Query().Get("q")
	if query == "" {
		query = req.URL.Query().Get("query")
	}

	issues, err := r.cfg.ConnectorsManager.SearchJiraIssues(req.Context(), query)
	if errors.Is(err, connectors.ErrJiraNotConnected) {
		r.writeError(w, http.StatusPreconditionFailed, err.Error())
		return
	}
	if err != nil {
		r.writeError(w, http.StatusBadGateway, "Failed to search JIRA issues: "+err.Error())
		return
	}
	r.writeJSON(w, http.StatusOK, issues)
}

// handleJiraImport adds one JIRA issue to the board. An issue already on the board is not
// duplicated: the response is 409 with the existing task.
func (r *Router) handleJiraImport(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		r.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var importReq types.ImportJiraIssueRequest
	if err := json.NewDecoder(req.Body).Decode(&importReq); err != nil {
		r.writeError(w, http.StatusBadRequest, "Invalid JSON payload: "+err.Error())
		return
	}
	key := strings.ToUpper(strings.TrimSpace(importReq.IssueKey))
	if key == "" {
		r.writeError(w, http.StatusBadRequest, "issue_key is required")
		return
	}

	r.mu.RLock()
	existing := cloneTask(r.jiraTaskIndexLocked()[key])
	r.mu.RUnlock()
	if existing != nil {
		r.writeJSON(w, http.StatusConflict, map[string]interface{}{
			"error": fmt.Sprintf("%s is already on the board as %s", key, existing.ID), "task": existing})
		return
	}

	issue, err := r.cfg.ConnectorsManager.GetJiraIssue(req.Context(), key)
	if err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, connectors.ErrJiraNotConnected) {
			status = http.StatusPreconditionFailed
		}
		r.writeError(w, status, fmt.Sprintf("Could not load JIRA issue %s: %v", key, err))
		return
	}

	startStage := importReq.StartStageID
	if importReq.ActiveSlice != nil && importReq.ActiveSlice.StartStageID != "" {
		startStage = importReq.ActiveSlice.StartStageID
	}

	r.mu.Lock()
	if t := r.jiraTaskIndexLocked()[key]; t != nil { // lost a race with a concurrent import/sync
		existing = cloneTask(t)
		r.mu.Unlock()
		r.writeJSON(w, http.StatusConflict, map[string]interface{}{
			"error": fmt.Sprintf("%s is already on the board as %s", key, existing.ID), "task": existing})
		return
	}
	delete(r.dismissedJira, key) // an explicit import overrides an earlier removal
	task := r.newJiraTaskLocked(*issue, jiraTaskOptions{
		WorkflowID:     importReq.WorkflowID,
		SelectedMethod: importReq.SelectedMethod,
		AssignedRepos:  importReq.AssignedRepos,
		StartStageID:   startStage,
		Source:         "import",
	})
	task.ActiveSlice = importReq.ActiveSlice
	for k, v := range importReq.Metadata {
		if _, reserved := task.Metadata[k]; !reserved {
			task.Metadata[k] = v
		}
	}
	created := cloneTask(task)
	r.mu.Unlock()

	r.writeJiraPRD(created.ID, *issue)
	r.addEntry(created.ID, types.ConsoleEntry{Kind: types.ConsoleKindSystem, Content: fmt.Sprintf(
		"Imported from JIRA %s at %s. Run the stage to start the agent, or ask it a question below.", issue.Key, created.CurrentStageID)})
	r.saveBoardNow()
	r.broadcastTask(created)
	r.writeJSON(w, http.StatusCreated, created)
}

// handleJiraSync reads (GET) or replaces (PUT) the board sync rules.
func (r *Router) handleJiraSync(w http.ResponseWriter, req *http.Request) {
	mgr := r.cfg.ConnectorsManager
	switch req.Method {
	case http.MethodGet:
	case http.MethodPut, http.MethodPost:
		var cfg types.JiraSyncConfig
		if err := json.NewDecoder(req.Body).Decode(&cfg); err != nil {
			r.writeError(w, http.StatusBadRequest, "Invalid JSON payload: "+err.Error())
			return
		}
		if cfg.DefaultStageID != "" && stageIndexOf(cfg.DefaultStageID) < 0 {
			r.writeError(w, http.StatusBadRequest, "Unknown default stage: "+cfg.DefaultStageID)
			return
		}
		for status, stage := range cfg.StatusStageMap {
			if stageIndexOf(stage) < 0 {
				r.writeError(w, http.StatusBadRequest, fmt.Sprintf("Unknown stage %q for JIRA status %q", stage, status))
				return
			}
		}
		if _, err := mgr.UpdateJiraSyncConfig(cfg); err != nil {
			r.writeError(w, http.StatusInternalServerError, "Failed to save sync rules: "+err.Error())
			return
		}
		r.kickJiraSync()
	default:
		r.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	r.writeJSON(w, http.StatusOK, types.JiraSyncSettings{Config: mgr.GetJiraSyncConfig(), Status: r.jiraSyncStatus()})
}

// handleJiraSyncRun syncs the board now and reports the outcome.
func (r *Router) handleJiraSyncRun(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		r.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	ctx, cancel := context.WithTimeout(req.Context(), time.Minute)
	defer cancel()
	status, err := r.syncJiraBoard(ctx)
	settings := types.JiraSyncSettings{Config: r.cfg.ConnectorsManager.GetJiraSyncConfig(), Status: status}
	switch {
	case err == nil:
		r.writeJSON(w, http.StatusOK, settings)
	case errors.Is(err, errJiraSyncBusy):
		r.writeJSON(w, http.StatusConflict, map[string]interface{}{"error": err.Error(), "config": settings.Config, "status": status})
	case !status.Connected:
		r.writeJSON(w, http.StatusPreconditionFailed, map[string]interface{}{"error": err.Error(), "config": settings.Config, "status": status})
	default:
		r.writeJSON(w, http.StatusBadGateway, map[string]interface{}{"error": err.Error(), "config": settings.Config, "status": status})
	}
}

// handleJiraSyncDismissed clears (DELETE) the list of issues the operator removed, so the next
// sync may bring them back.
func (r *Router) handleJiraSyncDismissed(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodDelete {
		r.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	r.mu.Lock()
	r.dismissedJira = make(map[string]bool)
	r.mu.Unlock()
	r.saveBoardNow()
	r.kickJiraSync()
	r.writeJSON(w, http.StatusOK, types.JiraSyncSettings{Config: r.cfg.ConnectorsManager.GetJiraSyncConfig(), Status: r.jiraSyncStatus()})
}

// handleConfluencePublish publishes tech docs or PRDs to Confluence.
func (r *Router) handleConfluencePublish(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		r.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var pubReq types.ConfluencePublishRequest
	if err := json.NewDecoder(req.Body).Decode(&pubReq); err != nil {
		r.writeError(w, http.StatusBadRequest, "Invalid JSON payload: "+err.Error())
		return
	}

	// Resolve content and title if task ID is provided
	var targetTask *types.Task
	if pubReq.TaskID != "" {
		r.mu.RLock()
		targetTask = r.tasks[pubReq.TaskID]
		r.mu.RUnlock()
	}

	if pubReq.ContentMarkdown == "" && pubReq.TaskID != "" {
		// Look up RFC or PRD from task artifact directory
		taskDir := filepath.Join(r.cfg.RootDir, ".sdlc", "artifacts", pubReq.TaskID)
		rfcPath := filepath.Join(taskDir, "TECH_DOC_RFC.md")
		prdPath := filepath.Join(taskDir, "PRD.md")

		if content, err := os.ReadFile(rfcPath); err == nil {
			pubReq.ContentMarkdown = string(content)
			if pubReq.Title == "" {
				pubReq.Title = fmt.Sprintf("RFC: %s", targetTask.Title)
			}
			if pubReq.DocType == "" {
				pubReq.DocType = "TECH_DOC_RFC"
			}
		} else if content, err := os.ReadFile(prdPath); err == nil {
			pubReq.ContentMarkdown = string(content)
			if pubReq.Title == "" {
				pubReq.Title = fmt.Sprintf("PRD: %s", targetTask.Title)
			}
			if pubReq.DocType == "" {
				pubReq.DocType = "PRD"
			}
		} else {
			// Fallback generated Tech Doc RFC
			if targetTask != nil {
				pubReq.ContentMarkdown = fmt.Sprintf("# Technical Design Document: %s\n\n"+
					"**Task ID:** %s\n**Stage:** %s\n**Workflow:** %s\n**Assigned Repos:** %s\n\n"+
					"## Architecture & Design\n"+
					"This technical design document outlines the architectural implementation details for %s.\n\n"+
					"### Key Components\n- API Gateway & Handlers\n- Domain Service Layer\n- Persistence and Data Models\n\n"+
					"### Security & Verification\n- Complete ATDD test harness\n- Token idempotency and replay attack prevention\n",
					targetTask.Title, targetTask.ID, targetTask.CurrentStageID, targetTask.WorkflowID,
					strings.Join(targetTask.AssignedRepos, ", "), targetTask.Description)
				if pubReq.Title == "" {
					pubReq.Title = fmt.Sprintf("RFC: %s", targetTask.Title)
				}
				if pubReq.DocType == "" {
					pubReq.DocType = "TECH_DOC_RFC"
				}
			}
		}
	}

	if pubReq.Title == "" {
		if targetTask != nil {
			pubReq.Title = fmt.Sprintf("Tech Doc: %s", targetTask.Title)
		} else {
			pubReq.Title = "Technical Design RFC Document"
		}
	}

	// Publish via Connector Manager
	resp, err := r.cfg.ConnectorsManager.PublishToConfluence(req.Context(), pubReq)
	if err != nil {
		r.writeError(w, http.StatusInternalServerError, "Failed to publish to Confluence: "+err.Error())
		return
	}

	// Update task metadata if linked to a task
	if targetTask != nil {
		r.mu.Lock()
		if targetTask.Metadata == nil {
			targetTask.Metadata = make(map[string]string)
		}
		targetTask.Metadata["confluence_page_url"] = resp.PageURL
		targetTask.Metadata["confluence_page_id"] = resp.PageID
		targetTask.Metadata["confluence_space"] = resp.SpaceKey
		targetTask.UpdatedAt = time.Now()
		r.mu.Unlock()

		if r.cfg.WSHub != nil {
			r.cfg.WSHub.BroadcastEvent(&types.OrchestratorEvent{
				Type:      types.EventTaskStatus,
				TaskID:    targetTask.ID,
				StageID:   targetTask.CurrentStageID,
				Timestamp: time.Now(),
				Payload:   targetTask,
			})
		}
	}

	r.writeJSON(w, http.StatusOK, resp)
}

// handleConnectorMCPConfig exports active connector configurations as a standard mcpServers JSON dictionary.
func (r *Router) handleConnectorMCPConfig(w http.ResponseWriter, req *http.Request) {
	if r.cfg.ConnectorsManager == nil {
		r.writeError(w, http.StatusServiceUnavailable, "Connectors manager not configured")
		return
	}
	mcpExport := r.cfg.ConnectorsManager.GetMCPExportConfig()
	r.writeJSON(w, http.StatusOK, mcpExport)
}

// handleConnectorPing triggers an immediate live connectivity probe across all active connectors.
func (r *Router) handleConnectorPing(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		r.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	if r.cfg.ConnectorsManager == nil {
		r.writeError(w, http.StatusServiceUnavailable, "Connectors manager not configured")
		return
	}

	summary := r.cfg.ConnectorsManager.PingAll(req.Context())
	r.writeJSON(w, http.StatusOK, summary)
}

// handleConnectorPingConfig retrieves or updates periodic background ping settings.
func (r *Router) handleConnectorPingConfig(w http.ResponseWriter, req *http.Request) {
	if r.cfg.ConnectorsManager == nil {
		r.writeError(w, http.StatusServiceUnavailable, "Connectors manager not configured")
		return
	}

	switch req.Method {
	case http.MethodGet:
		cfg := r.cfg.ConnectorsManager.GetPingConfig()
		r.writeJSON(w, http.StatusOK, cfg)

	case http.MethodPut, http.MethodPost:
		var pingCfg types.ConnectorPingConfig
		if err := json.NewDecoder(req.Body).Decode(&pingCfg); err != nil {
			r.writeError(w, http.StatusBadRequest, "Invalid JSON payload: "+err.Error())
			return
		}
		if err := r.cfg.ConnectorsManager.UpdatePingConfig(pingCfg); err != nil {
			r.writeError(w, http.StatusInternalServerError, "Failed to update ping config: "+err.Error())
			return
		}
		r.writeJSON(w, http.StatusOK, r.cfg.ConnectorsManager.GetPingConfig())

	default:
		r.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}
