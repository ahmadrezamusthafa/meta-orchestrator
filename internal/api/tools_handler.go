package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/toolchain"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/tools"
)

type ToolInstallRequest struct {
	ToolID  string `json:"tool_id"`
	Version string `json:"version"`
}

// toolsManifest is the Tool Hub manifest, relative to RootDir.
var toolsManifest = filepath.Join("configs", "tools.json")

// toolManager returns the configured Tool Hub manager, or one over RootDir's manifest.
func (r *Router) toolManager() *tools.ToolManager {
	r.toolMgrOnce.Do(func() {
		if r.cfg.ToolManager != nil {
			r.toolMgr = r.cfg.ToolManager
			return
		}
		r.toolMgr, _ = tools.Load(filepath.Join(r.cfg.RootDir, toolsManifest), r.toolchain)
	})
	return r.toolMgr
}

// handleTools: GET /api/v1/tools[?category=&search=] lists the manifest's tools with the result of
// running each health check on this machine.
func (r *Router) handleTools(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		r.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	category := req.URL.Query().Get("category")
	search := strings.ToLower(req.URL.Query().Get("search"))

	result := []tools.ToolStatus{}
	for _, t := range r.toolManager().List() {
		if category != "" && category != "All" && t.Category != category {
			continue
		}
		if search != "" && !strings.Contains(strings.ToLower(t.Name), search) && !strings.Contains(strings.ToLower(t.ID), search) {
			continue
		}
		result = append(result, t)
	}
	r.writeJSON(w, http.StatusOK, result)
}

// handleToolAction serves POST /tools/install, POST /tools/recheck, POST /tools/resolve-matrix and
// POST /tools/rollback (not supported: the orchestrator keeps no versioned install layout).
func (r *Router) handleToolAction(w http.ResponseWriter, req *http.Request) {
	path := strings.Trim(strings.TrimPrefix(req.URL.Path, "/api/v1/tools/"), "/")
	if req.Method != http.MethodPost {
		r.writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	tm := r.toolManager()
	switch path {
	case "install":
		var body ToolInstallRequest
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil || body.ToolID == "" {
			r.writeError(w, http.StatusBadRequest, `expected {"tool_id": "<id>"}`)
			return
		}
		job, err := tm.Install(body.ToolID, body.Version)
		var manual *tools.ManualInstallError
		switch {
		case errors.As(err, &manual):
			r.writeJSON(w, http.StatusNotImplemented, map[string]interface{}{
				"error": manual.Reason, "tool_id": manual.ToolID, "install": manual.Install})
		case errors.Is(err, tools.ErrUnknownTool):
			r.writeError(w, http.StatusNotFound, "unknown tool "+body.ToolID)
		case errors.Is(err, toolchain.ErrBusy):
			st, _ := tm.Get(body.ToolID)
			r.writeJSON(w, http.StatusConflict, map[string]interface{}{"error": err.Error(), "job": st.Job})
		case err != nil:
			r.writeError(w, http.StatusBadRequest, err.Error())
		default:
			r.writeJSON(w, http.StatusAccepted, job)
		}

	case "recheck":
		tm.Recheck()
		r.writeJSON(w, http.StatusOK, tm.List())

	case "resolve-matrix":
		r.writeJSON(w, http.StatusOK, tm.ResolveMatrix())

	case "rollback":
		r.writeError(w, http.StatusNotImplemented,
			"rollback is not supported: tools are installed by their package managers, which keep no previous versions the orchestrator could switch to")

	default:
		r.writeError(w, http.StatusNotFound, "Tool action not found")
	}
}
