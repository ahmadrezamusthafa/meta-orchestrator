package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/projects"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/router"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

func TestSuggestReposRanksProjectRepos(t *testing.T) {
	root := t.TempDir()
	pm := projects.NewProjectManager(root)
	proj, err := pm.Create(&types.Project{Name: "Billing", Repos: []types.ProjectRepo{
		{Name: "billing", Path: t.TempDir(), Role: types.RoleBackend},
		{Name: "billing-internal-fe", Path: t.TempDir(), Role: types.RoleFrontend},
		{Name: "api-contracts", Path: t.TempDir(), Role: types.RoleContracts},
	}})
	if err != nil {
		t.Fatal(err)
	}
	r := NewRouter(RouterConfig{RootDir: root, ProjectManager: pm})

	w := do(r, http.MethodPost, "/api/v1/projects/"+proj.ID+"/suggest-repos",
		`{"title":"Refund button on the invoice page","description":"Calls the billing refund endpoint"}`)
	var out struct {
		SuggestedRepos []router.RepoMatch `json:"suggested_repos"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || w.Code != http.StatusOK {
		t.Fatalf("suggest = %d %s", w.Code, w.Body.String())
	}
	if len(out.SuggestedRepos) != 2 || out.SuggestedRepos[0].Name != "billing" || out.SuggestedRepos[1].Name != "billing-internal-fe" {
		t.Fatalf("suggested = %+v", out.SuggestedRepos)
	}

	w = do(r, http.MethodPost, "/api/v1/projects/"+proj.ID+"/suggest-repos", `{"title":"Rename a variable"}`)
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if w.Code != http.StatusOK || len(out.SuggestedRepos) != 0 {
		t.Fatalf("no evidence must suggest nothing: %d %+v", w.Code, out.SuggestedRepos)
	}
	if w := do(r, http.MethodPost, "/api/v1/projects/nope/suggest-repos", `{"title":"x"}`); w.Code != http.StatusNotFound {
		t.Fatalf("unknown project = %d", w.Code)
	}
}
