package api

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	gitwt "github.com/ahmadrezamusthafa/meta-orchestrator/internal/git"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// taskWorktree is one assigned repository's isolated checkout for a task.
type taskWorktree struct {
	Repo     string `json:"repo"`
	Source   string `json:"source_path"`
	Dir      string `json:"worktree_path"` // where the repo lives inside the worktree
	Checkout string `json:"checkout_path"` // worktree root (shared by repos in one git repository)
	SubPath  string `json:"sub_path,omitempty"`
	gitRoot  string
	Branch   string `json:"branch"`
	BaseRef  string `json:"base_ref"`
	Exists   bool   `json:"exists"`
	Dirty    bool   `json:"dirty"`
	Error    string `json:"error,omitempty"`
}

func worktreesEnabled(t *types.Task) bool { return t.Metadata["worktree_enabled"] != "false" }

// taskWorktreeRoot holds every repository worktree of a task.
func (r *Router) taskWorktreeRoot(taskID string) string {
	return filepath.Join(r.cfg.RootDir, ".sdlc", "worktrees", strings.ToLower(taskID))
}

// plannedBranch is the branch a task's worktrees use. Once a worktree exists the name is frozen;
// before that it follows the task, so a JIRA key detected later still lands in the name.
func plannedBranch(t *types.Task) string {
	if b := t.Metadata["worktree_branch"]; b != "" && t.Metadata["worktree_created"] == "true" {
		return b
	}
	return gitwt.TaskBranchName(t.Metadata["jira_key"], t.ID, t.Title)
}

// describeWorktrees reports each assigned repository's worktree without creating anything.
// Repositories registered as subdirectories of one git repository share a single worktree.
func (r *Router) describeWorktrees(ctx context.Context, t *types.Task) []taskWorktree {
	sources := r.repoPaths(t.AssignedRepos)
	root := r.taskWorktreeRoot(t.ID)
	branch := plannedBranch(t)
	checkouts := map[string]string{} // git top-level → worktree checkout dir
	out := make([]taskWorktree, 0, len(t.AssignedRepos))
	for _, repo := range t.AssignedRepos {
		wt := taskWorktree{Repo: repo, Source: sources[repo], Branch: branch, BaseRef: t.Metadata["worktree_base."+repo]}
		top := ""
		if wt.Source != "" {
			top, _ = gitwt.TopLevel(ctx, wt.Source)
		}
		switch {
		case wt.Source == "":
			wt.Error = "repository is not registered in a project"
		case top == "":
			wt.Error = "source is not a git repository"
		default:
			wt.gitRoot = top
			real, err := filepath.EvalSymlinks(wt.Source)
			if err != nil {
				real = wt.Source
			}
			if rel, err := filepath.Rel(top, real); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
				wt.SubPath = rel
			}
			if c, ok := checkouts[top]; ok {
				wt.Checkout = c
			} else {
				wt.Checkout = filepath.Join(root, repo)
				checkouts[top] = wt.Checkout
			}
			wt.Dir = filepath.Join(wt.Checkout, wt.SubPath)
			if dirExists(wt.Checkout) && gitwt.IsRepo(wt.Checkout) {
				wt.Exists = true
				wt.Dirty, _ = gitwt.IsDirty(ctx, wt.Dir)
			}
		}
		out = append(out, wt)
	}
	return out
}

// ensureTaskWorktrees creates any missing worktrees for the task's git repositories. Failures are
// reported per repository; the agent then works in the source checkout for that repo.
func (r *Router) ensureTaskWorktrees(ctx context.Context, taskID string) []taskWorktree {
	r.worktreeMu.Lock()
	defer r.worktreeMu.Unlock()

	t := r.taskSnapshot(taskID)
	if t == nil || !worktreesEnabled(t) || len(t.AssignedRepos) == 0 {
		return nil
	}
	wts := r.describeWorktrees(ctx, t)
	bases := map[string]string{}
	var created, failures []string
	for i := range wts {
		wt := &wts[i]
		if wt.gitRoot == "" {
			continue
		}
		if wt.BaseRef == "" {
			wt.BaseRef = gitwt.DefaultBaseRef(ctx, wt.gitRoot)
		}
		if !wt.Exists {
			ok, err := gitwt.EnsureWorktree(ctx, wt.gitRoot, wt.Checkout, wt.Branch, wt.BaseRef)
			if err != nil {
				wt.Error = err.Error()
				failures = append(failures, fmt.Sprintf("%s: %v", wt.Repo, err))
				continue
			}
			if ok {
				created = append(created, fmt.Sprintf("%s (from %s)", wt.Repo, wt.BaseRef))
			}
			// Later repos in the same git repository now find this checkout.
			for j := i + 1; j < len(wts); j++ {
				if wts[j].Checkout == wt.Checkout {
					wts[j].Exists = true
				}
			}
		}
		wt.Exists, wt.Error = true, ""
		_ = gitwt.ExcludeDependencyDirs(ctx, wt.Checkout) // keeps `git add -A` by the agent off installed dependencies
		bases[wt.Repo] = wt.BaseRef
	}

	r.mu.Lock()
	if task, ok := r.tasks[taskID]; ok {
		if len(bases) > 0 {
			task.Metadata["worktree_branch"] = wts[0].Branch
			task.Metadata["worktree_created"] = "true"
			task.Metadata["worktree_enabled"] = "true"
		}
		for repo, base := range bases {
			task.Metadata["worktree_base."+repo] = base
		}
		if len(failures) > 0 {
			task.Metadata["worktree_error"] = strings.Join(failures, "; ")
		} else {
			delete(task.Metadata, "worktree_error")
		}
	}
	r.mu.Unlock()
	if len(created) > 0 {
		r.addEntry(taskID, types.ConsoleEntry{Kind: types.ConsoleKindSystem, Content: fmt.Sprintf(
			"Created worktree branch %s for %s. The agent edits these copies; your checkouts are untouched.",
			wts[0].Branch, strings.Join(created, ", "))})
		r.saveBoardNow()
	}
	if len(failures) > 0 {
		r.addEntry(taskID, types.ConsoleEntry{Kind: types.ConsoleKindSystem, Content: "Could not create a worktree — " +
			strings.Join(failures, "; ") + ". The agent works in the source checkout for these repositories."})
	}
	return wts
}

// assignRepos replaces the task's repositories. Each must be registered in a project so the agent
// has a real checkout to work in; changes are refused while the agent is running.
func (r *Router) assignRepos(taskID string, repos []string) error {
	seen := map[string]bool{}
	clean := []string{}
	for _, repo := range repos {
		if repo = strings.TrimSpace(repo); repo != "" && !seen[repo] {
			seen[repo] = true
			clean = append(clean, repo)
		}
	}
	known := r.repoPaths(clean)
	for _, repo := range clean {
		if known[repo] == "" {
			return fmt.Errorf("repository %q is not registered in any project", repo)
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.tasks[taskID]
	if !ok {
		return errTaskNotFound
	}
	if t.State == types.TaskStateRunning {
		return fmt.Errorf("pause %s before changing its repositories", taskID)
	}
	t.AssignedRepos = clean
	t.Metadata["repos_assigned_by"] = "operator"
	t.UpdatedAt = time.Now()
	return nil
}

// removeTaskWorktrees detaches clean worktrees when a task is deleted. Worktrees holding
// uncommitted work are left in place, and branches are always kept.
func (r *Router) removeTaskWorktrees(t *types.Task) []string {
	r.worktreeMu.Lock()
	defer r.worktreeMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	var kept []string
	seen := map[string]bool{}
	for _, wt := range r.describeWorktrees(ctx, t) {
		if !wt.Exists || seen[wt.Checkout] {
			continue
		}
		seen[wt.Checkout] = true
		if removed, _ := gitwt.RemoveWorktreeIfClean(ctx, wt.gitRoot, wt.Checkout); !removed {
			kept = append(kept, wt.Checkout)
		}
	}
	return kept
}

// handleTaskWorktree serves GET /api/v1/tasks/{id}/worktree.
func (r *Router) handleTaskWorktree(w http.ResponseWriter, req *http.Request, t *types.Task) {
	if req.Method != http.MethodGet {
		r.writeError(w, http.StatusMethodNotAllowed, "GET required for worktree")
		return
	}
	repos := r.describeWorktrees(req.Context(), t)
	exists := false
	for _, wt := range repos {
		exists = exists || wt.Exists
	}
	status := "PLANNED" // created on the first stage run or chat
	switch {
	case !worktreesEnabled(t):
		status = "DISABLED"
	case exists:
		status = "ACTIVE"
	case len(repos) == 0:
		status = "NO_REPOS"
	}
	path := r.taskWorktreeRoot(t.ID)
	base := ""
	if len(repos) == 1 {
		path, base = repos[0].Dir, repos[0].BaseRef
	}
	r.writeJSON(w, http.StatusOK, map[string]interface{}{
		"task_id":            t.ID,
		"is_worktree":        exists,
		"use_worktree":       worktreesEnabled(t),
		"worktree_path":      path,
		"branch":             plannedBranch(t),
		"base_ref":           base,
		"parallel_isolation": exists,
		"status":             status,
		"assigned_repos":     t.AssignedRepos,
		"repos":              repos,
		"error":              t.Metadata["worktree_error"],
	})
}

// repoDiffResult is one repository's entry in a task diff response.
type repoDiffResult struct {
	Repo string `json:"repo"`
	*gitwt.RepoDiff
	Error string `json:"error,omitempty"`
}

// handleTaskDiff serves GET /api/v1/tasks/{id}/diff?against=base|head[&repo=name].
// "base" is everything the branch adds since it forked from the base branch (what a pull request
// would contain); "head" is uncommitted work only.
func (r *Router) handleTaskDiff(w http.ResponseWriter, req *http.Request, t *types.Task) {
	if req.Method != http.MethodGet {
		r.writeError(w, http.StatusMethodNotAllowed, "GET required for diff")
		return
	}
	against := gitwt.DiffAgainst(req.URL.Query().Get("against"))
	if against == "" {
		against = gitwt.DiffAgainstBase
	}
	if against != gitwt.DiffAgainstBase && against != gitwt.DiffAgainstHead {
		r.writeError(w, http.StatusBadRequest, `against must be "base" or "head"`)
		return
	}
	only := req.URL.Query().Get("repo")
	if only != "" {
		found := false
		for _, repo := range t.AssignedRepos {
			found = found || repo == only
		}
		if !found {
			r.writeError(w, http.StatusBadRequest, fmt.Sprintf("%s is not assigned to %s", only, t.ID))
			return
		}
	}

	ctx, cancel := context.WithTimeout(req.Context(), time.Minute)
	defer cancel()
	results := []repoDiffResult{}
	for _, wt := range r.describeWorktrees(ctx, t) {
		if only != "" && wt.Repo != only {
			continue
		}
		res := repoDiffResult{Repo: wt.Repo}
		switch {
		case wt.Error != "":
			res.Error = wt.Error
		case !wt.Exists:
			res.Error = "no worktree yet — it is created when the task first runs"
		default:
			d, err := gitwt.Diff(ctx, wt.Checkout, wt.BaseRef, against, wt.SubPath)
			if err != nil {
				res.Error = err.Error()
			} else {
				res.RepoDiff = d
			}
		}
		results = append(results, res)
	}
	r.writeJSON(w, http.StatusOK, map[string]interface{}{"task_id": t.ID, "against": against, "repos": results})
}

// reportConflicts warns in the console about conflicts the agent left behind in the task's
// worktrees: an unfinished merge/rebase, paths still marked conflicted, or markers in files. A
// conflict "resolved" without this check could reach a pull request half-done.
func (r *Router) reportConflicts(taskID, turnID string) {
	t := r.taskSnapshot(taskID)
	if t == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	seen := map[string]bool{}
	for _, wt := range r.describeWorktrees(ctx, t) {
		if !wt.Exists || seen[wt.Checkout] {
			continue
		}
		seen[wt.Checkout] = true
		c, err := gitwt.Conflicts(ctx, wt.Checkout, wt.BaseRef)
		if err != nil || !c.Unresolved() {
			continue
		}
		var parts []string
		if c.Operation != "" {
			parts = append(parts, c.Operation+" still in progress")
		}
		if len(c.Unmerged) > 0 {
			parts = append(parts, "unresolved: "+strings.Join(limit(c.Unmerged, 5), ", "))
		}
		if len(c.Markers) > 0 {
			parts = append(parts, "conflict markers left at "+strings.Join(limit(c.Markers, 5), ", "))
		}
		r.addEntry(taskID, types.ConsoleEntry{Kind: types.ConsoleKindSystem, TurnID: turnID, Content: fmt.Sprintf(
			"⚠ %s has an unfinished conflict: %s. Ask the agent to resolve it (each hunk, keeping both sides' intent), or fix it in %s before opening or updating the pull request.",
			wt.Repo, strings.Join(parts, "; "), wt.Checkout)})
	}
}

func limit(items []string, n int) []string {
	if len(items) <= n {
		return items
	}
	return append(append([]string(nil), items[:n]...), fmt.Sprintf("and %d more", len(items)-n))
}
