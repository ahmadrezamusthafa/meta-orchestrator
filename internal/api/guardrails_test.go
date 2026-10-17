package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/llm"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/ws"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// flakyProvider fails every call with the same error, except for the request id, duration and
// attempt number that differ between calls — the breaker must still see one repeated failure.
type flakyProvider struct {
	stubProvider
	n   *int32
	msg string // overrides the error when set
}

func (p flakyProvider) Complete(ctx context.Context, req *llm.LLMRequest) (*llm.LLMResponse, error) {
	n := atomic.AddInt32(p.n, 1)
	if p.msg != "" {
		return nil, fmt.Errorf("%s", p.msg)
	}
	return nil, fmt.Errorf("upstream 503 for request %08x-1111-2222-3333-444455556666 after %dms (attempt %d)", n, 100+n*37, n)
}

func frustrationRouter(t *testing.T, root string, hub *ws.Hub) (*Router, *int32) {
	calls := new(int32)
	r := NewRouter(RouterConfig{RootDir: root, WSHub: hub})
	for _, p := range []string{"claude", "antigravity", "openai", "opencode"} {
		r.clientFactory.OverrideProvider(p, flakyProvider{n: calls})
	}
	r.mu.Lock()
	r.tasks["TASK-L"] = &types.Task{ID: "TASK-L", Title: "Loop", CurrentStageID: "task_breakdown", State: types.TaskStatePending, Metadata: map[string]string{}}
	r.mu.Unlock()
	return r, calls
}

func TestRepeatedStageFailureHaltsTaskAndOperatorCanContinue(t *testing.T) {
	hub := ws.NewHub()
	go hub.Run()
	t.Cleanup(hub.Stop)
	srv := httptest.NewServer(http.HandlerFunc(hub.HandleWebSocket))
	t.Cleanup(srv.Close)
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"?task_id=TASK-L", nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	halts := make(chan types.FrustrationHaltPayload, 4)
	go func() {
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			for _, line := range strings.Split(string(data), "\n") { // the hub batches queued events
				var ev struct {
					Type    types.EventType              `json:"type"`
					Payload types.FrustrationHaltPayload `json:"payload"`
				}
				if json.Unmarshal([]byte(line), &ev) == nil && ev.Type == types.EventFrustrationHalt {
					halts <- ev.Payload
				}
			}
		}
	}()
	time.Sleep(50 * time.Millisecond) // let the hub register the client

	r, _ := frustrationRouter(t, t.TempDir(), hub)
	for i := 1; i <= 2; i++ {
		r.executeTaskWithAI("TASK-L")
		if s := r.taskSnapshot("TASK-L"); s.State != types.TaskStateFailed || s.Metadata["frustration_count"] != fmt.Sprint(i) {
			t.Fatalf("after failure %d: %s %v", i, s.State, s.Metadata)
		}
	}
	r.executeTaskWithAI("TASK-L")
	s := r.taskSnapshot("TASK-L")
	if s.State != types.TaskStateBlockedFrustration || s.Metadata["failing_trace"] == "" || s.Metadata["frustration_count"] != "3" {
		t.Fatalf("third identical failure should block the task: %s %v", s.State, s.Metadata)
	}
	if e := lastEntry(r, "TASK-L"); e.Kind != types.ConsoleKindError || !strings.Contains(e.Content, "Halted: task_breakdown failed 3 times") {
		t.Fatalf("halt note = %+v", e)
	}
	if proc := r.taskProcesses["TASK-L"]; proc.Status != "BLOCKED" {
		t.Fatalf("process status = %s", proc.Status)
	}
	select {
	case p := <-halts:
		if p.TaskID != "TASK-L" || p.StageID != "task_breakdown" || p.ConsecutiveFails != 3 {
			t.Fatalf("halt event = %+v", p)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no event.frustration_halt broadcast")
	}

	// The operator continues the blocked task: it runs again, and the count starts over.
	if w := do(r, http.MethodPost, "/api/v1/tasks/TASK-L/resume", ""); w.Code != http.StatusOK {
		t.Fatalf("resume of a blocked task → %d %s", w.Code, w.Body.String())
	}
	s = waitState(t, r, "TASK-L", types.TaskStateFailed)
	if s.Metadata["frustration_count"] != "1" {
		t.Fatalf("count after continuing past the halt = %v", s.Metadata)
	}
	// Reset is an operator decision: the count is cleared before the next run.
	r.executeTaskWithAI("TASK-L")
	if w := do(r, http.MethodPost, "/api/v1/tasks/TASK-L/reset", ""); w.Code != http.StatusOK {
		t.Fatalf("reset → %d", w.Code)
	}
	if s = waitState(t, r, "TASK-L", types.TaskStateFailed); s.Metadata["frustration_count"] != "1" {
		t.Fatalf("count after reset = %v", s.Metadata)
	}
}

func TestFrustrationThresholdComesFromTheTaskMethod(t *testing.T) {
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, "configs"), 0o755)
	var methods []map[string]interface{}
	for _, id := range []string{"react", "bmad", "supervisor", "superpower"} {
		methods = append(methods, map[string]interface{}{"id": id, "max_consecutive_iterations": 2})
	}
	data, _ := json.Marshal(methods)
	_ = os.WriteFile(filepath.Join(root, "configs", "methods.json"), data, 0o644)
	r, _ := frustrationRouter(t, root, nil)
	r.executeTaskWithAI("TASK-L")
	r.executeTaskWithAI("TASK-L")
	if s := r.taskSnapshot("TASK-L"); s.State != types.TaskStateBlockedFrustration {
		t.Fatalf("method limit 2 should block on the second failure: %s (method %q)", s.State, s.SelectedMethod)
	}
	if r.frustrationThreshold("unknown") != defaultFrustrationThreshold {
		t.Fatal("unknown method should use the default")
	}
}

func TestDifferentFailuresAndTimeoutsDoNotHalt(t *testing.T) {
	r, calls := frustrationRouter(t, t.TempDir(), nil)
	for i, msg := range []string{"permission channel closed", "provider unavailable", "permission channel closed"} {
		for _, p := range []string{"claude", "antigravity", "openai", "opencode"} {
			r.clientFactory.OverrideProvider(p, flakyProvider{n: calls, msg: msg})
		}
		r.executeTaskWithAI("TASK-L")
		if s := r.taskSnapshot("TASK-L"); s.State != types.TaskStateFailed || s.Metadata["frustration_count"] != "1" {
			t.Fatalf("run %d: %s %v", i, s.State, s.Metadata)
		}
	}
	for i := 0; i < 4; i++ {
		r.mu.Lock()
		task := r.tasks["TASK-L"]
		halt := r.recordStageOutcome(task, "task_breakdown", fmt.Errorf("%w (3h)", llm.ErrTurnTimeLimit))
		r.mu.Unlock()
		if halt != nil {
			t.Fatal("a timeout is not a repeated failure")
		}
	}
}

// guardStub plays one scripted agent turn per call.
type guardStub struct {
	stubProvider
	mu   sync.Mutex
	reqs []llm.LLMRequest
	turn func(ctx context.Context, req *llm.LLMRequest, emit func(llm.StreamEvent)) string
}

func (s *guardStub) StreamActivity(ctx context.Context, req *llm.LLMRequest, emit func(llm.StreamEvent)) (*llm.LLMResponse, error) {
	s.mu.Lock()
	s.reqs = append(s.reqs, *req)
	turn := s.turn
	s.mu.Unlock()
	content := turn(ctx, req, emit)
	return &llm.LLMResponse{Content: content, Provider: "anthropic", Model: "claude-x", FinishReason: "stop", SessionID: "sess-atdd"}, nil
}

func atddRouter(t *testing.T, stub *guardStub) (*Router, string) {
	r, _ := worktreeRouter(t)
	for _, p := range []string{"claude", "antigravity", "openai", "opencode"} {
		r.clientFactory.OverrideProvider(p, stub)
	}
	r.mu.Lock()
	r.tasks["TASK-A"] = &types.Task{ID: "TASK-A", Title: "Refund 'button'", CurrentStageID: "atdd_creation", State: types.TaskStatePending,
		AssignedRepos: []string{"app"}, Metadata: map[string]string{"complexity": "LOW", "approve_all": "true"}}
	r.mu.Unlock()
	return r, r.resolveWorkDir(r.taskSnapshot("TASK-A"))
}

const atddOutput = "Cases:\n```atdd-cases\n" + `[{"id":"RF-TC-001","title":"Refund from the \"Invoices\" screen","platform":"WEB","steps":["1. Open Invoices"],"expected_results":["1. Refunded"]},
 {"id":"RF-TC-002","title":"Refund API","platform":"API","steps":["POST /refunds"]}]` + "\n```\nSummary: done"

func ask(ctx context.Context, req *llm.LLMRequest, id, tool string, input map[string]interface{}) llm.ApprovalDecision {
	return req.Approver(ctx, llm.ApprovalRequest{ToolName: tool, ToolUseID: id, Input: input})
}

func TestATDDStageWriteLocksSourceAndVerifiesRedPhase(t *testing.T) {
	decisions := map[string]llm.ApprovalDecision{}
	stub := &guardStub{}
	stub.turn = func(ctx context.Context, req *llm.LLMRequest, emit func(llm.StreamEvent)) string {
		spec := filepath.Join(req.WorkDir, "tests", "e2e", "refund.spec.ts")
		decisions["spec"] = ask(ctx, req, "tu_w", "Write", map[string]interface{}{"file_path": spec})
		decisions["src"] = ask(ctx, req, "tu_e", "Edit", map[string]interface{}{"file_path": filepath.Join(req.WorkDir, "main.go")})
		decisions["escape"] = ask(ctx, req, "tu_x", "Write", map[string]interface{}{"file_path": filepath.Join(req.WorkDir, "..", "other_test.go")})
		decisions["shell"] = ask(ctx, req, "tu_s", "Bash", map[string]interface{}{"command": "echo 'func x() {}' >> main.go"})
		run := map[string]interface{}{"command": "npx playwright test tests/e2e"}
		decisions["run"] = ask(ctx, req, "tu_t", "Bash", run)
		emit(llm.StreamEvent{Type: llm.StreamToolUse, ToolID: "tu_t", ToolName: "Bash", ToolInput: run})
		emit(llm.StreamEvent{Type: llm.StreamToolResult, ToolID: "tu_t", IsError: true,
			Text: "Exit code 1\n  1) refund.spec.ts:3 › refund\n    Error: expect(locator('#refund-toast')).toBeVisible() failed"})
		return atddOutput
	}
	r, _ := atddRouter(t, stub)
	r.executeTaskWithAI("TASK-A")

	s := r.taskSnapshot("TASK-A")
	if s.State != types.TaskStateWaitingGateApproval {
		t.Fatalf("state = %s (%v)\n%s", s.State, s.Metadata, consoleText(r, "TASK-A"))
	}
	if mode := stub.reqs[0].PermissionMode; mode != modeDefault {
		t.Fatalf("ATDD must run where edits reach the approver, got %q", mode)
	}
	if !strings.Contains(stub.reqs[0].Messages[0].Content, "Write lock (ATDD red phase)") {
		t.Fatal("agent was not briefed on the write lock")
	}
	if !decisions["spec"].Allow || decisions["src"].Allow || decisions["escape"].Allow || decisions["shell"].Allow || !decisions["run"].Allow {
		t.Fatalf("decisions = %+v", decisions)
	}
	if !strings.Contains(decisions["src"].Message, "write-locked") {
		t.Fatalf("denial should explain the lock: %q", decisions["src"].Message)
	}
	if s.Metadata["red_phase"] != "verified" || s.Metadata["red_phase_command"] != "npx playwright test tests/e2e" {
		t.Fatalf("red phase = %v", s.Metadata)
	}
	if r.writeLocks.IsLocked("TASK-A") {
		t.Fatal("a verified red phase unlocks production source")
	}
	log := consoleText(r, "TASK-A")
	for _, want := range []string{"ATDD red phase: production source is write-locked", "Refused Edit", "Red phase verified", "Playwright scaffold for 1 web case(s)"} {
		if !strings.Contains(log, want) {
			t.Errorf("console missing %q:\n%s", want, log)
		}
	}
	spec, err := os.ReadFile(filepath.Join(r.artifactDir("TASK-A"), "atdd", "acceptance.spec.ts"))
	if err != nil || !strings.Contains(string(spec), `test("RF-TC-001: Refund from the \"Invoices\" screen"`) ||
		strings.Contains(string(spec), "RF-TC-002") || !strings.Contains(string(spec), "notImplemented(\"Open Invoices\")") {
		t.Fatalf("scaffold (%v):\n%s", err, spec)
	}
}

func TestATDDWithoutTestRunIsNotVerifiedAndLockHoldsUntilStageChanges(t *testing.T) {
	var chatDecision llm.ApprovalDecision
	stub := &guardStub{}
	stub.turn = func(ctx context.Context, req *llm.LLMRequest, emit func(llm.StreamEvent)) string {
		// A test run the guard refused never ran: its error result is not a red phase.
		run := map[string]interface{}{"command": "rm main.go && npx playwright test"}
		ask(ctx, req, "tu_d", "Bash", run)
		emit(llm.StreamEvent{Type: llm.StreamToolUse, ToolID: "tu_d", ToolName: "Bash", ToolInput: run})
		emit(llm.StreamEvent{Type: llm.StreamToolResult, ToolID: "tu_d", IsError: true, Text: "operation rejected"})
		return "Summary: wrote the tests"
	}
	r, _ := atddRouter(t, stub)
	r.executeTaskWithAI("TASK-A")
	s := r.taskSnapshot("TASK-A")
	if s.Metadata["red_phase"] != "not_verified" || !strings.Contains(s.Metadata["red_phase_summary"], "did not run the acceptance tests") {
		t.Fatalf("red phase = %v", s.Metadata)
	}
	if !r.writeLocks.IsLocked("TASK-A") || !strings.Contains(consoleText(r, "TASK-A"), "Red phase not verified") {
		t.Fatal("lock should hold and the console explain why")
	}

	// A side conversation during review is still under the lock.
	stub.mu.Lock()
	stub.turn = func(ctx context.Context, req *llm.LLMRequest, emit func(llm.StreamEvent)) string {
		chatDecision = ask(ctx, req, "tu_c", "Edit", map[string]interface{}{"file_path": filepath.Join(req.WorkDir, "main.go")})
		return "ok"
	}
	stub.mu.Unlock()
	if w := do(r, http.MethodPost, "/api/v1/tasks/TASK-A/chat", `{"message":"just fix main.go"}`); w.Code != http.StatusOK && w.Code != http.StatusAccepted {
		t.Fatalf("chat → %d %s", w.Code, w.Body.String())
	}
	waitIdleTask(t, r, "TASK-A")
	stub.mu.Lock()
	last := stub.reqs[len(stub.reqs)-1]
	stub.mu.Unlock()
	if chatDecision.Allow || last.PermissionMode != modeDefault {
		t.Fatalf("chat on a locked ATDD stage: decision %+v mode %q", chatDecision, last.PermissionMode)
	}

	// Approving moves to the next stage, which lifts the lock.
	stub.mu.Lock()
	stub.turn = func(ctx context.Context, req *llm.LLMRequest, emit func(llm.StreamEvent)) string { return "design" }
	stub.mu.Unlock()
	if w := do(r, http.MethodPost, "/api/v1/tasks/TASK-A/gate", `{"approved":true}`); w.Code != http.StatusOK {
		t.Fatalf("approve → %d %s", w.Code, w.Body.String())
	}
	waitState(t, r, "TASK-A", types.TaskStateWaitingGateApproval)
	if r.writeLocks.IsLocked("TASK-A") {
		t.Fatal("leaving atdd_creation must lift the write lock")
	}
}
