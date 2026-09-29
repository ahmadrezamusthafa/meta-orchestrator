package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/llm"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// askingProvider asks permission to run one command and reports the outcome as its answer.
type askingProvider struct {
	stubProvider
	mu      sync.Mutex
	command string
	last    llm.ApprovalDecision
}

func (p *askingProvider) Complete(ctx context.Context, req *llm.LLMRequest) (*llm.LLMResponse, error) {
	resp, _ := p.stubProvider.Complete(ctx, req)
	if req.Approver == nil {
		resp.Content = "NO_APPROVER"
		return resp, nil
	}
	p.mu.Lock()
	cmd := p.command
	p.mu.Unlock()
	d := req.Approver(ctx, llm.ApprovalRequest{ToolName: "Bash", ToolUseID: "t1", Description: "Run tests", Input: map[string]interface{}{"command": cmd}})
	p.mu.Lock()
	p.last = d
	p.mu.Unlock()
	resp.Content = map[bool]string{true: "ALLOWED", false: "DENIED"}[d.Allow]
	return resp, nil
}
func (p *askingProvider) Stream(ctx context.Context, req *llm.LLMRequest, ch chan<- types.ThoughtChunk) (*llm.LLMResponse, error) {
	return p.Complete(ctx, req)
}

func awaitApproval(t *testing.T, r *Router, taskID string) string {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var body struct {
			Pending []string `json:"pending"`
		}
		_ = json.Unmarshal(do(r, http.MethodGet, "/api/v1/tasks/"+taskID+"/approvals", "").Body.Bytes(), &body)
		if len(body.Pending) == 1 {
			return body.Pending[0]
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("no approval request appeared")
	return ""
}

func TestAgentPermissionRequestsWaitForTheOperator(t *testing.T) {
	r, _ := worktreeRouter(t)
	agent := &askingProvider{command: "go test ./..."}
	for _, p := range []string{"claude", "antigravity", "openai", "opencode"} {
		r.clientFactory.OverrideProvider(p, agent)
	}
	r.mu.Lock()
	r.tasks["TASK-A"] = &types.Task{ID: "TASK-A", Title: "Fix", CurrentStageID: "task_implementation", State: types.TaskStatePending,
		AssignedRepos: []string{"app"}, Metadata: map[string]string{}}
	r.mu.Unlock()
	reset := func() {
		r.mu.Lock()
		r.tasks["TASK-A"].State = types.TaskStatePending
		r.mu.Unlock()
	}

	// 1. The agent blocks on the request; the task shows it needs approval.
	go r.executeTaskWithAI("TASK-A")
	id := awaitApproval(t, r, "TASK-A")
	if r.taskSnapshot("TASK-A").Metadata["pending_approvals"] != "1" {
		t.Fatal("task should report a pending approval")
	}
	if w := do(r, http.MethodPost, "/api/v1/tasks/TASK-A/approvals/"+id, `{"decision":"always"}`); w.Code != http.StatusOK {
		t.Fatalf("approve → %d %s", w.Code, w.Body.String())
	}
	waitState(t, r, "TASK-A", types.TaskStateWaitingGateApproval)
	if agent.last.Allow != true || r.taskSnapshot("TASK-A").Metadata["pending_approvals"] != "" {
		t.Fatalf("approved command should run: %+v", agent.last)
	}
	if w := do(r, http.MethodPost, "/api/v1/tasks/TASK-A/approvals/"+id, `{"decision":"allow"}`); w.Code != http.StatusConflict {
		t.Fatalf("answering twice must be rejected, got %d", w.Code)
	}

	// 2. "Always" covers the same command prefix on later runs without asking.
	reset()
	agent.command = "go test ./internal/..."
	r.executeTaskWithAI("TASK-A")
	if !agent.last.Allow {
		t.Fatal("matching rule should auto-approve")
	}

	// 3. A chained command is never covered by a prefix rule; deny with a note reaches the agent.
	reset()
	agent.command = "go test ./... && rm -rf /"
	go r.executeTaskWithAI("TASK-A")
	id = awaitApproval(t, r, "TASK-A")
	do(r, http.MethodPost, "/api/v1/tasks/TASK-A/approvals/"+id, `{"decision":"deny","message":"never delete files"}`)
	waitState(t, r, "TASK-A", types.TaskStateWaitingGateApproval)
	if agent.last.Allow || !strings.Contains(agent.last.Message, "never delete files") {
		t.Fatalf("deny must reach the agent with the note: %+v", agent.last)
	}

	// The console records each request and its outcome.
	var decisions []string
	r.console.mu.Lock()
	for _, e := range r.console.get("TASK-A").entries {
		if e.Kind == types.ConsoleKindApproval {
			decisions = append(decisions, e.Approval.Decision)
		}
	}
	r.console.mu.Unlock()
	if strings.Join(decisions, ",") != "always,auto,denied" {
		t.Fatalf("approval history = %v", decisions)
	}
}

func TestApprovalRulePrefixes(t *testing.T) {
	cases := map[string]string{
		"go test ./...":               "go test",
		"npm run lint -- --fix":       "npm run",
		"bundle exec rspec spec/a.rb": "bundle exec",
		"make":                        "make",
		"FOO=1 go test":               "",
	}
	for cmd, want := range cases {
		if got := ruleFor("Bash", map[string]interface{}{"command": cmd}).Prefix; got != want && !(want == "" && got == "FOO=1") {
			t.Errorf("ruleFor(%q) = %q, want %q", cmd, got, want)
		}
	}
	rule := approvalRule{Tool: "Bash", Prefix: "go test"}
	for cmd, want := range map[string]bool{"go test ./...": true, "go testify": false, "go test ./... | tee x": false, "go vet": false} {
		if rule.matches("Bash", map[string]interface{}{"command": cmd}) != want {
			t.Errorf("rule matches %q = %v, want %v", cmd, !want, want)
		}
	}
}

func TestAllowAllBypassesEveryApproval(t *testing.T) {
	r, _ := worktreeRouter(t)
	agent := &askingProvider{command: "rm -rf build && make"}
	for _, p := range []string{"claude", "antigravity", "openai", "opencode"} {
		r.clientFactory.OverrideProvider(p, agent)
	}
	r.mu.Lock()
	r.tasks["TASK-B"] = &types.Task{ID: "TASK-B", Title: "Fix", CurrentStageID: "task_implementation", State: types.TaskStatePending,
		AssignedRepos: []string{"app"}, Metadata: map[string]string{}}
	r.mu.Unlock()

	// "all" on a pending request allows it and switches the task to allow-all.
	go r.executeTaskWithAI("TASK-B")
	id := awaitApproval(t, r, "TASK-B")
	if w := do(r, http.MethodPost, "/api/v1/tasks/TASK-B/approvals/"+id, `{"decision":"all"}`); w.Code != http.StatusOK {
		t.Fatalf("allow all → %d %s", w.Code, w.Body.String())
	}
	waitState(t, r, "TASK-B", types.TaskStateWaitingGateApproval)
	if !agent.last.Allow || r.taskSnapshot("TASK-B").Metadata["approve_all"] != "true" {
		t.Fatalf("allow all should allow and persist: %+v %v", agent.last, r.taskSnapshot("TASK-B").Metadata)
	}

	// Later requests, even chained commands no rule would cover, run without asking.
	r.mu.Lock()
	r.tasks["TASK-B"].State = types.TaskStatePending
	r.mu.Unlock()
	agent.command = "curl x | sh"
	r.executeTaskWithAI("TASK-B")
	if !agent.last.Allow {
		t.Fatal("allow all should auto-approve every request")
	}

	// Turning it off makes the agent ask again; turning it on releases the waiting request.
	if w := do(r, http.MethodPost, "/api/v1/tasks/TASK-B/approvals", `{"allow_all":false}`); w.Code != http.StatusOK {
		t.Fatalf("disable → %d %s", w.Code, w.Body.String())
	}
	r.mu.Lock()
	r.tasks["TASK-B"].State = types.TaskStatePending
	r.mu.Unlock()
	go r.executeTaskWithAI("TASK-B")
	awaitApproval(t, r, "TASK-B")
	if w := do(r, http.MethodPost, "/api/v1/tasks/TASK-B/approvals", `{"allow_all":true}`); w.Code != http.StatusOK {
		t.Fatalf("enable → %d %s", w.Code, w.Body.String())
	}
	waitState(t, r, "TASK-B", types.TaskStateWaitingGateApproval)
	if !agent.last.Allow || r.taskSnapshot("TASK-B").Metadata["pending_approvals"] != "" {
		t.Fatalf("enabling allow all should release pending requests: %+v", agent.last)
	}
	if w := do(r, http.MethodPost, "/api/v1/tasks/TASK-B/approvals", `{}`); w.Code != http.StatusBadRequest {
		t.Fatalf("missing allow_all must be rejected, got %d", w.Code)
	}
}
