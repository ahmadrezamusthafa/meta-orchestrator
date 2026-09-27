package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/registry"
)

// handlePrompts returns discovered prompt templates across all tiers with optional filtering.
func (r *Router) handlePrompts(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		r.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	if r.cfg.PromptRegistry == nil {
		r.writeError(w, http.StatusServiceUnavailable, "Prompt registry not initialized")
		return
	}

	searchQuery := strings.ToLower(req.URL.Query().Get("search"))
	sourceFilter := strings.ToUpper(req.URL.Query().Get("source"))

	templates := r.cfg.PromptRegistry.ListPromptTemplates()
	filtered := make([]registry.PromptItemDTO, 0)

	for _, tmpl := range templates {
		if sourceFilter != "" && strings.ToUpper(tmpl.Source) != sourceFilter {
			continue
		}
		if searchQuery != "" {
			nameMatch := strings.Contains(strings.ToLower(tmpl.Name), searchQuery)
			idMatch := strings.Contains(strings.ToLower(tmpl.ID), searchQuery)
			descMatch := strings.Contains(strings.ToLower(tmpl.Description), searchQuery)
			if !nameMatch && !idMatch && !descMatch {
				continue
			}
		}
		filtered = append(filtered, tmpl)
	}

	r.writeJSON(w, http.StatusOK, filtered)
}

// handlePromptSources manages listing and adding external prompt template directories.
func (r *Router) handlePromptSources(w http.ResponseWriter, req *http.Request) {
	if r.cfg.PromptRegistry == nil {
		r.writeError(w, http.StatusServiceUnavailable, "Prompt registry not initialized")
		return
	}

	switch req.Method {
	case http.MethodGet:
		sources := r.cfg.PromptRegistry.ListSources()
		r.writeJSON(w, http.StatusOK, sources)

	case http.MethodPost:
		var payload struct {
			Name string `json:"name"`
			Path string `json:"path"`
		}
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
			r.writeError(w, http.StatusBadRequest, "Invalid JSON payload: "+err.Error())
			return
		}
		if payload.Path == "" {
			r.writeError(w, http.StatusBadRequest, "Directory path is required")
			return
		}

		src, discovered, err := r.cfg.PromptRegistry.RegisterSource(payload.Name, payload.Path)
		if err != nil {
			r.writeError(w, http.StatusBadRequest, "Failed to register prompt source: "+err.Error())
			return
		}

		r.writeJSON(w, http.StatusCreated, map[string]interface{}{
			"source":           src,
			"discovered_count": len(discovered),
			"templates":        discovered,
		})

	default:
		r.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// handlePromptSourceAction handles deletion of registered prompt sources.
func (r *Router) handlePromptSourceAction(w http.ResponseWriter, req *http.Request) {
	if r.cfg.PromptRegistry == nil {
		r.writeError(w, http.StatusServiceUnavailable, "Prompt registry not initialized")
		return
	}
	if req.Method != http.MethodDelete {
		r.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	sourceID := strings.TrimPrefix(req.URL.Path, "/api/v1/prompts/sources/")
	if sourceID == "" {
		r.writeError(w, http.StatusBadRequest, "Source ID is required")
		return
	}

	if err := r.cfg.PromptRegistry.UnregisterSource(sourceID); err != nil {
		r.writeError(w, http.StatusNotFound, "Failed to unregister source: "+err.Error())
		return
	}

	r.writeJSON(w, http.StatusOK, map[string]string{
		"status":    "deleted",
		"source_id": sourceID,
	})
}

// handlePromptCheckCompatibility inspects an un-registered prompt directory.
func (r *Router) handlePromptCheckCompatibility(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		r.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	if r.cfg.PromptRegistry == nil {
		r.writeError(w, http.StatusServiceUnavailable, "Prompt registry not initialized")
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
		r.writeError(w, http.StatusBadRequest, "Directory path is required")
		return
	}

	templates, err := r.cfg.PromptRegistry.CheckDirectoryCompatibility(payload.Path)
	if err != nil {
		r.writeJSON(w, http.StatusOK, map[string]interface{}{
			"compatible": false,
			"path":       payload.Path,
			"error":      err.Error(),
			"templates":  []interface{}{},
		})
		return
	}

	r.writeJSON(w, http.StatusOK, map[string]interface{}{
		"compatible":       true,
		"path":             payload.Path,
		"discovered_count": len(templates),
		"templates":        templates,
	})
}

// handlePromptRender renders a prompt template with parameter substitution.
func (r *Router) handlePromptRender(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		r.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	if r.cfg.PromptRegistry == nil {
		r.writeError(w, http.StatusServiceUnavailable, "Prompt registry not initialized")
		return
	}

	var payload struct {
		TemplateID  string            `json:"template_id"`
		RawTemplate string            `json:"raw_template"`
		Parameters  map[string]string `json:"parameters"`
	}
	if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
		r.writeError(w, http.StatusBadRequest, "Invalid JSON payload: "+err.Error())
		return
	}

	templateContent := payload.RawTemplate
	if payload.TemplateID != "" {
		resolved, err := r.cfg.PromptRegistry.ResolveTemplate(payload.TemplateID)
		if err != nil {
			r.writeError(w, http.StatusNotFound, "Failed to resolve prompt template: "+err.Error())
			return
		}
		templateContent = resolved
	}

	if templateContent == "" {
		r.writeError(w, http.StatusBadRequest, "Either template_id or raw_template must be provided")
		return
	}

	rendered := r.cfg.PromptRegistry.RenderCustom(templateContent, payload.Parameters)

	r.writeJSON(w, http.StatusOK, map[string]interface{}{
		"template_id": payload.TemplateID,
		"rendered":    rendered,
	})
}
