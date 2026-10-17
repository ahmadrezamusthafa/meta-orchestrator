package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/router"
)

type repoSuggestionRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

// handleSuggestRepos: POST /api/v1/projects/{id}/suggest-repos ranks the project's repositories
// by how strongly a draft task's title and description point at them (name, stack, role words).
// The New Task form offers the result; nothing is selected without the operator.
func (r *Router) handleSuggestRepos(w http.ResponseWriter, req *http.Request, projectID string) {
	if req.Method != http.MethodPost {
		r.writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var body repoSuggestionRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil || strings.TrimSpace(body.Title+body.Description) == "" {
		r.writeError(w, http.StatusBadRequest, `expected {"title": "…", "description": "…"}`)
		return
	}
	proj, err := r.cfg.ProjectManager.Get(projectID)
	if err != nil {
		r.writeError(w, http.StatusNotFound, err.Error())
		return
	}
	repos := make([]*router.DiscoveredRepo, 0, len(proj.Repos))
	for _, repo := range proj.Repos {
		repos = append(repos, &router.DiscoveredRepo{Name: repo.Name, Role: string(repo.Role), Stack: repo.Stack})
	}
	matches := router.FindImpactedRepos(repos, body.Title+"\n"+body.Description)
	if matches == nil {
		matches = []router.RepoMatch{}
	}
	r.writeJSON(w, http.StatusOK, map[string]interface{}{"suggested_repos": matches})
}
