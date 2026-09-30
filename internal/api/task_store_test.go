package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/connectors"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

func persistentRouter(t *testing.T, root string, mgr *connectors.Manager) *Router {
	t.Helper()
	r := NewRouter(RouterConfig{RootDir: root, TaskStorePath: filepath.Join(root, ".sdlc", "tasks.json"), ConnectorsManager: mgr})
	t.Cleanup(func() { _ = r.Close() })
	return r
}

func decodeTask(t *testing.T, body []byte) types.Task {
	t.Helper()
	var task types.Task
	if err := json.Unmarshal(body, &task); err != nil {
		t.Fatalf("decode task: %v (%s)", err, body)
	}
	return task
}

func TestBoardStartsEmptyAndSurvivesRestart(t *testing.T) {
	root := t.TempDir()
	r := persistentRouter(t, root, nil)
	if w := do(r, http.MethodGet, "/api/v1/tasks", ""); strings.TrimSpace(w.Body.String()) != "[]" {
		t.Fatalf("a fresh board must be empty, got %s", w.Body.String())
	}
	first := decodeTask(t, do(r, http.MethodPost, "/api/v1/tasks", `{"title":"Add refund endpoint"}`).Body.Bytes())
	second := decodeTask(t, do(r, http.MethodPost, "/api/v1/tasks", `{"title":"Refund audit log"}`).Body.Bytes())
	if first.ID == second.ID {
		t.Fatalf("task IDs collided: %s", first.ID)
	}
	if first.TokenUsage.TotalTokens != 0 {
		t.Fatalf("a new task has spent no tokens, got %d", first.TokenUsage.TotalTokens)
	}
	if w := do(r, http.MethodDelete, "/api/v1/tasks/"+first.ID, ""); w.Code != http.StatusOK {
		t.Fatalf("delete → %d %s", w.Code, w.Body.String())
	}
	_ = r.Close()

	restarted := persistentRouter(t, root, nil)
	if restarted.taskSnapshot(second.ID) == nil {
		t.Fatalf("%s was lost across a restart", second.ID)
	}
	if restarted.taskSnapshot(first.ID) != nil {
		t.Fatalf("deleted task %s came back after restart", first.ID)
	}
	third := decodeTask(t, do(restarted, http.MethodPost, "/api/v1/tasks", `{"title":"Third"}`).Body.Bytes())
	if third.ID == first.ID || third.ID == second.ID {
		t.Fatalf("restarted board reused ID %s", third.ID)
	}
}

func TestCorruptBoardIsMovedAsideNotOverwritten(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".sdlc", "tasks.json")
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	_ = os.WriteFile(path, []byte("{not json"), 0o600)
	persistentRouter(t, root, nil)
	backups, _ := filepath.Glob(path + ".corrupt-*")
	if len(backups) != 1 {
		t.Fatalf("expected the unreadable board to be preserved, found %v", backups)
	}
}

func TestDeletingPrerequisiteReleasesDependents(t *testing.T) {
	r := hermeticRouter(t)
	parent := decodeTask(t, do(r, http.MethodPost, "/api/v1/tasks", `{"title":"parent"}`).Body.Bytes())
	child := decodeTask(t, do(r, http.MethodPost, "/api/v1/tasks", `{"title":"child","dependencies":["`+parent.ID+`"]}`).Body.Bytes())
	if child.State != types.TaskStateWaitingDependency {
		t.Fatalf("child should wait on parent, got %s", child.State)
	}
	do(r, http.MethodDelete, "/api/v1/tasks/"+parent.ID, "")
	if got := r.taskSnapshot(child.ID); got.State != types.TaskStatePending || len(got.Dependencies) != 0 {
		t.Fatalf("child should be released to PENDING, got %s deps=%v", got.State, got.Dependencies)
	}
}

// fakeJira serves a JIRA search whose issues the test can change between syncs.
type fakeJira struct {
	body atomic.Value
}

func newFakeJira(t *testing.T, body string) (*fakeJira, *connectors.Manager) {
	t.Helper()
	isolateJiraEnv(t)
	f := &fakeJira{}
	f.body.Store(body)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(req.URL.Path, "/search"):
			_, _ = w.Write([]byte(f.body.Load().(string)))
		case strings.HasSuffix(req.URL.Path, "/issue/PAY-2"):
			_, _ = w.Write([]byte(`{"key":"PAY-2","fields":{"summary":"Second","status":{"name":"In Progress"}}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	mgr := connectors.NewManager(t.TempDir())
	_ = mgr.UpdateJira(types.JiraConfig{Enabled: true, BaseURL: srv.URL, Username: "dev@test.local", APIToken: "test-token", ProjectKey: "PAY"})
	return f, mgr
}

const twoIssues = `{"issues":[
	{"key":"PAY-1","fields":{"summary":"First","status":{"name":"To Do"},"priority":{"name":"High"}}},
	{"key":"PAY-2","fields":{"summary":"Second","status":{"name":"In Progress"}}}]}`

func TestJiraSyncLoadsIssuesOntoTheBoard(t *testing.T) {
	jira, mgr := newFakeJira(t, twoIssues)
	cfg := types.DefaultJiraSyncConfig()
	cfg.StatusStageMap = map[string]string{"In Progress": "task_implementation"}
	cfg.AssignedRepos = []string{"backend-core"}
	if _, err := mgr.UpdateJiraSyncConfig(cfg); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	r := persistentRouter(t, root, mgr)

	st, err := r.syncJiraBoard(context.Background())
	if err != nil || st.Created != 2 || st.Fetched != 2 {
		t.Fatalf("first sync: %+v err=%v", st, err)
	}
	byKey := map[string]*types.Task{}
	r.mu.RLock()
	for k, v := range r.jiraTaskIndexLocked() {
		byKey[k] = cloneTask(v)
	}
	r.mu.RUnlock()
	if byKey["PAY-1"].CurrentStageID != "prd_discovery" || byKey["PAY-2"].CurrentStageID != "task_implementation" {
		t.Fatalf("status → stage rules not applied: %s / %s", byKey["PAY-1"].CurrentStageID, byKey["PAY-2"].CurrentStageID)
	}
	if byKey["PAY-1"].State != types.TaskStatePending {
		t.Fatalf("synced tasks must wait for an operator to run them, got %s", byKey["PAY-1"].State)
	}

	// Re-sync is idempotent; changed issues refresh their task.
	jira.body.Store(strings.Replace(twoIssues, `"First"`, `"First (renamed)"`, 1))
	st, _ = r.syncJiraBoard(context.Background())
	if st.Created != 0 || st.Updated != 1 {
		t.Fatalf("second sync should only update PAY-1: %+v", st)
	}
	if got := r.taskSnapshot(byKey["PAY-1"].ID); !strings.Contains(got.Title, "renamed") {
		t.Fatalf("title not refreshed: %s", got.Title)
	}

	// A task the operator deletes is not re-created by the next sync...
	do(r, http.MethodDelete, "/api/v1/tasks/"+byKey["PAY-2"].ID, "")
	if st, _ = r.syncJiraBoard(context.Background()); st.Skipped != 1 || st.Created != 0 {
		t.Fatalf("dismissed issue should be skipped: %+v", st)
	}
	// ...and the dismissal survives a restart.
	_ = r.Close()
	r = persistentRouter(t, root, mgr)
	if st, _ = r.syncJiraBoard(context.Background()); st.Skipped != 1 {
		t.Fatalf("dismissal lost across restart: %+v", st)
	}

	// An explicit import brings it back, and a second import is rejected as a duplicate.
	if w := do(r, http.MethodPost, "/api/v1/connectors/jira/import", `{"issue_key":"PAY-2"}`); w.Code != http.StatusCreated {
		t.Fatalf("re-import → %d %s", w.Code, w.Body.String())
	}
	if w := do(r, http.MethodPost, "/api/v1/connectors/jira/import", `{"issue_key":"pay-2"}`); w.Code != http.StatusConflict {
		t.Fatalf("duplicate import → %d %s", w.Code, w.Body.String())
	}
}

func TestJiraSyncWithoutCredentialsCreatesNothing(t *testing.T) {
	isolateJiraEnv(t)
	r := NewRouter(RouterConfig{RootDir: t.TempDir(), ConnectorsManager: connectors.NewManager(t.TempDir())})
	w := do(r, http.MethodPost, "/api/v1/connectors/jira/sync/run", "")
	if w.Code != http.StatusPreconditionFailed {
		t.Fatalf("sync without credentials → %d %s", w.Code, w.Body.String())
	}
	if n := len(r.tasks); n != 0 {
		t.Fatalf("no fabricated tasks allowed, got %d", n)
	}
	if w := do(r, http.MethodPost, "/api/v1/connectors/jira/import", `{"issue_key":"PAY-9"}`); w.Code != http.StatusPreconditionFailed {
		t.Fatalf("import without credentials → %d %s", w.Code, w.Body.String())
	}
}

func TestJiraSyncRulesValidation(t *testing.T) {
	_, mgr := newFakeJira(t, twoIssues)
	r := NewRouter(RouterConfig{RootDir: t.TempDir(), ConnectorsManager: mgr})
	if w := do(r, http.MethodPut, "/api/v1/connectors/jira/sync", `{"status_stage_map":{"Done":"nowhere"}}`); w.Code != http.StatusBadRequest {
		t.Fatalf("unknown stage should be rejected, got %d", w.Code)
	}
	w := do(r, http.MethodPut, "/api/v1/connectors/jira/sync", `{"enabled":true,"jql":"project = PAY","interval_seconds":5,"status_stage_map":{"In Review":"e2e_validation"}}`)
	var settings types.JiraSyncSettings
	_ = json.Unmarshal(w.Body.Bytes(), &settings)
	if w.Code != http.StatusOK || settings.Config.IntervalSeconds != 60 || settings.Config.StatusStageMap["in review"] != "e2e_validation" {
		t.Fatalf("rules not normalized: %d %+v", w.Code, settings.Config)
	}
	if !settings.Status.Connected {
		t.Fatalf("status should report the live connection")
	}
}

func TestJiraSyncSkipsFinishedIssuesAndRecordsEpic(t *testing.T) {
	var gotJQL atomic.Value
	isolateJiraEnv(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		gotJQL.Store(req.URL.Query().Get("jql"))
		_, _ = w.Write([]byte(`{"issues":[
			{"key":"PAY-2","fields":{"summary":"Story","status":{"name":"To Do"},
				"parent":{"key":"PAY-1","fields":{"summary":"Checkout revamp","issuetype":{"name":"Epic","hierarchyLevel":1}}}}},
			{"key":"PAY-3","fields":{"summary":"Wrapped up","status":{"name":"Finish"}}}]}`))
	}))
	defer srv.Close()
	mgr := connectors.NewManager(t.TempDir())
	_ = mgr.UpdateJira(types.JiraConfig{Enabled: true, BaseURL: srv.URL, Username: "dev@test.local", APIToken: "test-token"})
	r := NewRouter(RouterConfig{RootDir: t.TempDir(), ConnectorsManager: mgr})

	st, err := r.syncJiraBoard(context.Background())
	if err != nil || st.Created != 1 {
		t.Fatalf("only the open story should sync: %+v err=%v", st, err)
	}
	if jql, _ := gotJQL.Load().(string); !strings.Contains(jql, "statusCategory != Done") {
		t.Fatalf("sync JQL must exclude finished issues, sent %q", jql)
	}
	r.mu.RLock()
	task := cloneTask(r.jiraTaskIndexLocked()["PAY-2"])
	r.mu.RUnlock()
	if task.Metadata["jira_epic_key"] != "PAY-1" || task.Metadata["jira_epic_name"] != "Checkout revamp" {
		t.Fatalf("epic not recorded: %v", task.Metadata)
	}
}

func taskActivity(t *testing.T, r *Router, taskID string) activityResp {
	t.Helper()
	w := do(r, http.MethodGet, "/api/v1/tasks/"+taskID+"/activity", "")
	if w.Code != http.StatusOK {
		t.Fatalf("activity → %d %s", w.Code, w.Body.String())
	}
	var a activityResp
	_ = json.Unmarshal(w.Body.Bytes(), &a)
	return a
}

func TestConsoleTranscriptSurvivesRestart(t *testing.T) {
	root := t.TempDir()
	r := persistentRouter(t, root, nil)
	task := decodeTask(t, do(r, http.MethodPost, "/api/v1/tasks", `{"title":"Refund endpoint"}`).Body.Bytes())
	r.addEntry(task.ID, types.ConsoleEntry{Kind: types.ConsoleKindUser, TurnID: "turn_1", Content: "why?"})
	live := r.addEntry(task.ID, types.ConsoleEntry{Kind: types.ConsoleKindAssistant, TurnID: "turn_1", Status: types.ConsoleStreaming})
	r.appendDelta(task.ID, live.ID, "assistant", "because")
	r.addEntry(task.ID, types.ConsoleEntry{Kind: types.ConsoleKindApproval, TurnID: "turn_1",
		Approval: &types.ConsoleApproval{ID: "apr_1", Decision: types.ApprovalPending}})
	r.console.mu.Lock()
	r.console.get(task.ID).sessionID = "sess-1"
	r.console.markDirty(task.ID)
	r.console.mu.Unlock()
	_ = r.Close()

	restarted := persistentRouter(t, root, nil)
	a := taskActivity(t, restarted, task.ID)
	var hasUser bool
	for _, e := range a.Entries {
		switch {
		case e.Kind == types.ConsoleKindUser:
			hasUser = e.Content == "why?"
		case e.ID == live.ID:
			if e.Content != "because" || e.Status != types.ConsoleCancelled {
				t.Fatalf("interrupted stream must keep its text and read as cancelled, got %+v", e)
			}
		case e.Kind == types.ConsoleKindApproval:
			if e.Approval.Decision != types.ApprovalExpired {
				t.Fatalf("a pending approval cannot survive a restart, got %s", e.Approval.Decision)
			}
		}
	}
	if !hasUser {
		t.Fatalf("transcript lost across restart: %+v", a.Entries)
	}
	if a.SessionID != "sess-1" || a.Busy {
		t.Fatalf("expected idle console resuming sess-1, got busy=%v session=%q", a.Busy, a.SessionID)
	}

	file := filepath.Join(root, ".sdlc", "consoles", task.ID+".json")
	do(restarted, http.MethodDelete, "/api/v1/tasks/"+task.ID+"/activity", "")
	restarted.saveConsolesNow()
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("cleared transcript must be removed from disk, stat err=%v", err)
	}

	restarted.addEntry(task.ID, types.ConsoleEntry{Kind: types.ConsoleKindUser, Content: "again"})
	restarted.saveConsolesNow()
	if _, err := os.Stat(file); err != nil {
		t.Fatalf("transcript not written: %v", err)
	}
	do(restarted, http.MethodDelete, "/api/v1/tasks/"+task.ID, "")
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("deleted task's transcript must be removed, stat err=%v", err)
	}
}
