package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

type ToolDTO struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Category       string   `json:"category"`
	CurrentVersion string   `json:"current_version"`
	LatestVersion  string   `json:"latest_version"`
	BestFitVersion string   `json:"best_fit_version"`
	Status         string   `json:"status"` // "HEALTHY", "UPDATE_AVAILABLE", "ERROR", "NOT_INSTALLED"
	Description    string   `json:"description"`
	PastVersions   []string `json:"past_versions"`
}

type ToolInstallRequest struct {
	ToolID  string `json:"tool_id"`
	Version string `json:"version"`
}

type ToolRollbackRequest struct {
	ToolID        string `json:"tool_id"`
	TargetVersion string `json:"target_version"`
}

var defaultTools = []ToolDTO{
	{
		ID:             "bmad",
		Name:           "BMAD Multi-Agent Framework",
		Category:       "Methodologies",
		CurrentVersion: "v2.4.0",
		LatestVersion:  "v2.4.1",
		BestFitVersion: "v2.4.0",
		Status:         "UPDATE_AVAILABLE",
		Description:    "Autonomous multi-agent persona collaboration engine for complex architectural flows.",
		PastVersions:   []string{"v2.3.9", "v2.3.5", "v2.2.0"},
	},
	{
		ID:             "superpower",
		Name:           "Superpower Tool Suite",
		Category:       "Methodologies",
		CurrentVersion: "v1.8.2",
		LatestVersion:  "v1.8.2",
		BestFitVersion: "v1.8.2",
		Status:         "HEALTHY",
		Description:    "High-speed bare-metal execution tools with strict capability boundaries.",
		PastVersions:   []string{"v1.8.0", "v1.7.5"},
	},
	{
		ID:             "playwright",
		Name:           "Playwright Headless Browser",
		Category:       "Runtimes",
		CurrentVersion: "v1.45.0",
		LatestVersion:  "v1.45.0",
		BestFitVersion: "v1.45.0",
		Status:         "HEALTHY",
		Description:    "Fast, reliable end-to-end testing with video recording and viewport capture.",
		PastVersions:   []string{"v1.44.1", "v1.43.0"},
	},
	{
		ID:             "tree-sitter",
		Name:           "Tree-Sitter AST Parsers",
		Category:       "Parsers",
		CurrentVersion: "v0.22.6",
		LatestVersion:  "v0.22.6",
		BestFitVersion: "v0.22.6",
		Status:         "HEALTHY",
		Description:    "Incremental parsing library generating syntax trees for TS, Go, PHP, Python.",
		PastVersions:   []string{"v0.22.1", "v0.21.0"},
	},
	{
		ID:             "xterm-js",
		Name:           "Xterm.js Terminal Engine",
		Category:       "UI Kits",
		CurrentVersion: "v5.5.0",
		LatestVersion:  "v5.5.0",
		BestFitVersion: "v5.5.0",
		Status:         "HEALTHY",
		Description:    "Hardware-accelerated terminal front-end for real-time stdout/stderr streaming.",
		PastVersions:   []string{"v5.4.0"},
	},
}

func (r *Router) handleTools(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		r.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	category := req.URL.Query().Get("category")
	search := strings.ToLower(req.URL.Query().Get("search"))

	var result []ToolDTO
	for _, tool := range defaultTools {
		if category != "" && category != "All" && tool.Category != category {
			continue
		}
		if search != "" && !strings.Contains(strings.ToLower(tool.Name), search) && !strings.Contains(strings.ToLower(tool.ID), search) {
			continue
		}
		result = append(result, tool)
	}

	r.writeJSON(w, http.StatusOK, result)
}

func (r *Router) handleToolAction(w http.ResponseWriter, req *http.Request) {
	path := strings.TrimPrefix(req.URL.Path, "/api/v1/tools/")

	switch path {
	case "install":
		if req.Method != http.MethodPost {
			r.writeError(w, http.StatusMethodNotAllowed, "POST required")
			return
		}
		var body ToolInstallRequest
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			r.writeError(w, http.StatusBadRequest, "Invalid request body")
			return
		}

		// Broadcast installation progress steps over WebSocket
		go func() {
			steps := []string{"1. Pre-flight checks passed", "2. Fetching & compiling binaries", "3. Running verification diagnostics", "4. Activation complete"}
			for i, step := range steps {
				time.Sleep(150 * time.Millisecond)
				if r.cfg.WSHub != nil {
					r.cfg.WSHub.BroadcastEvent(&types.OrchestratorEvent{
						Type:      types.EventToolProgress,
						Timestamp: time.Now(),
						Payload: map[string]interface{}{
							"tool_id": body.ToolID,
							"version": body.Version,
							"step":    i + 1,
							"total":   len(steps),
							"message": step,
						},
					})
				}
			}
		}()

		r.writeJSON(w, http.StatusOK, map[string]string{
			"status":  "installing",
			"tool_id": body.ToolID,
			"version": body.Version,
		})

	case "rollback":
		if req.Method != http.MethodPost {
			r.writeError(w, http.StatusMethodNotAllowed, "POST required")
			return
		}
		var body ToolRollbackRequest
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			r.writeError(w, http.StatusBadRequest, "Invalid request body")
			return
		}

		// Simulate atomic symlink swap
		for i, t := range defaultTools {
			if t.ID == body.ToolID {
				defaultTools[i].CurrentVersion = body.TargetVersion
				defaultTools[i].Status = "HEALTHY"
				break
			}
		}

		r.writeJSON(w, http.StatusOK, map[string]interface{}{
			"status":         "rollback_success",
			"tool_id":        body.ToolID,
			"active_version": body.TargetVersion,
		})

	case "resolve-matrix":
		if req.Method != http.MethodPost {
			r.writeError(w, http.StatusMethodNotAllowed, "POST required")
			return
		}

		r.writeJSON(w, http.StatusOK, map[string]interface{}{
			"matrix_status": "optimal",
			"host_os":       "darwin_arm64",
			"tools":         defaultTools,
			"conflicts":     []string{},
		})

	default:
		r.writeError(w, http.StatusNotFound, "Tool action not found")
	}
}
