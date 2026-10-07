package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/llm"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

type erroringProvider struct{ stubProvider }

func (erroringProvider) Complete(ctx context.Context, req *llm.LLMRequest) (*llm.LLMResponse, error) {
	return nil, fmt.Errorf("provider unavailable")
}

func waitState(t *testing.T, r *Router, id string, want types.TaskState) *types.Task {
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if s := r.taskSnapshot(id); s.State == want {
			return s
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("%s never reached %s (now %s)", id, want, r.taskSnapshot(id).State)
	return nil
}

func lastEntry(r *Router, id string) types.ConsoleEntry {
	r.console.mu.Lock()
	defer r.console.mu.Unlock()
	e := r.console.get(id).entries
	return e[len(e)-1]
}

func TestSeededRunningTasksAreReconciledToHonestState(t *testing.T) {
	r := hermeticRouter(t)
	for id, task := range r.tasks {
		if task.State == types.TaskStateRunning {
			t.Fatalf("%s reported RUNNING with no live agent", id)
		}
	}
	if e := lastEntry(r, "TASK-8942"); !strings.Contains(e.Content, "No agent is running this task") {
		t.Fatalf("console should explain why the task is not running: %q", e.Content)
	}
}

func TestCreatedTaskStartsPendingUntilRun(t *testing.T) {
	r := hermeticRouter(t)
	w := do(r, http.MethodPost, "/api/v1/tasks", `{"title":"Add refund endpoint"}`)
	if w.Code != http.StatusCreated || !strings.Contains(w.Body.String(), `"state":"PENDING"`) {
		t.Fatalf("create → %d %s", w.Code, w.Body.String())
	}
}

func TestPauseInterruptsLiveTurnAndResumeContinuesSession(t *testing.T) {
	stub := &streamingStub{block: make(chan struct{}), session: "sess-live"}
	r := consoleRouter(t, stub)
	if w := do(r, http.MethodPost, "/api/v1/tasks/TASK-C1/execute", ""); w.Code != http.StatusOK {
		t.Fatalf("execute → %d %s", w.Code, w.Body.String())
	}
	if s := r.taskSnapshot("TASK-C1"); s.State != types.TaskStateRunning {
		t.Fatalf("state after execute = %s", s.State)
	}
	if w := do(r, http.MethodPost, "/api/v1/tasks/TASK-C1/execute", ""); w.Code != http.StatusConflict {
		t.Fatalf("second execute while running → %d, want 409", w.Code)
	}
	if w := do(r, http.MethodPost, "/api/v1/tasks/TASK-C1/pause", ""); w.Code != http.StatusOK {
		t.Fatalf("pause → %d", w.Code)
	}
	waitState(t, r, "TASK-C1", types.TaskStateSuspended)
	deadline := time.Now().Add(2 * time.Second)
	for !strings.Contains(lastEntry(r, "TASK-C1").Content, "Paused during task_implementation") && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if e := lastEntry(r, "TASK-C1"); !strings.Contains(e.Content, "Resume to continue") {
		t.Fatalf("pause note = %q", e.Content)
	}

	stub.mu.Lock()
	stub.block = nil
	stub.mu.Unlock()
	if w := do(r, http.MethodPost, "/api/v1/tasks/TASK-C1/resume", ""); w.Code != http.StatusOK {
		t.Fatalf("resume → %d", w.Code)
	}
	waitState(t, r, "TASK-C1", types.TaskStateWaitingGateApproval)
	stub.mu.Lock()
	last := stub.reqs[len(stub.reqs)-1]
	stub.mu.Unlock()
	prompt := last.Messages[len(last.Messages)-1].Content
	if last.SessionID != "sess-live" || !strings.Contains(prompt, "from where you left off") || strings.Contains(prompt, "Implement the change") {
		t.Fatalf("resume should continue the interrupted session, got session %q prompt %q", last.SessionID, prompt)
	}
}

// A card moved back to a stage keeps the agent's conversation: Continue resumes the same session
// and briefs it on the stage.
func TestResumeContinuesSessionOnAStageItNeverReached(t *testing.T) {
	stub := &streamingStub{}
	r := consoleRouter(t, stub)
	r.console.mu.Lock()
	c := r.console.get("TASK-C1")
	c.sessionID, c.sessionStage = "sess-old", "task_breakdown" // left over from the previous stage
	r.console.mu.Unlock()
	if w := do(r, http.MethodPost, "/api/v1/tasks/TASK-C1/resume", ""); w.Code != http.StatusOK {
		t.Fatalf("resume → %d", w.Code)
	}
	waitState(t, r, "TASK-C1", types.TaskStateWaitingGateApproval)
	stub.mu.Lock()
	last := stub.reqs[len(stub.reqs)-1]
	stub.mu.Unlock()
	prompt := last.Messages[len(last.Messages)-1].Content
	if last.SessionID != "sess-old" || !strings.Contains(prompt, "Continue in this conversation") || !strings.Contains(prompt, "Implement the change") {
		t.Fatalf("resume should keep the session and brief it on the stage: session %q prompt %q", last.SessionID, prompt)
	}
}

func TestFailedStageIsReportedAndRetryable(t *testing.T) {
	r := hermeticRouter(t)
	for _, p := range []string{"claude", "antigravity", "openai", "opencode"} {
		r.clientFactory.OverrideProvider(p, erroringProvider{})
	}
	r.mu.Lock()
	r.tasks["TASK-F"] = &types.Task{ID: "TASK-F", Title: "x", CurrentStageID: "task_breakdown", State: types.TaskStatePending, Metadata: map[string]string{}}
	r.mu.Unlock()
	r.executeTaskWithAI("TASK-F")
	s := r.taskSnapshot("TASK-F")
	if s.State != types.TaskStateFailed || !strings.Contains(s.Metadata["last_error"], "provider unavailable") {
		t.Fatalf("failed stage = %s %v", s.State, s.Metadata)
	}
	e := lastEntry(r, "TASK-F")
	if e.Kind != types.ConsoleKindError || !strings.Contains(e.Content, "Continue to pick up the agent's session") {
		t.Fatalf("failure note = %+v", e)
	}
	if proc := r.taskProcesses["TASK-F"]; proc.Status != "FAILED" {
		t.Fatalf("process status = %s", proc.Status)
	}
}

func TestRejectNeedsFeedbackAndRerunsWithIt(t *testing.T) {
	stub := &streamingStub{}
	r := consoleRouter(t, stub)
	r.executeTaskWithAI("TASK-C1")
	waitState(t, r, "TASK-C1", types.TaskStateWaitingGateApproval)

	if w := do(r, http.MethodPost, "/api/v1/tasks/TASK-C1/gate", `{"approved":false}`); w.Code != http.StatusConflict ||
		!strings.Contains(w.Body.String(), "feedback is required") {
		t.Fatalf("reject without feedback → %d %s", w.Code, w.Body.String())
	}
	if w := do(r, http.MethodPost, "/api/v1/tasks/TASK-C1/gate", `{"approved":false,"feedback":"use table-driven tests"}`); w.Code != http.StatusOK {
		t.Fatalf("reject → %d %s", w.Code, w.Body.String())
	}
	waitState(t, r, "TASK-C1", types.TaskStateWaitingGateApproval)
	stub.mu.Lock()
	last := stub.reqs[len(stub.reqs)-1]
	stub.mu.Unlock()
	prompt := last.Messages[len(last.Messages)-1].Content
	if !strings.Contains(prompt, "use table-driven tests") || !strings.Contains(prompt, "task_implementation") {
		t.Fatalf("re-run prompt lacks feedback: %q", prompt)
	}
	if s := r.taskSnapshot("TASK-C1"); s.CurrentStageID != "task_implementation" {
		t.Fatalf("reject must stay on the same stage, got %s", s.CurrentStageID)
	}
}

// Rejecting output from a session that ran this stage sends only the feedback: the session already
// holds the brief, and re-sending the task description can trip local prompt hooks (PII filters).
func TestRejectInSameSessionSendsOnlyFeedback(t *testing.T) {
	stub := &streamingStub{}
	r := consoleRouter(t, stub)
	r.mu.Lock()
	r.tasks["TASK-C1"].Description = "rotate the key of svc@example.iam"
	r.tasks["TASK-C1"].State = types.TaskStateWaitingGateApproval
	r.mu.Unlock()
	r.console.mu.Lock()
	c := r.console.get("TASK-C1")
	c.sessionID, c.sessionStage = "sess-live", "task_implementation"
	r.console.mu.Unlock()

	if w := do(r, http.MethodPost, "/api/v1/tasks/TASK-C1/gate", `{"approved":false,"feedback":"static analysis fails on pronto"}`); w.Code != http.StatusOK {
		t.Fatalf("reject → %d %s", w.Code, w.Body.String())
	}
	waitState(t, r, "TASK-C1", types.TaskStateWaitingGateApproval)
	stub.mu.Lock()
	last := stub.reqs[len(stub.reqs)-1]
	stub.mu.Unlock()
	prompt := last.Messages[len(last.Messages)-1].Content
	if last.SessionID != "sess-live" || !strings.Contains(prompt, "static analysis fails on pronto") ||
		strings.Contains(prompt, "svc@example.iam") || strings.Contains(prompt, "Implement the change") {
		t.Fatalf("reject should continue the session with the feedback only: session %q prompt %q", last.SessionID, prompt)
	}
}

func TestFinalApprovalCompletesAndStartsDependents(t *testing.T) {
	r := hermeticRouter(t)
	r.mu.Lock()
	r.tasks["TASK-P"] = &types.Task{ID: "TASK-P", Title: "parent", CurrentStageID: "signoff_merge", State: types.TaskStatePending, Metadata: map[string]string{}}
	r.tasks["TASK-D"] = &types.Task{ID: "TASK-D", Title: "child", CurrentStageID: "prd_discovery", Dependencies: []string{"TASK-P"},
		State: types.TaskStatePending, Metadata: map[string]string{}}
	r.mu.Unlock()

	if w := do(r, http.MethodPost, "/api/v1/tasks/TASK-D/execute", ""); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "blocked_dependency") {
		t.Fatalf("blocked execute → %d %s", w.Code, w.Body.String())
	}
	if s := r.taskSnapshot("TASK-D"); s.State != types.TaskStateWaitingDependency {
		t.Fatalf("child state = %s", s.State)
	}

	r.executeTaskWithAI("TASK-P")
	if w := do(r, http.MethodPost, "/api/v1/tasks/TASK-P/gate", `{"approved":true}`); w.Code != http.StatusOK {
		t.Fatalf("final approve → %d %s", w.Code, w.Body.String())
	}
	if s := r.taskSnapshot("TASK-P"); s.State != types.TaskStateCompleted {
		t.Fatalf("parent state = %s", s.State)
	}
	waitState(t, r, "TASK-D", types.TaskStateWaitingGateApproval) // started automatically and ran prd_discovery
}
