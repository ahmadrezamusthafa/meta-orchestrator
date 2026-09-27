package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// handleSkills returns the modular catalog of skills across all sources with compatibility reports.
func (r *Router) handleSkills(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		r.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	if r.cfg.SkillResolver == nil {
		r.writeError(w, http.StatusServiceUnavailable, "Skill resolver not initialized")
		return
	}

	enabledOnly := req.URL.Query().Get("enabled_only") == "true" || req.URL.Query().Get("active") == "true"
	formatFilter := strings.ToLower(req.URL.Query().Get("format"))
	sourceTypeFilter := strings.ToLower(req.URL.Query().Get("source_type"))
	searchQuery := strings.ToLower(req.URL.Query().Get("search"))

	skillsList, err := r.cfg.SkillResolver.ListAllSkillsWithDetails(enabledOnly)
	if err != nil {
		r.writeError(w, http.StatusInternalServerError, "Failed to list skills: "+err.Error())
		return
	}

	filtered := make([]*types.UniversalSkillContract, 0)
	for _, s := range skillsList {
		if formatFilter != "" && strings.ToLower(string(s.SourceFormat)) != formatFilter {
			continue
		}
		if sourceTypeFilter != "" && strings.ToLower(s.SourceType) != sourceTypeFilter {
			continue
		}
		if searchQuery != "" {
			nameMatch := strings.Contains(strings.ToLower(s.Name), searchQuery)
			descMatch := strings.Contains(strings.ToLower(s.Description), searchQuery)
			if !nameMatch && !descMatch {
				continue
			}
		}
		filtered = append(filtered, s)
	}

	r.writeJSON(w, http.StatusOK, filtered)
}

// handleSkillAction processes item-level operations like inspection, toggling, and compatibility audit.
func (r *Router) handleSkillAction(w http.ResponseWriter, req *http.Request) {
	if r.cfg.SkillResolver == nil {
		r.writeError(w, http.StatusServiceUnavailable, "Skill resolver not initialized")
		return
	}

	// Route: /api/v1/skills/{name} or /api/v1/skills/{name}/toggle or /api/v1/skills/{name}/compatibility
	path := strings.TrimPrefix(req.URL.Path, "/api/v1/skills/")
	parts := strings.Split(path, "/")
	skillName := parts[0]
	if skillName == "" {
		r.writeError(w, http.StatusBadRequest, "Skill name required")
		return
	}

	// 1. Toggle enabled state
	if len(parts) >= 2 && parts[1] == "toggle" {
		if req.Method != http.MethodPost && req.Method != http.MethodPut {
			r.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}
		var payload struct {
			Enabled bool `json:"enabled"`
		}
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
			r.writeError(w, http.StatusBadRequest, "Invalid JSON payload: "+err.Error())
			return
		}
		if err := r.cfg.SkillResolver.ToggleSkill(skillName, payload.Enabled); err != nil {
			r.writeError(w, http.StatusInternalServerError, "Failed to toggle skill: "+err.Error())
			return
		}
		r.writeJSON(w, http.StatusOK, map[string]interface{}{
			"name":    skillName,
			"enabled": payload.Enabled,
			"status":  "updated",
		})
		return
	}

	// 2. Compatibility details
	if len(parts) >= 2 && parts[1] == "compatibility" {
		if req.Method != http.MethodGet {
			r.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}
		skill, err := r.cfg.SkillResolver.ResolveSkill(skillName)
		if err != nil {
			r.writeError(w, http.StatusNotFound, "Skill not found: "+err.Error())
			return
		}
		r.writeJSON(w, http.StatusOK, skill.Compatibility)
		return
	}

	// 3. Inspect skill details
	if req.Method == http.MethodGet {
		all, err := r.cfg.SkillResolver.ListAllSkillsWithDetails(false)
		if err != nil {
			r.writeError(w, http.StatusInternalServerError, "Failed to query skill: "+err.Error())
			return
		}
		for _, s := range all {
			if s.Name == skillName {
				r.writeJSON(w, http.StatusOK, s)
				return
			}
		}
		r.writeError(w, http.StatusNotFound, "Skill '"+skillName+"' not found")
		return
	}

	r.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
}

// handleSkillSources manages registration and listing of external skill directories (e.g. Claude).
func (r *Router) handleSkillSources(w http.ResponseWriter, req *http.Request) {
	if r.cfg.SkillResolver == nil {
		r.writeError(w, http.StatusServiceUnavailable, "Skill resolver not initialized")
		return
	}

	switch req.Method {
	case http.MethodGet:
		sources := r.cfg.SkillResolver.ListSources()
		r.writeJSON(w, http.StatusOK, sources)

	case http.MethodPost:
		var payload struct {
			Name   string            `json:"name"`
			Path   string            `json:"path"`
			Format types.SkillFormat `json:"format"`
		}
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
			r.writeError(w, http.StatusBadRequest, "Invalid request payload: "+err.Error())
			return
		}
		if payload.Path == "" {
			r.writeError(w, http.StatusBadRequest, "Directory path is required")
			return
		}

		src, discovered, err := r.cfg.SkillResolver.RegisterSource(payload.Name, payload.Path, payload.Format)
		if err != nil {
			r.writeError(w, http.StatusBadRequest, "Failed to register skill source: "+err.Error())
			return
		}

		r.writeJSON(w, http.StatusCreated, map[string]interface{}{
			"source":           src,
			"discovered_count": len(discovered),
			"skills":           discovered,
		})

	default:
		r.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// handleSkillSourceAction handles deletion of registered sources.
func (r *Router) handleSkillSourceAction(w http.ResponseWriter, req *http.Request) {
	if r.cfg.SkillResolver == nil {
		r.writeError(w, http.StatusServiceUnavailable, "Skill resolver not initialized")
		return
	}
	if req.Method != http.MethodDelete {
		r.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	sourceID := strings.TrimPrefix(req.URL.Path, "/api/v1/skills/sources/")
	if sourceID == "" {
		r.writeError(w, http.StatusBadRequest, "Source ID is required")
		return
	}

	if err := r.cfg.SkillResolver.UnregisterSource(sourceID); err != nil {
		r.writeError(w, http.StatusNotFound, "Failed to unregister source: "+err.Error())
		return
	}

	r.writeJSON(w, http.StatusOK, map[string]string{
		"status":    "deleted",
		"source_id": sourceID,
	})
}

// handleSkillCheckCompatibility performs a pre-flight compatibility check on an un-registered path or directory.
func (r *Router) handleSkillCheckCompatibility(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		r.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	if r.cfg.SkillResolver == nil {
		r.writeError(w, http.StatusServiceUnavailable, "Skill resolver not initialized")
		return
	}

	var payload struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
		r.writeError(w, http.StatusBadRequest, "Invalid JSON payload: "+err.Error())
		return
	}
	if payload.Path == "" {
		r.writeError(w, http.StatusBadRequest, "Path parameter is required")
		return
	}

	skills, err := r.cfg.SkillResolver.CheckPathCompatibility(payload.Path)
	if err != nil {
		r.writeJSON(w, http.StatusOK, map[string]interface{}{
			"compatible": false,
			"path":       payload.Path,
			"error":      err.Error(),
			"skills":     []interface{}{},
		})
		return
	}

	allCompatible := true
	for _, s := range skills {
		if s.Compatibility != nil && s.Compatibility.Status == types.CompatibilityStatusIncompatible {
			allCompatible = false
			break
		}
	}

	r.writeJSON(w, http.StatusOK, map[string]interface{}{
		"compatible":       allCompatible,
		"path":             payload.Path,
		"discovered_count": len(skills),
		"skills":           skills,
	})
}

// handleSkillRescan triggers a full refresh of all skill directories and in-memory caches.
func (r *Router) handleSkillRescan(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		r.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	if r.cfg.SkillResolver == nil {
		r.writeError(w, http.StatusServiceUnavailable, "Skill resolver not initialized")
		return
	}

	r.cfg.SkillResolver.Rescan()
	all, _ := r.cfg.SkillResolver.ListAllSkillsWithDetails(false)

	r.writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":      "rescanned",
		"total_count": len(all),
	})
}
