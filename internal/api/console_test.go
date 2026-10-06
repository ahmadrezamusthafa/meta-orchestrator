package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/llm"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// streamingStub is an ActivityStreamer that replays a Claude-Code-like turn, optionally blocking
// until cancelled, and records the requests it received.
type streamingStub struct {
	stubProvider
	mu      sync.Mutex
	block   chan struct{}
	reqs    []llm.LLMRequest
	session string // reported as a live session before the turn answers, when set
}

func (s *streamingStub) StreamActivity(ctx context.Context, req *llm.LLMRequest, emit func(llm.StreamEvent)) (*llm.LLMResponse, error) {
	s.mu.Lock()
	s.reqs = append(s.reqs, *req)
	block, session := s.block, s.session
	s.mu.Unlock()
	if session != "" {
		emit(llm.StreamEvent{Type: llm.StreamSession, SessionID: session})
	}
	emit(llm.StreamEvent{Type: llm.StreamThinkingDelta, Text: "looking at the handler"})
	emit(llm.StreamEvent{Type: llm.StreamTextDelta, Text: "Let me "})
	if block != nil {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	emit(llm.StreamEvent{Type: llm.StreamTextDelta, Text: "check."})
	emit(llm.StreamEvent{Type: llm.StreamToolUse, ToolID: "tu_1", ToolName: "Read", ToolInput: map[string]interface{}{"file_path": "main.go"}})
	emit(llm.StreamEvent{Type: llm.StreamToolResult, ToolID: "tu_1", Text: "package main"})
	emit(llm.StreamEvent{Type: llm.StreamTextDelta, Text: "It compiles."})
	return &llm.LLMResponse{Content: "Let me check.It compiles.", Provider: "anthropic", Model: "claude-x", FinishReason: "stop",
		SessionID: "sess-123", TokenUsage: types.TokenUsage{PromptTokens: 1000, CompletionTokens: 50, CachedTokens: 800, TotalTokens: 1050}}, nil
}

// inReview puts a task in gate review, where a typed message is a side conversation that leaves the
// stage alone (while the stage is idle a message continues it instead).
func inReview(r *Router, taskID string) {
	r.mu.Lock()
	r.tasks[taskID].State = types.TaskStateWaitingGateApproval
	r.mu.Unlock()
}

func consoleRouter(t *testing.T, stub *streamingStub) *Router {
	r := NewRouter(RouterConfig{RootDir: t.TempDir()})
	for _, p := range []string{"claude", "antigravity", "openai", "opencode"} {
		r.clientFactory.OverrideProvider(p, stub)
	}
	r.mu.Lock()
	r.tasks["TASK-C1"] = &types.Task{ID: "TASK-C1", Title: "Add CRUD endpoint", CurrentStageID: "task_implementation",
		State: types.TaskStatePending, AssignedRepos: []string{"backend-core"}, Metadata: map[string]string{"complexity": "LOW"}}
	r.mu.Unlock()
	return r
}

func do(r *Router, method, path, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(method, path, bytes.NewBufferString(body)))
	return w
}

type activityResp struct {
	Entries   []types.ConsoleEntry `json:"entries"`
	Busy      bool                 `json:"busy"`
	SessionID string               `json:"session_id"`
}

func activity(t *testing.T, r *Router, query string) activityResp {
	w := do(r, http.MethodGet, "/api/v1/tasks/TASK-C1/activity"+query, "")
	if w.Code != http.StatusOK {
		t.Fatalf("activity → %d %s", w.Code, w.Body.String())
	}
	var a activityResp
	_ = json.Unmarshal(w.Body.Bytes(), &a)
	return a
}

func waitIdle(t *testing.T, r *Router) activityResp {
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if a := activity(t, r, ""); !a.Busy {
			return a
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("turn never finished")
	return activityResp{}
}

func kinds(entries []types.ConsoleEntry) string {
	var k []string
	for _, e := range entries {
		k = append(k, e.Kind)
	}
	return strings.Join(k, ",")
}

func TestChatTurnStreamsRealActivityTranscript(t *testing.T) {
	stub := &streamingStub{}
	r := consoleRouter(t, stub)
	inReview(r, "TASK-C1")

	w := do(r, http.MethodPost, "/api/v1/tasks/TASK-C1/chat", `{"message":"does it compile?"}`)
	if w.Code != http.StatusAccepted {
		t.Fatalf("chat → %d %s", w.Code, w.Body.String())
	}
	a := waitIdle(t, r)

	want := "user,request,thinking,assistant,tool_use,tool_result,assistant,response"
	if got := kinds(a.Entries); got != want {
		t.Fatalf("entries = %s\nwant      %s", got, want)
	}
	e := a.Entries
	if e[0].Content != "does it compile?" || e[0].TurnID == "" {
		t.Fatalf("user entry = %+v", e[0])
	}
	for _, x := range e[1:] {
		if x.TurnID != e[0].TurnID {
			t.Fatalf("entry %s not grouped in turn: %+v", x.Kind, x)
		}
	}
	req := e[1].Request
	if req == nil || req.Source != "chat" || len(req.Messages) != 2 || req.Messages[0].Role != "system" || req.Messages[1].Content != "does it compile?" {
		t.Fatalf("request entry = %+v", req)
	}
	if e[2].Content != "looking at the handler" || e[3].Content != "Let me check." || e[3].Status != types.ConsoleDone {
		t.Fatalf("streamed entries = %+v / %+v", e[2], e[3])
	}
	if e[4].Tool.Name != "Read" || e[5].Tool.ID != "tu_1" || e[5].Content != "package main" || e[6].Content != "It compiles." {
		t.Fatalf("tool/assistant entries = %+v %+v %+v", e[4], e[5], e[6])
	}
	u := e[7].Usage
	if u == nil || u.PromptTokens != 1000 || u.CachedTokens != 800 || u.CostUSD <= 0 || u.SessionID != "sess-123" {
		t.Fatalf("usage = %+v", u)
	}
	if a.SessionID != "sess-123" {
		t.Fatalf("session not retained: %q", a.SessionID)
	}

	// Incremental fetch.
	if inc := activity(t, r, "?after="+e[5].ID); kinds(inc.Entries) != "assistant,response" {
		t.Fatalf("after= fetch = %s", kinds(inc.Entries))
	}

	// Second turn resumes the provider session and replays history for stateless providers.
	do(r, http.MethodPost, "/api/v1/tasks/TASK-C1/chat", `{"message":"and the tests?"}`)
	waitIdle(t, r)
	stub.mu.Lock()
	second := stub.reqs[len(stub.reqs)-1]
	stub.mu.Unlock()
	if second.SessionID != "sess-123" {
		t.Fatalf("second turn session = %q", second.SessionID)
	}
	var roles []string
	for _, m := range second.Messages {
		roles = append(roles, string(m.Role))
	}
	if strings.Join(roles, ",") != "system,user,assistant,user" || second.Messages[2].Content != "Let me check.\n\nIt compiles." {
		t.Fatalf("history = %v %+v", roles, second.Messages)
	}
	if second.WorkDir == "" {
		t.Fatal("agentic providers need a real working directory")
	}
}

func TestChatBusyCancelAndClear(t *testing.T) {
	stub := &streamingStub{block: make(chan struct{})}
	r := consoleRouter(t, stub)
	inReview(r, "TASK-C1")

	if w := do(r, http.MethodPost, "/api/v1/tasks/TASK-C1/chat", `{"message":"long task"}`); w.Code != http.StatusAccepted {
		t.Fatalf("chat → %d", w.Code)
	}
	deadline := time.Now().Add(time.Second)
	for !activity(t, r, "").Busy && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	if w := do(r, http.MethodPost, "/api/v1/tasks/TASK-C1/chat", `{"message":"again"}`); w.Code != http.StatusConflict {
		t.Fatalf("concurrent chat → %d, want 409", w.Code)
	}
	if w := do(r, http.MethodPost, "/api/v1/tasks/TASK-C1/chat/cancel", ""); !strings.Contains(w.Body.String(), "cancelled") {
		t.Fatalf("cancel → %s", w.Body.String())
	}
	a := waitIdle(t, r)
	last := a.Entries[len(a.Entries)-1]
	if last.Kind != types.ConsoleKindSystem || last.Content != "Interrupted by operator" {
		t.Fatalf("last entry = %+v (all: %s)", last, kinds(a.Entries))
	}
	for _, e := range a.Entries {
		if e.Kind == types.ConsoleKindAssistant && e.Status != types.ConsoleCancelled {
			t.Fatalf("streaming entry not marked cancelled: %+v", e)
		}
	}
	if w := do(r, http.MethodPost, "/api/v1/tasks/TASK-C1/chat/cancel", ""); !strings.Contains(w.Body.String(), "idle") {
		t.Fatalf("idle cancel → %s", w.Body.String())
	}

	if w := do(r, http.MethodPost, "/api/v1/tasks/TASK-C1/chat", `{"message":"  "}`); w.Code != http.StatusBadRequest {
		t.Fatalf("empty message → %d", w.Code)
	}
	if w := do(r, http.MethodDelete, "/api/v1/tasks/TASK-C1/activity", ""); w.Code != http.StatusOK {
		t.Fatalf("clear → %d", w.Code)
	}
	if a := activity(t, r, ""); len(a.Entries) != 0 || a.SessionID != "" {
		t.Fatalf("after clear = %+v", a)
	}
}

func TestExecuteStreamsIntoConsoleWithoutFabricatedLogs(t *testing.T) {
	stub := &streamingStub{}
	r := consoleRouter(t, stub)
	r.executeTaskWithAI("TASK-C1")

	a := activity(t, r, "")
	got := kinds(a.Entries)
	// state, "▶ Running", the read-only notice (no worktree for this repo), then the model turn.
	if !strings.HasPrefix(got, "state,system,system,request,") || !strings.HasSuffix(got, ",response,state,system") {
		t.Fatalf("execute entries = %s", got)
	}
	if st := a.Entries[0].State; st.From != "PENDING" || st.To != "RUNNING" || st.Stage != "task_implementation" {
		t.Fatalf("state entry = %+v", st)
	}
	if !strings.Contains(a.Entries[2].Content, "read-only") {
		t.Fatalf("a code stage without a worktree must say it runs read-only: %q", a.Entries[2].Content)
	}
	if a.Entries[3].Request.Source != "execute" || a.Entries[3].Request.Method == "" {
		t.Fatalf("request = %+v", a.Entries[3].Request)
	}
	n := len(a.Entries)
	if st := a.Entries[n-2].State; st.From != "RUNNING" || st.To != "WAITING_GATE_APPROVAL" {
		t.Fatalf("stage must settle into review, got %+v", st)
	}
	if !strings.Contains(a.Entries[n-1].Content, "approve to continue to e2e_validation") {
		t.Fatalf("operator next-step note = %q", a.Entries[n-1].Content)
	}

	r.mu.RLock()
	logs := strings.Join(r.taskProcesses["TASK-C1"].Logs, "\n")
	r.mu.RUnlock()
	for _, fake := range []string{"12/12 PASS", "Indexed symbols", "Spawning isolated container", "Zero syntax errors"} {
		if strings.Contains(logs, fake) {
			t.Fatalf("fabricated activity %q still emitted", fake)
		}
	}
}

// Typing while the stage is idle continues the stage in the agent's session, so the task runs and
// then goes to review — no separate "Continue session" click needed.
func TestMessageWhileStageIsIdleContinuesTheStage(t *testing.T) {
	stub := &streamingStub{}
	r := consoleRouter(t, stub)
	r.console.mu.Lock()
	c := r.console.get("TASK-C1")
	c.sessionID, c.sessionStage = "sess-live", "task_implementation"
	r.console.mu.Unlock()

	w := do(r, http.MethodPost, "/api/v1/tasks/TASK-C1/chat", `{"message":"fix the failing pipelines and push"}`)
	if w.Code != http.StatusAccepted || !strings.Contains(w.Body.String(), `"mode":"stage"`) {
		t.Fatalf("message → %d %s", w.Code, w.Body.String())
	}
	waitState(t, r, "TASK-C1", types.TaskStateWaitingGateApproval)
	stub.mu.Lock()
	last := stub.reqs[len(stub.reqs)-1]
	stub.mu.Unlock()
	prompt := last.Messages[len(last.Messages)-1].Content
	if last.SessionID != "sess-live" || !strings.Contains(prompt, "fix the failing pipelines and push") ||
		strings.Contains(prompt, "Implement the change") || strings.Contains(prompt, "rejected") {
		t.Fatalf("the message should continue the stage's session as the operator's instruction: session %q prompt %q", last.SessionID, prompt)
	}
	if last.Approver == nil || last.PermissionMode == "" {
		t.Fatal("a stage run needs its permission setup")
	}
	var user int
	for _, e := range activity(t, r, "").Entries {
		if e.Kind == types.ConsoleKindUser && e.Content == "fix the failing pipelines and push" {
			user++
		}
	}
	if user != 1 {
		t.Fatalf("the message should be recorded once, got %d", user)
	}

	// In review the stage is left alone: a message is a side conversation.
	if w := do(r, http.MethodPost, "/api/v1/tasks/TASK-C1/chat", `{"message":"why this approach?"}`); !strings.Contains(w.Body.String(), `"mode":"chat"`) {
		t.Fatalf("a message during review must not re-run the stage: %s", w.Body.String())
	}
	waitIdle(t, r)
	if s := r.taskSnapshot("TASK-C1").State; s != types.TaskStateWaitingGateApproval {
		t.Fatalf("review state must be kept, got %s", s)
	}
}
