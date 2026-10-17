package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/deps"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/projects"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

const depsLock = `{"lockfileVersion": 3, "packages": {"": {"name": "web"}, "node_modules/react": {"version": "18.3.1"}}}`

// fakeNpm is a stand-in npm: `npm view` answers 19.1.0, anything else "installs" react into
// node_modules of the folder it runs in. Every call is appended to the calls file.
func fakeNpm(calls, extra string) string {
	return "#!/bin/sh\necho \"$*\" >> " + calls + "\n" +
		"if [ \"$1\" = view ]; then echo 19.1.0; exit 0; fi\n" + extra +
		"echo \"installing in $PWD\"\n/bin/mkdir -p node_modules/react\n" +
		"echo '{\"version\":\"18.3.1\"}' > node_modules/react/package.json\necho '{}' > node_modules/.package-lock.json\n"
}

// depsRouter registers a git repository "web" (package.json + package-lock.json) in project
// "shop", with a fake npm first on PATH. It returns the router, the source folder and the calls
// log of the fake npm.
func depsRouter(t *testing.T, npmExtra string) (*Router, string, string) {
	t.Helper()
	root, src, bin := t.TempDir(), t.TempDir(), t.TempDir()
	calls := filepath.Join(t.TempDir(), "npm-calls")
	if err := os.WriteFile(filepath.Join(bin, "npm"), []byte(fakeNpm(calls, npmExtra)), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	_ = os.WriteFile(filepath.Join(src, "package.json"), []byte(`{"dependencies": {"react": "^18.2.0"}, "devDependencies": {"vitest": "^1.0.0"}}`), 0o644)
	_ = os.WriteFile(filepath.Join(src, "package-lock.json"), []byte(depsLock), 0o644)
	runGit(t, src, "init", "-q", "-b", "master")
	runGit(t, src, "add", ".")
	runGit(t, src, "commit", "-q", "-m", "init")

	pm := projects.NewProjectManager(root)
	if _, err := pm.Create(&types.Project{Name: "Shop", Repos: []types.ProjectRepo{{Name: "web", Path: src}}}); err != nil {
		t.Fatal(err)
	}
	r := NewRouter(RouterConfig{RootDir: root, ProjectManager: pm})
	t.Cleanup(func() { r.dependencies().jobs.Wait() })
	return r, src, calls
}

func waitDependencyJob(t *testing.T, r *Router, id string) deps.Job {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		w := do(r, http.MethodGet, "/api/v1/dependencies/jobs/"+id, "")
		var j deps.Job
		if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &j) != nil {
			t.Fatalf("job → %d %s", w.Code, w.Body.String())
		}
		if j.Done() {
			return j
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("job %s did not finish", id)
	return deps.Job{}
}

type depsResponse struct {
	ProjectID string `json:"project_id"`
	Repos     []struct {
		Repo       string           `json:"repo"`
		Path       string           `json:"path"`
		Ecosystems []deps.Ecosystem `json:"ecosystems"`
		Job        *deps.Job        `json:"job"`
	} `json:"repos"`
}

func listDeps(t *testing.T, r *Router, query string) depsResponse {
	t.Helper()
	w := do(r, http.MethodGet, "/api/v1/projects/shop/dependencies"+query, "")
	var resp depsResponse
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &resp) != nil {
		t.Fatalf("GET dependencies → %d %s", w.Code, w.Body.String())
	}
	return resp
}

func TestDependenciesContractShape(t *testing.T) {
	r, _, _ := depsRouter(t, "")
	w := do(r, http.MethodGet, "/api/v1/projects/shop/dependencies", "")
	var raw map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil || w.Code != http.StatusOK {
		t.Fatalf("→ %d %s", w.Code, w.Body.String())
	}
	keys := func(m map[string]any) []string {
		var out []string
		for k := range m {
			out = append(out, k)
		}
		sort.Strings(out)
		return out
	}
	if got := keys(raw); !reflect.DeepEqual(got, []string{"project_id", "repos"}) {
		t.Fatalf("top level %v", got)
	}
	repo := raw["repos"].([]any)[0].(map[string]any)
	if got := keys(repo); !reflect.DeepEqual(got, []string{"ecosystems", "job", "path", "repo"}) || repo["job"] != nil {
		t.Fatalf("repo %v", repo)
	}
	eco := repo["ecosystems"].([]any)[0].(map[string]any)
	if got := keys(eco); !reflect.DeepEqual(got, []string{"dependencies", "drift", "ecosystem", "install", "installed", "lockfile", "manager", "manifest", "missing_tool"}) {
		t.Fatalf("ecosystem %v", got)
	}
	if got := keys(eco["install"].(map[string]any)); !reflect.DeepEqual(got, []string{"auto", "command", "note"}) {
		t.Fatalf("install %v", got)
	}
	if got := keys(eco["dependencies"].([]any)[0].(map[string]any)); !reflect.DeepEqual(got, []string{"dev", "installed", "key", "latest", "name", "wanted"}) {
		t.Fatalf("dependency %v", got)
	}
}

func TestDependenciesListAndInstall(t *testing.T) {
	r, src, calls := depsRouter(t, "")
	resp := listDeps(t, r, "")
	if resp.ProjectID != "shop" || len(resp.Repos) != 1 || resp.Repos[0].Repo != "web" || resp.Repos[0].Path != src || resp.Repos[0].Job != nil {
		t.Fatalf("list: %+v", resp)
	}
	eco := resp.Repos[0].Ecosystems[0]
	if eco.Ecosystem != "node" || eco.Manager != "npm" || eco.Lockfile != "package-lock.json" || eco.Installed ||
		!reflect.DeepEqual(eco.Install.Command, []string{"npm", "ci"}) || !eco.Install.Auto || eco.MissingTool != "" {
		t.Fatalf("ecosystem: %+v", eco)
	}
	if eco.Dependencies[0].Name != "react" || !eco.Dependencies[0].Key || eco.Dependencies[0].Latest != "" {
		t.Fatalf("react: %+v", eco.Dependencies[0])
	}

	w := do(r, http.MethodPost, "/api/v1/projects/shop/dependencies/install", `{"repo":"web","ecosystem":"node"}`)
	var job deps.Job
	if w.Code != http.StatusAccepted || json.Unmarshal(w.Body.Bytes(), &job) != nil || job.Package != "repo:web" || job.Action != "install" {
		t.Fatalf("install → %d %s", w.Code, w.Body.String())
	}
	done := waitDependencyJob(t, r, job.ID)
	if done.State != deps.StateSucceeded || !strings.Contains(strings.Join(done.Log, "\n"), "installing in") {
		t.Fatalf("job: %+v", done)
	}
	if _, err := os.Stat(filepath.Join(src, "node_modules", "react", "package.json")); err != nil {
		t.Fatal("npm ci ran in the registered source folder")
	}
	resp = listDeps(t, r, "")
	if j := resp.Repos[0].Job; j == nil || j.ID != job.ID || j.State != deps.StateSucceeded {
		t.Fatalf("the repo reports its last job: %+v", j)
	}
	if !resp.Repos[0].Ecosystems[0].Installed {
		t.Fatalf("installed after npm ci: %+v", resp.Repos[0].Ecosystems[0])
	}
	if data, _ := os.ReadFile(calls); strings.Contains(string(data), "view") {
		t.Fatal("the default GET never asks the registry")
	}

	for _, c := range []struct {
		method, path, body string
		code               int
	}{
		{http.MethodGet, "/api/v1/projects/nope/dependencies", "", http.StatusNotFound},
		{http.MethodPost, "/api/v1/projects/shop/dependencies", "", http.StatusMethodNotAllowed},
		{http.MethodGet, "/api/v1/projects/shop/dependencies/install", "", http.StatusMethodNotAllowed},
		{http.MethodGet, "/api/v1/projects/shop/dependencies/other", "", http.StatusNotFound},
		{http.MethodPost, "/api/v1/projects/shop/dependencies/install", `{"repo":"web"}`, http.StatusBadRequest},
		{http.MethodPost, "/api/v1/projects/shop/dependencies/install", `{"repo":"../etc","ecosystem":"node"}`, http.StatusNotFound},
		{http.MethodPost, "/api/v1/projects/shop/dependencies/install", `{"repo":"web","ecosystem":"go"}`, http.StatusNotFound},
		{http.MethodPost, "/api/v1/projects/shop/dependencies/install", `{"repo":"web","ecosystem":"node","task":"TASK-404"}`, http.StatusNotFound},
		{http.MethodGet, "/api/v1/dependencies/jobs/dep-missing", "", http.StatusNotFound},
	} {
		if w := do(r, c.method, c.path, c.body); w.Code != c.code {
			t.Errorf("%s %s %s → %d, want %d: %s", c.method, c.path, c.body, w.Code, c.code, w.Body.String())
		}
	}
}

func TestDependenciesLatestOnRequestOnly(t *testing.T) {
	r, _, calls := depsRouter(t, "")
	resp := listDeps(t, r, "?latest=1")
	for _, d := range resp.Repos[0].Ecosystems[0].Dependencies {
		if d.Latest != "19.1.0" {
			t.Fatalf("latest via npm view: %+v", d)
		}
	}
	listDeps(t, r, "?latest=1")
	data, _ := os.ReadFile(calls)
	if n := strings.Count(string(data), "view"); n != 2 {
		t.Fatalf("one npm view per dependency, then cached; got %d:\n%s", n, data)
	}
}

func TestDependencyInstallOnePerRepo(t *testing.T) {
	r, _, _ := depsRouter(t, "/bin/sleep 1\n")
	first := do(r, http.MethodPost, "/api/v1/projects/shop/dependencies/install", `{"repo":"web","ecosystem":"node"}`)
	if first.Code != http.StatusAccepted {
		t.Fatalf("first → %d %s", first.Code, first.Body.String())
	}
	second := do(r, http.MethodPost, "/api/v1/projects/shop/dependencies/install", `{"repo":"web","ecosystem":"node"}`)
	var busy struct {
		Error string   `json:"error"`
		Job   deps.Job `json:"job"`
	}
	var started deps.Job
	_ = json.Unmarshal(first.Body.Bytes(), &started)
	if second.Code != http.StatusConflict || json.Unmarshal(second.Body.Bytes(), &busy) != nil || busy.Job.ID != started.ID || busy.Error == "" {
		t.Fatalf("second → %d %s", second.Code, second.Body.String())
	}
	waitDependencyJob(t, r, started.ID)
}

func TestDependencyInstallMissingTool(t *testing.T) {
	r, _, _ := depsRouter(t, "")
	t.Setenv("PATH", t.TempDir()) // no npm anywhere
	w := do(r, http.MethodPost, "/api/v1/projects/shop/dependencies/install", `{"repo":"web","ecosystem":"node"}`)
	var resp struct {
		Error       string       `json:"error"`
		Install     deps.Install `json:"install"`
		MissingTool string       `json:"missing_tool"`
	}
	if w.Code != http.StatusNotImplemented || json.Unmarshal(w.Body.Bytes(), &resp) != nil {
		t.Fatalf("→ %d %s", w.Code, w.Body.String())
	}
	if resp.MissingTool != "node" || !reflect.DeepEqual(resp.Install.Command, []string{"npm", "ci"}) || !strings.Contains(resp.Error, "node") {
		t.Fatalf("501 body: %+v", resp)
	}
	if eco := listDeps(t, r, "").Repos[0].Ecosystems[0]; eco.MissingTool != "node" {
		t.Fatalf("GET reports the missing tool: %+v", eco)
	}
}

func TestDependenciesInTaskWorktree(t *testing.T) {
	r, src, _ := depsRouter(t, "")
	r.mu.Lock()
	r.tasks["TASK-1"] = &types.Task{ID: "TASK-1", Title: "Bump deps", CurrentStageID: "task_implementation",
		State: types.TaskStatePending, AssignedRepos: []string{"web"}, Metadata: map[string]string{}}
	r.tasks["TASK-2"] = &types.Task{ID: "TASK-2", Title: "Other repo", CurrentStageID: "task_implementation",
		State: types.TaskStatePending, AssignedRepos: []string{"api"}, Metadata: map[string]string{}}
	r.mu.Unlock()

	if resp := listDeps(t, r, "?task=TASK-1"); len(resp.Repos) != 0 {
		t.Fatalf("no worktree yet → nothing to show for the task: %+v", resp)
	}
	if w := do(r, http.MethodPost, "/api/v1/projects/shop/dependencies/install", `{"repo":"web","ecosystem":"node","task":"TASK-1"}`); w.Code != http.StatusBadRequest ||
		!strings.Contains(w.Body.String(), "no worktree") {
		t.Fatalf("install before the worktree exists → %d %s", w.Code, w.Body.String())
	}
	if w := do(r, http.MethodPost, "/api/v1/projects/shop/dependencies/install", `{"repo":"web","ecosystem":"node","task":"TASK-2"}`); w.Code != http.StatusBadRequest ||
		!strings.Contains(w.Body.String(), "not assigned") {
		t.Fatalf("a task's worktree must be its own repo's → %d %s", w.Code, w.Body.String())
	}
	if w := do(r, http.MethodGet, "/api/v1/projects/shop/dependencies?task=TASK-404", ""); w.Code != http.StatusNotFound {
		t.Fatalf("unknown task → %d", w.Code)
	}

	wt := r.resolveWorkDir(r.taskSnapshot("TASK-1"))
	if wt != filepath.Join(r.taskWorktreeRoot("TASK-1"), "web") {
		t.Fatalf("worktree %s", wt)
	}
	resp := listDeps(t, r, "?task=TASK-1")
	if len(resp.Repos) != 1 || resp.Repos[0].Path != wt || len(resp.Repos[0].Ecosystems) != 1 {
		t.Fatalf("task listing reads the worktree: %+v", resp)
	}
	w := do(r, http.MethodPost, "/api/v1/projects/shop/dependencies/install", `{"repo":"web","ecosystem":"node","task":"TASK-1"}`)
	var job deps.Job
	if w.Code != http.StatusAccepted || json.Unmarshal(w.Body.Bytes(), &job) != nil {
		t.Fatalf("install in worktree → %d %s", w.Code, w.Body.String())
	}
	if done := waitDependencyJob(t, r, job.ID); done.State != deps.StateSucceeded {
		t.Fatalf("job: %+v", done)
	}
	if _, err := os.Stat(filepath.Join(wt, "node_modules", "react")); err != nil {
		t.Fatal("installed into the task worktree")
	}
	if _, err := os.Stat(filepath.Join(src, "node_modules")); err == nil {
		t.Fatal("the source checkout must stay untouched")
	}
}

func TestDependencyUpgradeCreatesTask(t *testing.T) {
	r, _, _ := depsRouter(t, "")
	w := do(r, http.MethodPost, "/api/v1/projects/shop/dependencies/upgrade",
		`{"repo":"web","ecosystem":"node","name":"react","target":"19.1.0","note":"check the new JSX transform"}`)
	var resp struct {
		Task types.Task `json:"task"`
	}
	if w.Code != http.StatusCreated || json.Unmarshal(w.Body.Bytes(), &resp) != nil {
		t.Fatalf("upgrade → %d %s", w.Code, w.Body.String())
	}
	task := resp.Task
	if task.Title != "Upgrade react to 19.1.0 in web" || !reflect.DeepEqual(task.AssignedRepos, []string{"web"}) ||
		task.CurrentStageID != "task_implementation" || task.ID == "" {
		t.Fatalf("task: %+v", task)
	}
	for _, want := range []string{"npm install react@19.1.0", "package-lock.json", "react-dom", "breaking change", "Operator note: check the new JSX transform"} {
		if !strings.Contains(task.Description, want) {
			t.Fatalf("description lacks %q:\n%s", want, task.Description)
		}
	}
	if got := r.taskSnapshot(task.ID); got == nil || got.Title != task.Title {
		t.Fatal("the task is on the board")
	}
	if w := do(r, http.MethodGet, "/api/v1/tasks", ""); !strings.Contains(w.Body.String(), "Upgrade react to 19.1.0 in web") {
		t.Fatalf("task list: %s", w.Body.String())
	}

	for body, code := range map[string]int{
		`{"repo":"web","ecosystem":"node","name":"react"}`:                           http.StatusBadRequest,
		`{"repo":"web","ecosystem":"node","name":"react","target":"19\n; rm -rf /"}`: http.StatusBadRequest,
		`{"repo":"web","ecosystem":"node","name":"--global","target":"1"}`:           http.StatusBadRequest,
		`{"repo":"web","ecosystem":"node","name":"left-pad","target":"2.0.0"}`:       http.StatusNotFound,
		`{"repo":"web","ecosystem":"go","name":"react","target":"19.1.0"}`:           http.StatusNotFound,
		`{"repo":"api","ecosystem":"node","name":"react","target":"19.1.0"}`:         http.StatusNotFound,
		`not json`: http.StatusBadRequest,
	} {
		if w := do(r, http.MethodPost, "/api/v1/projects/shop/dependencies/upgrade", body); w.Code != code {
			t.Errorf("%s → %d, want %d: %s", body, w.Code, code, w.Body.String())
		}
	}
}
