package api

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/projects"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// worktreeRouter registers a real git repo named "app" in a project.
func worktreeRouter(t *testing.T) (*Router, string) {
	t.Helper()
	root, src := t.TempDir(), t.TempDir()
	runGit(t, src, "init", "-q", "-b", "master")
	_ = os.WriteFile(filepath.Join(src, "main.go"), []byte("package main\n"), 0o644)
	runGit(t, src, "add", ".")
	runGit(t, src, "commit", "-q", "-m", "init")

	pm := projects.NewProjectManager(root)
	if _, err := pm.Create(&types.Project{Name: "Billing", Repos: []types.ProjectRepo{{Name: "app", Path: src}}}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	r := NewRouter(RouterConfig{RootDir: root, ProjectManager: pm})
	return r, src
}

func TestTaskWorktreeUsesJiraKeyAndServesDiff(t *testing.T) {
	r, src := worktreeRouter(t)
	r.mu.Lock()
	r.tasks["TASK-1"] = &types.Task{ID: "TASK-1", Title: "[PAY-42] Refund webhook retries", CurrentStageID: "task_implementation",
		State: types.TaskStatePending, AssignedRepos: []string{"app"}, Metadata: map[string]string{"jira_key": "PAY-42"}}
	r.mu.Unlock()

	if w := do(r, http.MethodGet, "/api/v1/tasks/TASK-1/diff", ""); !strings.Contains(w.Body.String(), "no worktree yet") {
		t.Fatalf("diff before the first run should explain there is no worktree: %s", w.Body.String())
	}

	dir := r.resolveWorkDir(r.taskSnapshot("TASK-1"))
	if dir != filepath.Join(r.taskWorktreeRoot("TASK-1"), "app") {
		t.Fatalf("agent should work in the task worktree, got %s", dir)
	}
	if b := r.taskSnapshot("TASK-1").Metadata["worktree_branch"]; b != "feat/PAY-42-refund-webhook-retries" {
		t.Fatalf("branch = %q", b)
	}
	_ = os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc retry() {}\n"), 0o644)

	w := do(r, http.MethodGet, "/api/v1/tasks/TASK-1/diff?against=base", "")
	var resp struct {
		Repos []repoDiffResult `json:"repos"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if w.Code != http.StatusOK || len(resp.Repos) != 1 || resp.Repos[0].RepoDiff == nil {
		t.Fatalf("diff → %d %s", w.Code, w.Body.String())
	}
	d := resp.Repos[0]
	if d.BaseRef != "master" || len(d.Files) != 1 || d.Additions != 2 || !strings.Contains(d.Patch, "+func retry() {}") {
		t.Fatalf("unexpected diff: %+v", d.RepoDiff)
	}
	if data, _ := os.ReadFile(filepath.Join(src, "main.go")); strings.Contains(string(data), "retry") {
		t.Fatal("the source checkout must stay untouched")
	}
	if w := do(r, http.MethodGet, "/api/v1/tasks/TASK-1/diff?against=origin", ""); w.Code != http.StatusBadRequest {
		t.Fatalf("invalid against → %d", w.Code)
	}
	if w := do(r, http.MethodGet, "/api/v1/tasks/TASK-1/worktree", ""); !strings.Contains(w.Body.String(), `"status":"ACTIVE"`) ||
		!strings.Contains(w.Body.String(), `"dirty":true`) {
		t.Fatalf("worktree status: %s", w.Body.String())
	}

	// Deleting keeps a worktree that still holds uncommitted work.
	do(r, http.MethodDelete, "/api/v1/tasks/TASK-1", "")
	if !dirExists(dir) {
		t.Fatal("a dirty worktree must survive task deletion")
	}
}

func TestCreatedTaskPlansBranchFromTaskID(t *testing.T) {
	r := hermeticRouter(t)
	task := decodeTask(t, do(r, http.MethodPost, "/api/v1/tasks", `{"title":"Add refund endpoint"}`).Body.Bytes())
	if want := "feat/" + strings.ToLower(task.ID) + "-add-refund-endpoint"; task.Metadata["worktree_branch"] != want {
		t.Fatalf("branch = %q, want %q", task.Metadata["worktree_branch"], want)
	}
}

func TestReposInsideOneGitRepoShareAWorktree(t *testing.T) {
	root, src := t.TempDir(), t.TempDir()
	runGit(t, src, "init", "-q", "-b", "master")
	_ = os.MkdirAll(filepath.Join(src, "web"), 0o755)
	_ = os.WriteFile(filepath.Join(src, "main.go"), []byte("package main\n"), 0o644)
	_ = os.WriteFile(filepath.Join(src, "web", "app.ts"), []byte("export {}\n"), 0o644)
	runGit(t, src, "add", ".")
	runGit(t, src, "commit", "-q", "-m", "init")
	pm := projects.NewProjectManager(root)
	_, _ = pm.Create(&types.Project{Name: "Mono", Repos: []types.ProjectRepo{{Name: "api", Path: src}, {Name: "web", Path: filepath.Join(src, "web")}}})
	r := NewRouter(RouterConfig{RootDir: root, ProjectManager: pm})
	r.mu.Lock()
	r.tasks["TASK-2"] = &types.Task{ID: "TASK-2", Title: "Mono change", AssignedRepos: []string{"api", "web"}, Metadata: map[string]string{}}
	r.mu.Unlock()

	dir := r.resolveWorkDir(r.taskSnapshot("TASK-2"))
	if dir != filepath.Join(r.taskWorktreeRoot("TASK-2"), "api") {
		t.Fatalf("both repos should share one checkout, agent dir = %s", dir)
	}
	_ = os.WriteFile(filepath.Join(dir, "web", "app.ts"), []byte("export const x = 1\n"), 0o644)

	var resp struct {
		Repos []repoDiffResult `json:"repos"`
	}
	_ = json.Unmarshal(do(r, http.MethodGet, "/api/v1/tasks/TASK-2/diff", "").Body.Bytes(), &resp)
	counts := map[string]int{}
	for _, rd := range resp.Repos {
		if rd.Error != "" {
			t.Fatalf("%s: %s", rd.Repo, rd.Error)
		}
		counts[rd.Repo] = len(rd.Files)
	}
	// The api repo is the whole git repo, so it sees the web change too; web is scoped to web/.
	if counts["web"] != 1 || counts["api"] != 1 {
		t.Fatalf("unexpected file counts: %v", counts)
	}
}

func TestAssignReposValidatesAgainstProjects(t *testing.T) {
	r, _ := worktreeRouter(t)
	task := decodeTask(t, do(r, http.MethodPost, "/api/v1/tasks", `{"title":"Synced from JIRA"}`).Body.Bytes())
	if w := do(r, http.MethodPatch, "/api/v1/tasks/"+task.ID, `{"assigned_repos":["nope"]}`); w.Code != http.StatusBadRequest {
		t.Fatalf("unknown repo must be rejected: %d", w.Code)
	}
	w := do(r, http.MethodPatch, "/api/v1/tasks/"+task.ID, `{"assigned_repos":["app","app"]}`)
	if got := decodeTask(t, w.Body.Bytes()); w.Code != http.StatusOK || len(got.AssignedRepos) != 1 || got.AssignedRepos[0] != "app" {
		t.Fatalf("assign → %d %s", w.Code, w.Body.String())
	}
}
