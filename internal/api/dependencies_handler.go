package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/deps"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/fsm"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// Repository dependencies: list each registered repository's project dependencies per ecosystem,
// install them exactly as the lockfile says (which is also the rollback for local drift), and turn
// a version change into a task so it is made in a worktree and reviewed in a pull request.
//
//	GET  /api/v1/projects/{id}/dependencies[?task={taskId}][&latest=1]
//	POST /api/v1/projects/{id}/dependencies/install   {"repo","ecosystem","task"?}
//	POST /api/v1/projects/{id}/dependencies/upgrade   {"repo","ecosystem","name","target","note"?}
//	GET  /api/v1/dependencies/jobs/{id}

// eventDependencyJob carries a deps.Job on every state change and while it logs.
const eventDependencyJob types.EventType = "dependency.job"

// upgradeStartStage is where an upgrade task starts when the default workflow has it.
const upgradeStartStage = "task_implementation"

const maxDependencyBody = 64 << 10

// repoDeps is one repository in the GET response.
type repoDeps struct {
	Repo       string           `json:"repo"`
	Path       string           `json:"path"`
	Ecosystems []deps.Ecosystem `json:"ecosystems"`
	Job        *deps.Job        `json:"job"`
}

type dependencyInstallRequest struct {
	Repo      string `json:"repo"`
	Ecosystem string `json:"ecosystem"`
	Task      string `json:"task,omitempty"`
}

type dependencyUpgradeRequest struct {
	Repo      string `json:"repo"`
	Ecosystem string `json:"ecosystem"`
	Name      string `json:"name"`
	Target    string `json:"target"`
	Note      string `json:"note,omitempty"`
}

// dependencyState is the install service and latest-version cache of one Router. It lives beside
// the Router rather than in it, keyed by the Router, so this feature stays in its own file.
type dependencyState struct {
	jobs   *deps.Service
	latest *deps.Latest
}

var dependencyStates sync.Map // *Router → *dependencyState

func (r *Router) dependencies() *dependencyState {
	if v, ok := dependencyStates.Load(r); ok {
		return v.(*dependencyState)
	}
	st := &dependencyState{jobs: deps.NewService(), latest: deps.NewLatest()}
	st.jobs.OnUpdate = func(j deps.Job) {
		if r.cfg.WSHub != nil {
			r.cfg.WSHub.BroadcastEvent(&types.OrchestratorEvent{Type: eventDependencyJob, Payload: j})
		}
	}
	v, _ := dependencyStates.LoadOrStore(r, st)
	return v.(*dependencyState)
}

func dependencyKey(repo string) string { return "repo:" + repo }

// handleProjectDependencies serves /api/v1/projects/{id}/dependencies[/install|/upgrade].
func (r *Router) handleProjectDependencies(w http.ResponseWriter, req *http.Request, projectID string, rest []string) {
	action := ""
	if len(rest) > 0 {
		action = rest[0]
	}
	if len(rest) > 1 && rest[1] != "" {
		r.writeError(w, http.StatusNotFound, "Not found")
		return
	}
	proj, err := r.cfg.ProjectManager.Get(projectID)
	if err != nil {
		r.writeError(w, http.StatusNotFound, err.Error())
		return
	}
	switch action {
	case "":
		if req.Method != http.MethodGet {
			r.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}
		r.listDependencies(w, req, proj)
	case "install", "upgrade":
		if req.Method != http.MethodPost {
			r.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}
		req.Body = http.MaxBytesReader(w, req.Body, maxDependencyBody)
		if action == "install" {
			r.installDependencies(w, req, proj)
		} else {
			r.upgradeDependency(w, req, proj)
		}
	default:
		r.writeError(w, http.StatusNotFound, "Not found")
	}
}

func (r *Router) listDependencies(w http.ResponseWriter, req *http.Request, proj *types.Project) {
	q := req.URL.Query()
	var task *types.Task
	if id := q.Get("task"); id != "" {
		if task = r.taskSnapshot(id); task == nil {
			r.writeError(w, http.StatusNotFound, fmt.Sprintf("task %s not found", id))
			return
		}
	}
	latest := q.Get("latest") == "1" || q.Get("latest") == "true"
	st := r.dependencies()
	out := []repoDeps{}
	for _, repo := range proj.Repos {
		if task != nil && !slices.Contains(task.AssignedRepos, repo.Name) {
			continue
		}
		dir, _, err := r.dependencyDir(req.Context(), repo, task)
		if err != nil && task != nil {
			continue // the task has no checkout of this repository yet
		}
		rd := repoDeps{Repo: repo.Name, Path: dir, Ecosystems: []deps.Ecosystem{}}
		if err == nil {
			rd.Ecosystems = deps.Scan(dir, deps.Options{})
			if latest {
				st.latest.Fill(req.Context(), dir, rd.Ecosystems, nil)
			}
		}
		if j, ok := st.jobs.Last(dependencyKey(repo.Name)); ok {
			rd.Job = &j
		}
		out = append(out, rd)
	}
	r.writeJSON(w, http.StatusOK, map[string]any{"project_id": proj.ID, "repos": out})
}

func (r *Router) installDependencies(w http.ResponseWriter, req *http.Request, proj *types.Project) {
	var body dependencyInstallRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		r.writeError(w, http.StatusBadRequest, "Invalid request JSON payload")
		return
	}
	if body.Repo == "" || body.Ecosystem == "" {
		r.writeError(w, http.StatusBadRequest, "repo and ecosystem are required")
		return
	}
	repo, ok := projectRepo(proj, body.Repo)
	if !ok {
		r.writeError(w, http.StatusNotFound, fmt.Sprintf("repository %q is not part of project %s", body.Repo, proj.ID))
		return
	}
	var task *types.Task
	if body.Task != "" {
		if task = r.taskSnapshot(body.Task); task == nil {
			r.writeError(w, http.StatusNotFound, fmt.Sprintf("task %s not found", body.Task))
			return
		}
	}
	dir, status, err := r.dependencyDir(req.Context(), repo, task)
	if err != nil {
		r.writeError(w, status, err.Error())
		return
	}
	eco, ok := deps.Find(deps.Scan(dir, deps.Options{}), body.Ecosystem)
	if !ok {
		r.writeError(w, http.StatusNotFound, fmt.Sprintf("no %s manifest found in %s", body.Ecosystem, repo.Name))
		return
	}
	if !eco.Runnable() {
		r.writeJSON(w, http.StatusNotImplemented, map[string]any{
			"error": notRunnableReason(eco), "install": eco.Install, "missing_tool": eco.MissingTool})
		return
	}
	job, err := r.dependencies().jobs.Start(dependencyKey(repo.Name), filepath.Join(dir, filepath.FromSlash(eco.Dir)), eco)
	switch {
	case errors.Is(err, deps.ErrBusy):
		r.writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "job": job})
	case err != nil:
		r.writeError(w, http.StatusInternalServerError, err.Error())
	default:
		r.writeJSON(w, http.StatusAccepted, job)
	}
}

func notRunnableReason(e deps.Ecosystem) string {
	cmd := strings.Join(e.Install.Command, " ")
	switch {
	case e.Missing && e.MissingTool != "":
		return fmt.Sprintf("%s is not available on this machine: install %s from the toolchain, or run `%s` yourself", e.Install.Command[0], e.MissingTool, cmd)
	case e.Missing:
		return fmt.Sprintf("%s is not available on this machine: run `%s` yourself once it is installed", e.Install.Command[0], cmd)
	default:
		return fmt.Sprintf("`%s` is not run automatically: %s", cmd, e.Install.Note)
	}
}

func (r *Router) upgradeDependency(w http.ResponseWriter, req *http.Request, proj *types.Project) {
	var body dependencyUpgradeRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		r.writeError(w, http.StatusBadRequest, "Invalid request JSON payload")
		return
	}
	body.Name, body.Target = strings.TrimSpace(body.Name), strings.TrimSpace(body.Target)
	if body.Repo == "" || body.Ecosystem == "" || body.Name == "" || body.Target == "" {
		r.writeError(w, http.StatusBadRequest, "repo, ecosystem, name and target are required")
		return
	}
	if err := deps.ValidUpgrade(body.Name, body.Target); err != nil {
		r.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(body.Note) > 2000 {
		r.writeError(w, http.StatusBadRequest, "note is limited to 2000 characters")
		return
	}
	repo, ok := projectRepo(proj, body.Repo)
	if !ok {
		r.writeError(w, http.StatusNotFound, fmt.Sprintf("repository %q is not part of project %s", body.Repo, proj.ID))
		return
	}
	dir, status, err := r.dependencyDir(req.Context(), repo, nil)
	if err != nil {
		r.writeError(w, status, err.Error())
		return
	}
	eco, ok := deps.Find(deps.Scan(dir, deps.Options{}), body.Ecosystem)
	if !ok {
		r.writeError(w, http.StatusNotFound, fmt.Sprintf("no %s manifest found in %s", body.Ecosystem, repo.Name))
		return
	}
	i := slices.IndexFunc(eco.Dependencies, func(d deps.Dependency) bool { return d.Name == body.Name })
	if i < 0 {
		r.writeError(w, http.StatusNotFound, fmt.Sprintf("%s is not a dependency in %s", body.Name, eco.Manifest))
		return
	}

	create := CreateTaskRequest{
		Title:         deps.UpgradeTitle(body.Name, body.Target, repo.Name),
		Description:   deps.UpgradeDescription(eco, eco.Dependencies[i], body.Target, repo.Name, body.Note),
		AssignedRepos: []string{repo.Name},
	}
	if wf, err := r.workflowForNewTask(""); err == nil && indexIn(fsm.StageIDs(wf), upgradeStartStage) >= 0 {
		create.ActiveSlice = &types.StageSlice{StartStageID: upgradeStartStage}
	}
	// Create it through POST /api/v1/tasks itself, so the task gets exactly the validation,
	// routing plan, board persistence and broadcast every other task gets.
	payload, _ := json.Marshal(create)
	sub, err := http.NewRequestWithContext(req.Context(), http.MethodPost, "/api/v1/tasks", bytes.NewReader(payload))
	if err != nil {
		r.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	sub.Header.Set("Content-Type", "application/json")
	rec := &bufferedResponse{header: http.Header{}}
	r.handleTasks(rec, sub)
	if rec.status != http.StatusCreated {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(rec.status)
		_, _ = w.Write(rec.body.Bytes())
		return
	}
	var task types.Task
	if err := json.Unmarshal(rec.body.Bytes(), &task); err != nil {
		r.writeError(w, http.StatusInternalServerError, "task created but its response could not be read")
		return
	}
	r.writeJSON(w, http.StatusCreated, map[string]any{"task": task})
}

// handleDependencyJob serves GET /api/v1/dependencies/jobs/{id}.
func (r *Router) handleDependencyJob(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		r.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	id := strings.Trim(strings.TrimPrefix(req.URL.Path, "/api/v1/dependencies/jobs/"), "/")
	job, ok := r.dependencies().jobs.Get(id)
	if id == "" || !ok {
		r.writeError(w, http.StatusNotFound, "job not found")
		return
	}
	r.writeJSON(w, http.StatusOK, job)
}

func projectRepo(p *types.Project, name string) (types.ProjectRepo, bool) {
	for _, r := range p.Repos {
		if r.Name == name {
			return r, true
		}
	}
	return types.ProjectRepo{}, false
}

// dependencyDir is where a repository's dependencies are read and installed: its registered
// source path, or — for a task — the task's own worktree of it. Paths never come from the
// request: the source is the projects manager's, the worktree must sit inside the task's
// worktree root.
func (r *Router) dependencyDir(ctx context.Context, repo types.ProjectRepo, task *types.Task) (string, int, error) {
	if task == nil || !worktreesEnabled(task) {
		if task != nil && !slices.Contains(task.AssignedRepos, repo.Name) {
			return "", http.StatusBadRequest, fmt.Errorf("repository %s is not assigned to task %s", repo.Name, task.ID)
		}
		if repo.Path == "" || !isDirectory(repo.Path) {
			return "", http.StatusBadRequest, fmt.Errorf("repository %s has no source folder on this machine", repo.Name)
		}
		return repo.Path, 0, nil
	}
	if !slices.Contains(task.AssignedRepos, repo.Name) {
		return "", http.StatusBadRequest, fmt.Errorf("repository %s is not assigned to task %s", repo.Name, task.ID)
	}
	for _, wt := range r.describeWorktrees(ctx, task) {
		if wt.Repo != repo.Name {
			continue
		}
		if !wt.Exists || !isDirectory(wt.Dir) {
			return "", http.StatusBadRequest, fmt.Errorf("task %s has no worktree of %s yet: run a stage first", task.ID, repo.Name)
		}
		if !within(r.taskWorktreeRoot(task.ID), wt.Dir) {
			return "", http.StatusBadRequest, fmt.Errorf("worktree of %s is outside task %s's worktree folder", repo.Name, task.ID)
		}
		return wt.Dir, 0, nil
	}
	return "", http.StatusBadRequest, fmt.Errorf("task %s has no worktree of %s", task.ID, repo.Name)
}

// within reports whether path lies inside root once symlinks are resolved.
func within(root, path string) bool {
	if real, err := filepath.EvalSymlinks(root); err == nil {
		root = real
	}
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(root, real)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func isDirectory(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

// bufferedResponse captures a handler's response so another handler can reuse it.
type bufferedResponse struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (b *bufferedResponse) Header() http.Header { return b.header }

func (b *bufferedResponse) WriteHeader(status int) {
	if b.status == 0 {
		b.status = status
	}
}

func (b *bufferedResponse) Write(p []byte) (int, error) {
	if b.status == 0 {
		b.status = http.StatusOK
	}
	return b.body.Write(p)
}
