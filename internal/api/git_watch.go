package api

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	gitwt "github.com/ahmadrezamusthafa/meta-orchestrator/internal/git"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// Git used outside the orchestrator (a commit, rebase, push or force push in a terminal) must
// show up at once, or the board and the task tabs describe a branch that no longer exists. The
// watcher fingerprints every task worktree every gitWatchInterval and, on a change, broadcasts
// EventWorktreeChanged and recomputes the pull request sync state. Pushes made from another clone
// only reach the local tracking ref through a fetch, so tasks with an opened pull request also
// fetch their branch every gitFetchEvery ticks.
const (
	gitWatchInterval = 3 * time.Second
	gitFetchEvery    = 10 // ticks: 30s
)

// gitWatch holds the last fingerprint per task checkout. Only the watcher goroutine touches it.
type gitWatch struct {
	sigs map[string]string // taskID + "\x00" + checkout → signature
	tick int
}

func (r *Router) startGitWatcher() {
	go func() {
		w := &gitWatch{sigs: map[string]string{}}
		ticker := time.NewTicker(gitWatchInterval)
		defer ticker.Stop()
		for range ticker.C {
			w.tick++
			r.scanGitState(w, w.tick%gitFetchEvery == 0)
		}
	}()
}

// watchedCheckout is one git checkout under a task's worktree root.
type watchedCheckout struct {
	taskID, repo, dir, branch string
	prOpened                  bool
}

// watchedCheckouts lists the existing worktree checkouts of every task, without running git.
func (r *Router) watchedCheckouts() []watchedCheckout {
	r.mu.Lock()
	tasks := make([]*types.Task, 0, len(r.tasks))
	for _, t := range r.tasks {
		tasks = append(tasks, cloneTask(t))
	}
	r.mu.Unlock()
	var out []watchedCheckout
	for _, t := range tasks {
		root := r.taskWorktreeRoot(t.ID)
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		opened := false
		for k := range t.Metadata {
			if strings.HasPrefix(k, "pr_url.") {
				opened = true
				break
			}
		}
		branch := plannedBranch(t)
		for _, e := range entries {
			dir := filepath.Join(root, e.Name())
			if !e.IsDir() || !pathExists(filepath.Join(dir, ".git")) {
				continue
			}
			out = append(out, watchedCheckout{taskID: t.ID, repo: e.Name(), dir: dir, branch: branch, prOpened: opened})
		}
	}
	return out
}

// scanGitState fingerprints every checkout and announces the ones that changed since the last
// scan, returning the changed repositories per task. The first sighting of a checkout only
// records it.
func (r *Router) scanGitState(w *gitWatch, fetch bool) map[string][]string {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	changed := map[string][]string{} // taskID → repos
	seen := map[string]bool{}
	for _, c := range r.watchedCheckouts() {
		if fetch && c.prOpened {
			_ = gitwt.FetchBranch(ctx, c.dir, c.branch) // offline or no access: keep the last known state
		}
		sig, err := gitwt.StateSignature(ctx, c.dir, c.branch)
		if err != nil {
			continue
		}
		key := c.taskID + "\x00" + c.dir
		seen[key] = true
		if prev, ok := w.sigs[key]; ok && prev != sig {
			changed[c.taskID] = append(changed[c.taskID], c.repo)
		}
		w.sigs[key] = sig
	}
	for key := range w.sigs {
		if !seen[key] {
			delete(w.sigs, key)
		}
	}
	for taskID, repos := range changed {
		sort.Strings(repos)
		r.onWorktreeChanged(taskID, repos)
	}
	return changed
}

// onWorktreeChanged tells open task views to reload and refreshes the pull request sync badge.
func (r *Router) onWorktreeChanged(taskID string, repos []string) {
	r.broadcast(taskID, types.EventWorktreeChanged, map[string]interface{}{"repos": repos})
	t := r.taskSnapshot(taskID)
	if t == nil || t.State == types.TaskStateRunning {
		return // notePRSync runs after every agent turn
	}
	key := "prsync:" + taskID
	if _, busy := r.busyOps.LoadOrStore(key, true); busy {
		return
	}
	go func() {
		defer r.busyOps.Delete(key)
		r.notePRSync(taskID, "")
	}()
}

// fetchTaskBranches refreshes origin's copy of a task's branches in the background, so the Pull
// Request tab shows pushes made from another clone; the watcher announces what the fetch changed.
func (r *Router) fetchTaskBranches(taskID string) {
	key := "fetch:" + taskID
	if _, busy := r.busyOps.LoadOrStore(key, true); busy {
		return
	}
	go func() {
		defer r.busyOps.Delete(key)
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		for _, c := range r.watchedCheckouts() {
			if c.taskID == taskID {
				_ = gitwt.FetchBranch(ctx, c.dir, c.branch)
			}
		}
	}()
}

func pathExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
