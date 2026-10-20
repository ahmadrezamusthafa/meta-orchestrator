package api

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// Git run in a terminal on a task worktree is picked up by the next scan, and an opened pull
// request is flagged out of sync without waiting for an agent turn or a page reload.
func TestGitWatcherPicksUpChangesMadeOutsideTheOrchestrator(t *testing.T) {
	r, _ := worktreeRouter(t)
	r.mu.Lock()
	r.tasks["TASK-1"] = &types.Task{ID: "TASK-1", Title: "x", CurrentStageID: "task_implementation",
		State: types.TaskStateWaitingGateApproval, AssignedRepos: []string{"app"},
		Metadata: map[string]string{"pr_url.app": "https://bitbucket.org/ws/app/pull-requests/1"}}
	r.mu.Unlock()
	dir := r.resolveWorkDir(r.taskSnapshot("TASK-1"))

	w := &gitWatch{sigs: map[string]string{}}
	if changed := r.scanGitState(w, false); len(changed) != 0 {
		t.Fatalf("the first scan only records checkouts, got %v", changed)
	}
	if changed := r.scanGitState(w, false); len(changed) != 0 {
		t.Fatalf("nothing changed, got %v", changed)
	}

	_ = os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc edited() {}\n"), 0o644)
	changed := r.scanGitState(w, false)
	if len(changed["TASK-1"]) != 1 || changed["TASK-1"][0] != "app" {
		t.Fatalf("an edit made outside the orchestrator must be reported, got %v", changed)
	}
	runGit(t, dir, "commit", "-q", "-am", "edit from a terminal")
	if changed := r.scanGitState(w, false); len(changed["TASK-1"]) != 1 {
		t.Fatalf("a terminal commit must be reported, got %v", changed)
	}

	deadline := time.Now().Add(5 * time.Second)
	for r.taskSnapshot("TASK-1").Metadata["pr_out_of_sync"] == "" {
		if time.Now().After(deadline) {
			t.Fatal("the opened pull request was not flagged out of sync")
		}
		time.Sleep(50 * time.Millisecond)
	}
}
