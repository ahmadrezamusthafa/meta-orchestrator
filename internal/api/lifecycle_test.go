package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/fsm"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/hooks"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/llm"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/pullrequest"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// lifecycleRouter is a router whose stages run through stub, on a pool of the given size.
func lifecycleRouter(t *testing.T, cfg RouterConfig, stub llm.ProviderClient) *Router {
	t.Helper()
	if cfg.RootDir == "" {
		cfg.RootDir = t.TempDir()
	}
	r := NewRouter(cfg)
	t.Cleanup(func() { _ = r.Close() })
	for _, p := range []string{"claude", "antigravity", "openai", "opencode"} {
		r.clientFactory.OverrideProvider(p, stub)
	}
	return r
}

func addTask(r *Router, id, stage string) {
	r.mu.Lock()
	r.tasks[id] = &types.Task{ID: id, Title: "Add CRUD endpoint " + id, CurrentStageID: stage, State: types.TaskStatePending,
		AssignedRepos: []string{"backend-core"}, Metadata: map[string]string{"complexity": "LOW"}}
	r.mu.Unlock()
}

func waitConsole(t *testing.T, r *Router, id, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(consoleText(r, id), want) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("console of %s never showed %q:\n%s", id, want, consoleText(r, id))
}

func writeHooks(t *testing.T, root string, defs string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, ".sdlc"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".sdlc", "hooks.json"), []byte(defs), 0o600); err != nil {
		t.Fatal(err)
	}
}

// Stages run on the bounded pool: with one slot and one queue place, a third stage is refused with
// 503 and its claim released, never left RUNNING with no agent.
func TestStagePoolQueuesThenRefusesWhenFull(t *testing.T) {
	stub := &streamingStub{block: make(chan struct{})}
	r := lifecycleRouter(t, RouterConfig{MaxConcurrentStages: 1, StageQueueSize: 1}, stub)
	for _, id := range []string{"TASK-P1", "TASK-P2", "TASK-P3"} {
		addTask(r, id, "task_implementation")
	}
	if w := do(r, http.MethodPost, "/api/v1/tasks/TASK-P1/execute", ""); w.Code != http.StatusOK {
		t.Fatalf("first execute → %d %s", w.Code, w.Body.String())
	}
	waitConsole(t, r, "TASK-P1", "Let me")
	if w := do(r, http.MethodPost, "/api/v1/tasks/TASK-P2/execute", ""); w.Code != http.StatusOK {
		t.Fatalf("second execute should queue → %d %s", w.Code, w.Body.String())
	}
	waitConsole(t, r, "TASK-P2", "is queued")
	w := do(r, http.MethodPost, "/api/v1/tasks/TASK-P3/execute", "")
	if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "no_capacity") {
		t.Fatalf("a full pool must answer 503 → %d %s", w.Code, w.Body.String())
	}
	if s := r.taskSnapshot("TASK-P3"); s.State != types.TaskStatePending {
		t.Fatalf("refused stage must return to its previous state, got %s", s.State)
	}
	if a := activityFor(t, r, "TASK-P3"); a.Busy {
		t.Fatal("refused stage must release the console")
	}
	waitConsole(t, r, "TASK-P3", "did not start")

	// Pausing the running stage frees the slot; the queued one then runs.
	stub.mu.Lock()
	stub.block = nil
	stub.mu.Unlock()
	if w := do(r, http.MethodPost, "/api/v1/tasks/TASK-P1/pause", ""); w.Code != http.StatusOK {
		t.Fatalf("pause → %d", w.Code)
	}
	waitState(t, r, "TASK-P2", types.TaskStateWaitingGateApproval)
	if peak := r.stages.MaxActiveSeen(); peak != 1 {
		t.Fatalf("pool ran %d stages at once, limit 1", peak)
	}
}

func activityFor(t *testing.T, r *Router, id string) activityResp {
	t.Helper()
	w := do(r, http.MethodGet, "/api/v1/tasks/"+id+"/activity", "")
	var a activityResp
	_ = json.Unmarshal(w.Body.Bytes(), &a)
	return a
}

type panickingStub struct{ stubProvider }

func (panickingStub) StreamActivity(ctx context.Context, req *llm.LLMRequest, emit func(llm.StreamEvent)) (*llm.LLMResponse, error) {
	panic("driver bug")
}

// A panic inside a stage fails that stage with a console error; the daemon keeps serving.
func TestPanickingStageFailsInsteadOfCrashing(t *testing.T) {
	r := lifecycleRouter(t, RouterConfig{}, panickingStub{})
	addTask(r, "TASK-X", "task_breakdown")
	if w := do(r, http.MethodPost, "/api/v1/tasks/TASK-X/execute", ""); w.Code != http.StatusOK {
		t.Fatalf("execute → %d", w.Code)
	}
	s := waitState(t, r, "TASK-X", types.TaskStateFailed)
	if !strings.Contains(s.Metadata["last_error"], "driver bug") {
		t.Fatalf("last_error = %q", s.Metadata["last_error"])
	}
	waitConsole(t, r, "TASK-X", "internal error")
	if a := activityFor(t, r, "TASK-X"); a.Busy {
		t.Fatal("the panicked turn must release the console")
	}
	if w := do(r, http.MethodGet, "/api/v1/tasks/TASK-X", ""); w.Code != http.StatusOK {
		t.Fatal("router must keep serving after a stage panic")
	}
}

// Close interrupts running stages so they settle as paused before the board is flushed.
func TestCloseInterruptsRunningStages(t *testing.T) {
	stub := &streamingStub{block: make(chan struct{})}
	r := lifecycleRouter(t, RouterConfig{}, stub)
	addTask(r, "TASK-S", "task_implementation")
	if w := do(r, http.MethodPost, "/api/v1/tasks/TASK-S/execute", ""); w.Code != http.StatusOK {
		t.Fatalf("execute → %d", w.Code)
	}
	waitConsole(t, r, "TASK-S", "Let me")
	_ = r.Close()
	if s := r.taskSnapshot("TASK-S"); s.State != types.TaskStateSuspended {
		t.Fatalf("after Close the stage should be SUSPENDED, got %s", s.State)
	}
	addTask(r, "TASK-T", "task_breakdown")
	if w := do(r, http.MethodPost, "/api/v1/tasks/TASK-T/execute", ""); w.Code != http.StatusServiceUnavailable {
		t.Fatalf("a closed router must refuse new stages with 503, got %d", w.Code)
	}
}

// A blocking pre-stage hook that fails stops the stage before the agent runs, with its output in the console.
func TestBlockingPreStageHookFailsTheStage(t *testing.T) {
	root := t.TempDir()
	writeHooks(t, root, `[{"id":"policy","event":"pre-stage","driver":"shell","target":"echo policy-says-no-for-$META_STAGE; exit 4","is_blocking":true}]`)
	stub := &streamingStub{}
	r := lifecycleRouter(t, RouterConfig{RootDir: root}, stub)
	addTask(r, "TASK-H", "task_breakdown")
	if w := do(r, http.MethodPost, "/api/v1/tasks/TASK-H/execute", ""); w.Code != http.StatusOK {
		t.Fatalf("execute → %d", w.Code)
	}
	s := waitState(t, r, "TASK-H", types.TaskStateFailed)
	if !strings.Contains(s.Metadata["last_error"], "policy-says-no-for-task_breakdown") {
		t.Fatalf("hook output should be the stage error, got %q", s.Metadata["last_error"])
	}
	waitConsole(t, r, "TASK-H", "policy-says-no-for-task_breakdown")
	stub.mu.Lock()
	calls := len(stub.reqs)
	stub.mu.Unlock()
	if calls != 0 {
		t.Fatalf("the agent must not run after a blocking pre-stage failure (%d calls)", calls)
	}
}

// post-stage and on-gate fire after a stage settles in review, on-gate again on the operator's
// decision, on-failure after a failure; non-blocking failures land in the console.
func TestLifecycleHooksFireAroundStages(t *testing.T) {
	root := t.TempDir()
	log := filepath.Join(t.TempDir(), "events.log")
	rec := `echo $META_EVENT $META_STAGE $META_TASK_ID >> ` + log
	writeHooks(t, root, `[
	 {"id":"post","event":"post-stage","driver":"shell","target":"`+rec+`"},
	 {"id":"gate","event":"on-gate","driver":"shell","target":"`+rec+`"},
	 {"id":"fail","event":"on-failure","driver":"shell","target":"`+rec+`"},
	 {"id":"flaky","event":"post-stage","driver":"shell","target":"echo flaky-output; exit 1"}]`)
	r := lifecycleRouter(t, RouterConfig{RootDir: root}, &streamingStub{})
	addTask(r, "TASK-L", "task_breakdown")
	if w := do(r, http.MethodPost, "/api/v1/tasks/TASK-L/execute", ""); w.Code != http.StatusOK {
		t.Fatalf("execute → %d", w.Code)
	}
	waitState(t, r, "TASK-L", types.TaskStateWaitingGateApproval)
	waitConsole(t, r, "TASK-L", "flaky-output")
	waitFile(t, log, "on-gate task_breakdown TASK-L")
	if s := r.taskSnapshot("TASK-L"); s.State != types.TaskStateWaitingGateApproval {
		t.Fatalf("a non-blocking hook failure must not change the state, got %s", s.State)
	}

	if w := do(r, http.MethodPost, "/api/v1/tasks/TASK-L/gate", `{"approved":true}`); w.Code != http.StatusOK {
		t.Fatalf("approve → %d %s", w.Code, w.Body.String())
	}
	waitFile(t, log, "on-gate task_breakdown TASK-L\non-gate task_breakdown TASK-L")
	waitState(t, r, "TASK-L", types.TaskStateWaitingGateApproval) // task_implementation ran next
	got, _ := os.ReadFile(log)
	if !strings.Contains(string(got), "post-stage task_breakdown TASK-L") {
		t.Fatalf("post-stage did not fire:\n%s", got)
	}

	for _, p := range []string{"claude", "antigravity", "openai", "opencode"} {
		r.clientFactory.OverrideProvider(p, erroringProvider{})
	}
	if w := do(r, http.MethodPost, "/api/v1/tasks/TASK-L/gate", `{"approved":false,"feedback":"redo"}`); w.Code != http.StatusOK {
		t.Fatalf("reject → %d", w.Code)
	}
	waitState(t, r, "TASK-L", types.TaskStateFailed)
	waitFile(t, log, "on-failure task_implementation TASK-L")
}

// A blocking post-stage hook is a check on the stage's output: its failure fails the stage.
func TestBlockingPostStageHookFailsReviewedStage(t *testing.T) {
	root := t.TempDir()
	writeHooks(t, root, `[{"id":"check","event":"post-stage","driver":"shell","target":"echo output-check-failed; exit 1","is_blocking":true}]`)
	r := lifecycleRouter(t, RouterConfig{RootDir: root}, &streamingStub{})
	addTask(r, "TASK-B", "task_breakdown")
	if w := do(r, http.MethodPost, "/api/v1/tasks/TASK-B/execute", ""); w.Code != http.StatusOK {
		t.Fatalf("execute → %d", w.Code)
	}
	s := waitState(t, r, "TASK-B", types.TaskStateFailed)
	if !strings.Contains(s.Metadata["last_error"], "output-check-failed") {
		t.Fatalf("last_error = %q", s.Metadata["last_error"])
	}
}

func waitFile(t *testing.T, path, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if b, _ := os.ReadFile(path); strings.Contains(string(b), want) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	b, _ := os.ReadFile(path)
	t.Fatalf("%s never contained %q:\n%s", path, want, b)
}

func TestHooksEndpointListsConfiguredHooksRedacted(t *testing.T) {
	root := t.TempDir()
	writeHooks(t, root, `[{"id":"notify","event":"on-failure","driver":"webhook","target":"https://hooks.example.test/services/SECRET-PATH"}]`)
	_ = os.MkdirAll(filepath.Join(root, ".sdlc", "hooks"), 0o755)
	_ = os.WriteFile(filepath.Join(root, ".sdlc", "hooks", "pre-commit.sh"), []byte("true\n"), 0o755)
	r := lifecycleRouter(t, RouterConfig{RootDir: root}, stubProvider{})
	w := do(r, http.MethodGet, "/api/v1/hooks", "")
	body := w.Body.String()
	if w.Code != http.StatusOK || !strings.Contains(body, `"id":"notify"`) || !strings.Contains(body, `"convention-pre-commit"`) {
		t.Fatalf("hooks → %d %s", w.Code, body)
	}
	if strings.Contains(body, "SECRET-PATH") {
		t.Fatalf("webhook URL path must be redacted: %s", body)
	}
}

// A blocking pre-commit hook stops the pull request's commit; once it passes, the commit happens and
// post-commit fires in the task worktree.
func TestCommitHooksWrapPullRequestCommit(t *testing.T) {
	r, src := worktreeRouter(t)
	bare := t.TempDir()
	runGit(t, bare, "init", "-q", "--bare")
	runGit(t, src, "remote", "add", "origin", "https://bitbucket.org/ws/app.git")
	runGit(t, src, "config", "remote.origin.pushurl", bare)
	runGit(t, src, "config", "user.name", "t")
	runGit(t, src, "config", "user.email", "t@t")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"values":[]}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":7,"links":{"html":{"href":"https://bitbucket.org/ws/app/pull-requests/7"}}}`))
	}))
	defer srv.Close()
	r.prClient = &pullrequest.Client{BaseURL: srv.URL}
	if _, err := r.cfg.ConnectorsManager.UpdateConnector(types.ConnectorItem{ID: "bitbucket", Enabled: true, Username: "u", APIToken: "tok"}); err != nil {
		t.Fatal(err)
	}
	allow := filepath.Join(t.TempDir(), "allow")
	marker := filepath.Join(t.TempDir(), "post")
	r.cfg.HookEngine.RegisterHook(hookDef("lint", "pre-commit", `[ -f `+allow+` ] || { echo lint-says-no; exit 1; }`, true))
	r.cfg.HookEngine.RegisterHook(hookDef("after", "post-commit", `git log -1 --format=%s > `+marker, false))

	r.mu.Lock()
	r.tasks["TASK-1"] = &types.Task{ID: "TASK-1", Title: "Refund retries", CurrentStageID: "task_implementation",
		State: types.TaskStateWaitingGateApproval, AssignedRepos: []string{"app"}, Metadata: map[string]string{}}
	r.mu.Unlock()
	dir := r.resolveWorkDir(r.taskSnapshot("TASK-1"))
	_ = os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc retry() {}\n"), 0o644)

	w := do(r, http.MethodPost, "/api/v1/tasks/TASK-1/pull-request", `{"repo":"app"}`)
	if w.Code == http.StatusOK || !strings.Contains(w.Body.String(), "lint-says-no") {
		t.Fatalf("failing pre-commit must stop the PR → %d %s", w.Code, w.Body.String())
	}
	if out := runGitOut(t, dir, "status", "--porcelain"); !strings.Contains(out, "main.go") {
		t.Fatalf("nothing may be committed after a failing pre-commit hook, status %q", out)
	}

	_ = os.WriteFile(allow, nil, 0o600)
	if w := do(r, http.MethodPost, "/api/v1/tasks/TASK-1/pull-request", `{"repo":"app"}`); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"committed":true`) {
		t.Fatalf("open → %d %s", w.Code, w.Body.String())
	}
	waitFile(t, marker, "Refund retries")
}

func hookDef(id, event, target string, blocking bool) hooks.HookDefinition {
	return hooks.HookDefinition{ID: id, Event: hooks.HookEvent(event), Driver: hooks.HookDriverShell, Target: target, IsBlocking: blocking}
}

func runGitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
	return string(out)
}

// Custom workflows decide the stages a task runs, survive a restart, and only use runnable stages.
func TestCustomWorkflowDrivesTheTaskAndPersists(t *testing.T) {
	root := t.TempDir()
	r := lifecycleRouter(t, RouterConfig{RootDir: root}, &streamingStub{})
	if w := do(r, http.MethodPost, "/api/v1/workflows", `{"id":"lean","name":"Lean","stages":[{"id":"task_breakdown"},{"id":"made_up_stage"}]}`); w.Code != http.StatusBadRequest {
		t.Fatalf("unknown stage must be rejected with 400, got %d %s", w.Code, w.Body.String())
	}
	if w := do(r, http.MethodPost, "/api/v1/workflows", `{"id":"general_ai_sdlc","name":"x","stages":[{"id":"task_breakdown"}]}`); w.Code != http.StatusConflict {
		t.Fatalf("built-in workflows are read-only, got %d", w.Code)
	}
	if w := do(r, http.MethodPost, "/api/v1/workflows", `{"id":"lean","name":"Lean","stages":[{"id":"task_breakdown"},{"id":"e2e_validation"}]}`); w.Code != http.StatusCreated {
		t.Fatalf("create workflow → %d %s", w.Code, w.Body.String())
	}
	if w := do(r, http.MethodPost, "/api/v1/tasks", `{"title":"x","workflow_id":"nope"}`); w.Code != http.StatusBadRequest {
		t.Fatalf("unknown workflow on create must be 400, got %d", w.Code)
	}

	// A restarted router knows the workflow.
	r2 := lifecycleRouter(t, RouterConfig{RootDir: root}, &streamingStub{})
	if _, err := r2.cfg.WorkflowReg.Get("lean"); err != nil {
		t.Fatalf("custom workflow did not survive restart: %v", err)
	}

	w := do(r2, http.MethodPost, "/api/v1/tasks", `{"title":"Lean change","workflow_id":"lean","assigned_repos":["backend-core"]}`)
	created := decodeTask(t, w.Body.Bytes())
	if created.CurrentStageID != "task_breakdown" || created.CurrentStageIndex != 0 || created.WorkflowID != "lean" {
		t.Fatalf("task should start at the workflow's first stage, got %s/%d (%s)", created.CurrentStageID, created.CurrentStageIndex, created.WorkflowID)
	}
	if len(created.RoutingPlan) != 2 {
		t.Fatalf("routing plan should cover the workflow's 2 stages, got %d", len(created.RoutingPlan))
	}
	if w := do(r2, http.MethodPatch, "/api/v1/tasks/"+created.ID, `{"current_stage_id":"uat_verification"}`); w.Code != http.StatusBadRequest {
		t.Fatalf("moving to a stage outside the workflow must be 400, got %d", w.Code)
	}
	if w := do(r2, http.MethodPost, "/api/v1/tasks/"+created.ID+"/execute", ""); w.Code != http.StatusOK {
		t.Fatalf("execute → %d %s", w.Code, w.Body.String())
	}
	waitState(t, r2, created.ID, types.TaskStateWaitingGateApproval)
	waitConsole(t, r2, created.ID, "approve to continue to e2e_validation")
	if w := do(r2, http.MethodPost, "/api/v1/tasks/"+created.ID+"/gate", `{"approved":true}`); w.Code != http.StatusOK {
		t.Fatalf("approve → %d", w.Code)
	}
	if s := r2.taskSnapshot(created.ID); s.CurrentStageID != "e2e_validation" || s.CurrentStageIndex != 1 {
		t.Fatalf("approval should follow the workflow to e2e_validation, got %s/%d", s.CurrentStageID, s.CurrentStageIndex)
	}
	waitState(t, r2, created.ID, types.TaskStateWaitingGateApproval)
	waitConsole(t, r2, created.ID, "This is the final stage")
	if w := do(r2, http.MethodPost, "/api/v1/tasks/"+created.ID+"/gate", `{"approved":true}`); w.Code != http.StatusOK {
		t.Fatalf("approve last → %d", w.Code)
	}
	if s := r2.taskSnapshot(created.ID); s.State != types.TaskStateCompleted {
		t.Fatalf("approving the workflow's last stage completes the task, got %s", s.State)
	}

	if w := do(r2, http.MethodPut, "/api/v1/workflows/lean", `{"name":"Lean v2","stages":[{"id":"task_implementation"}]}`); w.Code != http.StatusOK {
		t.Fatalf("update custom workflow → %d %s", w.Code, w.Body.String())
	}
	if w := do(r2, http.MethodGet, "/api/v1/workflows/stages", ""); !strings.Contains(w.Body.String(), "signoff_merge") {
		t.Fatalf("stages endpoint → %s", w.Body.String())
	}
}

func TestWorkflowYAMLIsLoadedAndLegacyIDsRunTheDefaultPipeline(t *testing.T) {
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, ".sdlc"), 0o755)
	yaml := "workflow:\n  id: from_yaml\n  name: From YAML\n  stages:\n    - id: prd_discovery\n    - id: signoff_merge\n"
	if err := os.WriteFile(filepath.Join(root, ".sdlc", "workflow.yaml"), []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	r := lifecycleRouter(t, RouterConfig{RootDir: root}, stubProvider{})
	if got := r.workflowStages("from_yaml"); strings.Join(got, ",") != "prd_discovery,signoff_merge" {
		t.Fatalf("workflow.yaml stages = %v", got)
	}
	legacy := &types.Task{WorkflowID: "general-ai-sdlc"}
	if r.nextStageFor(legacy, "techdoc_rfc") != "task_breakdown" || r.nextStageFor(&types.Task{WorkflowID: "gone"}, "techdoc_rfc") != "task_breakdown" {
		t.Fatal("legacy and unknown workflow IDs keep running the default pipeline")
	}
	if strings.Join(stagePipeline, ",") != strings.Join(fsm.StageIDs(fsm.GetGeneralAISDLCWorkflow()), ",") {
		t.Fatal("stagePipeline must be the default workflow's stages")
	}
}

// The lifecycle transition table guards operator-set states.
func TestPatchRefusesIllegalStateTransition(t *testing.T) {
	r := lifecycleRouter(t, RouterConfig{}, stubProvider{})
	addTask(r, "TASK-Q", "task_breakdown")
	if w := do(r, http.MethodPatch, "/api/v1/tasks/TASK-Q", `{"state":"WAITING_GATE_APPROVAL"}`); w.Code != http.StatusConflict {
		t.Fatalf("PENDING → WAITING_GATE_APPROVAL has no output to review: want 409, got %d", w.Code)
	}
	if s := r.taskSnapshot("TASK-Q"); s.State != types.TaskStatePending {
		t.Fatalf("state changed to %s", s.State)
	}
}

// With a task-state mirror configured, snapshots are mirrored on persist and active tasks the board
// file lacks are restored at startup; deleted tasks leave the mirror.
func TestTaskMirrorRestoresTasksTheBoardLacks(t *testing.T) {
	root := t.TempDir()
	store := fsm.NewMemoryTaskStateStore()
	cfg := RouterConfig{RootDir: root, TaskStorePath: filepath.Join(root, ".sdlc", "tasks.json"), TaskStore: store}
	r := lifecycleRouter(t, cfg, stubProvider{})
	keep := decodeTask(t, do(r, http.MethodPost, "/api/v1/tasks", `{"title":"Keep me"}`).Body.Bytes())
	gone := decodeTask(t, do(r, http.MethodPost, "/api/v1/tasks", `{"title":"Delete me"}`).Body.Bytes())
	if _, err := store.GetTask(context.Background(), keep.ID); err != nil {
		t.Fatalf("created task was not mirrored: %v", err)
	}
	if w := do(r, http.MethodDelete, "/api/v1/tasks/"+gone.ID, ""); w.Code != http.StatusOK {
		t.Fatalf("delete → %d", w.Code)
	}
	if _, err := store.GetTask(context.Background(), gone.ID); err == nil {
		t.Fatal("deleted task must leave the mirror")
	}
	_ = r.Close()

	if err := os.Remove(cfg.TaskStorePath); err != nil { // the board file is lost
		t.Fatal(err)
	}
	r2 := lifecycleRouter(t, cfg, stubProvider{})
	s := r2.taskSnapshot(keep.ID)
	if s == nil || s.Title != "Keep me" {
		t.Fatalf("active task was not restored from the mirror: %+v", s)
	}
	if r2.taskSnapshot(gone.ID) != nil {
		t.Fatal("a deleted task must not come back")
	}
	waitConsole(t, r2, keep.ID, "Restored from the task state mirror")
	if next := decodeTask(t, do(r2, http.MethodPost, "/api/v1/tasks", `{"title":"Next"}`).Body.Bytes()); next.ID == keep.ID {
		t.Fatal("restored task IDs must not be reused")
	}
}
