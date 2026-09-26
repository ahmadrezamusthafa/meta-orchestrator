package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

type ScanDirectoryRequest struct {
	Path string `json:"path"`
}

func (r *Router) handleProjects(w http.ResponseWriter, req *http.Request) {
	if r.cfg.ProjectManager == nil {
		r.writeError(w, http.StatusInternalServerError, "Project manager uninitialized")
		return
	}

	switch req.Method {
	case http.MethodGet:
		projects := r.cfg.ProjectManager.List()
		r.writeJSON(w, http.StatusOK, projects)

	case http.MethodPost:
		var p types.Project
		if err := json.NewDecoder(req.Body).Decode(&p); err != nil {
			r.writeError(w, http.StatusBadRequest, fmt.Sprintf("Invalid JSON: %v", err))
			return
		}

		created, err := r.cfg.ProjectManager.Create(&p)
		if err != nil {
			r.writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		r.writeJSON(w, http.StatusCreated, created)

	default:
		r.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

func (r *Router) handleProjectItem(w http.ResponseWriter, req *http.Request) {
	if r.cfg.ProjectManager == nil {
		r.writeError(w, http.StatusInternalServerError, "Project manager uninitialized")
		return
	}

	path := strings.TrimPrefix(req.URL.Path, "/api/v1/projects/")
	parts := strings.Split(path, "/")
	projectID := parts[0]

	if projectID == "" {
		r.writeError(w, http.StatusBadRequest, "Missing project ID")
		return
	}

	// Handle sub-action: /api/v1/projects/{id}/resync
	if len(parts) > 1 && parts[1] == "resync" {
		if req.Method != http.MethodPost {
			r.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}
		resynced, err := r.cfg.ProjectManager.ResyncSymlinks(projectID)
		if err != nil {
			r.writeError(w, http.StatusBadRequest, fmt.Sprintf("Failed to resync symlinks: %v", err))
			return
		}
		r.writeJSON(w, http.StatusOK, resynced)
		return
	}

	switch req.Method {
	case http.MethodGet:
		proj, err := r.cfg.ProjectManager.Get(projectID)
		if err != nil {
			r.writeError(w, http.StatusNotFound, err.Error())
			return
		}
		r.writeJSON(w, http.StatusOK, proj)

	case http.MethodPut:
		var p types.Project
		if err := json.NewDecoder(req.Body).Decode(&p); err != nil {
			r.writeError(w, http.StatusBadRequest, fmt.Sprintf("Invalid JSON: %v", err))
			return
		}
		updated, err := r.cfg.ProjectManager.Update(projectID, &p)
		if err != nil {
			r.writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		r.writeJSON(w, http.StatusOK, updated)

	case http.MethodDelete:
		if err := r.cfg.ProjectManager.Delete(projectID); err != nil {
			r.writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		r.writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "id": projectID})

	default:
		r.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

func (r *Router) handleProjectScan(w http.ResponseWriter, req *http.Request) {
	if r.cfg.ProjectManager == nil {
		r.writeError(w, http.StatusInternalServerError, "Project manager uninitialized")
		return
	}

	if req.Method != http.MethodPost && req.Method != http.MethodGet {
		r.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var targetPath string
	if req.Method == http.MethodGet {
		targetPath = req.URL.Query().Get("path")
	} else {
		var body ScanDirectoryRequest
		_ = json.NewDecoder(req.Body).Decode(&body)
		targetPath = body.Path
	}

	result, err := r.cfg.ProjectManager.ScanDirectory(targetPath)
	if err != nil {
		r.writeError(w, http.StatusBadRequest, fmt.Sprintf("Scan failed: %v", err))
		return
	}

	r.writeJSON(w, http.StatusOK, result)
}
