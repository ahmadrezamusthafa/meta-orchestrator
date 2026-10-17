package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/pullrequest"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// fakeURLSecret stands in for a token embedded in a remote URL; it is not a real credential.
const fakeURLSecret = "FAKESECRET123"

// Remote URLs may embed credentials (https://user:token@host/…): neither a draft's blocker nor a
// failed push may hand them to the UI or the console.
func TestPullRequestErrorsNeverEchoRemoteCredentials(t *testing.T) {
	r, src := worktreeRouter(t)
	runGit(t, src, "remote", "add", "origin", "https://bitbucket.org/ws/app.git")
	runGit(t, src, "config", "remote.origin.pushurl", "https://user:"+fakeURLSecret+"@127.0.0.1:1/ws/app.git")
	runGit(t, src, "config", "user.name", "t")
	runGit(t, src, "config", "user.email", "t@t")
	r.prClient = &pullrequest.Client{BaseURL: "http://127.0.0.1:1"}
	if _, err := r.cfg.ConnectorsManager.UpdateConnector(types.ConnectorItem{ID: "bitbucket", Enabled: true, Username: "u", APIToken: "tok"}); err != nil {
		t.Fatal(err)
	}
	r.mu.Lock()
	r.tasks["TASK-1"] = &types.Task{ID: "TASK-1", Title: "x", CurrentStageID: "task_implementation",
		State: types.TaskStateWaitingGateApproval, AssignedRepos: []string{"app"}, Metadata: map[string]string{}}
	r.mu.Unlock()
	dir := r.resolveWorkDir(r.taskSnapshot("TASK-1"))
	_ = os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644)

	w := do(r, http.MethodPost, "/api/v1/tasks/TASK-1/pull-request", `{"repo":"app"}`)
	if w.Code == http.StatusOK {
		t.Fatalf("push to an unreachable remote must fail: %s", w.Body.String())
	}
	if strings.Contains(w.Body.String(), fakeURLSecret) {
		t.Fatalf("push error leaks the remote's credentials: %s", w.Body.String())
	}

	// An origin the orchestrator cannot parse is reported as a blocker, without its credentials.
	runGit(t, src, "remote", "set-url", "origin", "https://user:"+fakeURLSecret+"@bitbucket.org")
	w = do(r, http.MethodGet, "/api/v1/tasks/TASK-1/pull-request", "")
	if strings.Contains(w.Body.String(), fakeURLSecret) {
		t.Fatalf("draft blocker leaks the remote's credentials: %s", w.Body.String())
	}
	for _, e := range r.taskSnapshot("TASK-1").Metadata {
		if strings.Contains(e, fakeURLSecret) {
			t.Fatalf("task metadata holds the remote's credentials")
		}
	}
}

func TestRedactURLCredentials(t *testing.T) {
	for in, want := range map[string]string{
		"fatal: unable to access 'https://user:" + fakeURLSecret + "@host/o/r.git/': refused": "fatal: unable to access 'https://***@host/o/r.git/': refused",
		"remote \"https://x-token-auth:" + fakeURLSecret + "@bitbucket.org\" has no path":     "remote \"https://***@bitbucket.org\" has no path",
		"git@github.com:o/r.git rejected": "git@github.com:o/r.git rejected",
		"https://github.com/o/r.git":      "https://github.com/o/r.git",
	} {
		if got := redactURLCredentials(in); got != want {
			t.Errorf("redactURLCredentials(%q) = %q, want %q", in, got, want)
		}
	}
}
