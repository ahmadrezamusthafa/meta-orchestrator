package api

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
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

// interactionProvider files one fixed approval request and records the decision.
type interactionProvider struct {
	stubProvider
	mu   sync.Mutex
	ask  llm.ApprovalRequest
	reqs []llm.LLMRequest
	last llm.ApprovalDecision
}

func (p *interactionProvider) Complete(ctx context.Context, req *llm.LLMRequest) (*llm.LLMResponse, error) {
	resp, _ := p.stubProvider.Complete(ctx, req)
	p.mu.Lock()
	p.reqs = append(p.reqs, *req)
	ask := p.ask
	p.mu.Unlock()
	if req.Approver == nil {
		resp.Content = "NO_APPROVER"
		return resp, nil
	}
	d := req.Approver(ctx, ask)
	p.mu.Lock()
	p.last = d
	p.mu.Unlock()
	return resp, nil
}
func (p *interactionProvider) Stream(ctx context.Context, req *llm.LLMRequest, ch chan<- types.ThoughtChunk) (*llm.LLMResponse, error) {
	return p.Complete(ctx, req)
}
func (p *interactionProvider) decision() llm.ApprovalDecision {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.last
}

func interactionRouter(t *testing.T, stage string, ask llm.ApprovalRequest) (*Router, *interactionProvider) {
	r, _ := worktreeRouter(t)
	agent := &interactionProvider{ask: ask}
	for _, p := range []string{"claude", "antigravity", "openai", "opencode"} {
		r.clientFactory.OverrideProvider(p, agent)
	}
	r.mu.Lock()
	r.tasks["TASK-P"] = &types.Task{ID: "TASK-P", Title: "Fix CI", CurrentStageID: stage, State: types.TaskStatePending,
		AssignedRepos: []string{"app"}, Metadata: map[string]string{}}
	r.mu.Unlock()
	return r, agent
}

func lastApproval(r *Router, taskID string) *types.ConsoleApproval {
	r.console.mu.Lock()
	defer r.console.mu.Unlock()
	entries := r.console.get(taskID).entries
	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i].Approval != nil {
			a := *entries[i].Approval
			return &a
		}
	}
	return nil
}

func waitChatIdle(t *testing.T, r *Router, taskID string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		r.console.mu.Lock()
		busy := r.console.get(taskID).busy
		r.console.mu.Unlock()
		if !busy {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("chat turn never finished")
}

// The incident: console chat ran read-only with nobody to ask, so pushes and fixes were silently
// refused. Chat turns must reach the operator.
func TestChatTurnsAskTheOperatorInsteadOfSilentlyRefusing(t *testing.T) {
	r, agent := interactionRouter(t, "signoff_merge", llm.ApprovalRequest{ToolName: "Bash", ToolUseID: "t1",
		Input: map[string]interface{}{"command": "git push origin fix-ci"}})
	if w := do(r, http.MethodPost, "/api/v1/tasks/TASK-P/chat", `{"message":"fix the pipelines and push"}`); w.Code != http.StatusAccepted {
		t.Fatalf("chat → %d %s", w.Code, w.Body.String())
	}
	id := awaitApproval(t, r, "TASK-P")
	do(r, http.MethodPost, "/api/v1/tasks/TASK-P/approvals/"+id, `{"decision":"allow"}`)
	waitChatIdle(t, r, "TASK-P")
	if !agent.decision().Allow {
		t.Fatal("the approved push must be allowed")
	}
	req := agent.reqs[0]
	if req.PermissionMode != "acceptEdits" || !r.isTaskWorktree("TASK-P", req.WorkDir) {
		t.Fatalf("chat in the task worktree should edit freely and ask for the rest: mode=%q dir=%q", req.PermissionMode, req.WorkDir)
	}
	if !strings.Contains(req.Messages[0].Content, "make the call directly") {
		t.Fatalf("the agent must be told how permissions work:\n%s", req.Messages[0].Content)
	}
}

func TestApprovingAPlanLetsTheAgentAct(t *testing.T) {
	r, agent := interactionRouter(t, "techdoc_rfc", llm.ApprovalRequest{ToolName: llm.ToolExitPlanMode, ToolUseID: "t1",
		Input: map[string]interface{}{"plan": "1. Fix the lint error\n2. Push"}})
	go r.executeTaskWithAI("TASK-P")
	id := awaitApproval(t, r, "TASK-P")
	a := lastApproval(r, "TASK-P")
	if a.Kind != types.ApprovalKindPlan || a.Summary != "1. Fix the lint error\n2. Push" || a.Mode != "acceptEdits" || a.RuleLabel != "" {
		t.Fatalf("plan request should show the plan and what approving grants: %+v", a)
	}
	do(r, http.MethodPost, "/api/v1/tasks/TASK-P/approvals/"+id, `{"decision":"allow"}`)
	waitState(t, r, "TASK-P", types.TaskStateWaitingGateApproval)
	if d := agent.decision(); !d.Allow || d.Mode != "acceptEdits" {
		t.Fatalf("approving the plan should switch the agent to acceptEdits: %+v", d)
	}

	// Rejecting keeps it planning, with the operator's note.
	r.mu.Lock()
	r.tasks["TASK-P"].State = types.TaskStatePending
	r.mu.Unlock()
	go r.executeTaskWithAI("TASK-P")
	id = awaitApproval(t, r, "TASK-P")
	do(r, http.MethodPost, "/api/v1/tasks/TASK-P/approvals/"+id, `{"decision":"deny","message":"cover the retry path too"}`)
	waitState(t, r, "TASK-P", types.TaskStateWaitingGateApproval)
	if d := agent.decision(); d.Allow || !strings.Contains(d.Message, "plan mode") || !strings.Contains(d.Message, "retry path") {
		t.Fatalf("rejecting the plan should keep the agent planning: %+v", d)
	}
}

func TestAgentQuestionsNeedTheOperatorsAnswer(t *testing.T) {
	r, agent := interactionRouter(t, "prd_discovery", llm.ApprovalRequest{ToolName: llm.ToolAskUserQuestion, ToolUseID: "t1",
		Input: map[string]interface{}{"questions": []interface{}{map[string]interface{}{"question": "Which branch?", "header": "Branch",
			"options": []interface{}{map[string]interface{}{"label": "main"}, map[string]interface{}{"label": "release"}}}}}})
	r.setAllowAll("TASK-P", true) // allow-all cannot answer a question
	go r.executeTaskWithAI("TASK-P")
	id := awaitApproval(t, r, "TASK-P")
	a := lastApproval(r, "TASK-P")
	if a.Kind != types.ApprovalKindQuestion || len(a.Questions) != 1 || len(a.Questions[0].Options) != 2 {
		t.Fatalf("question request should carry its questions: %+v", a)
	}
	if w := do(r, http.MethodPost, "/api/v1/tasks/TASK-P/approvals/"+id, `{"decision":"allow"}`); w.Code != http.StatusBadRequest {
		t.Fatalf("allowing without answers must be rejected, got %d", w.Code)
	}
	if w := do(r, http.MethodPost, "/api/v1/tasks/TASK-P/approvals/"+id, `{"decision":"allow","answers":{"Which branch?":"release"}}`); w.Code != http.StatusOK {
		t.Fatalf("answer → %d %s", w.Code, w.Body.String())
	}
	waitState(t, r, "TASK-P", types.TaskStateWaitingGateApproval)
	d := agent.decision()
	answers, _ := d.UpdatedInput["answers"].(map[string]string)
	if !d.Allow || answers["Which branch?"] != "release" || d.UpdatedInput["questions"] == nil {
		t.Fatalf("the answer must reach the agent in the tool input: %+v", d)
	}
	if got := lastApproval(r, "TASK-P").Answers["Which branch?"]; got != "release" {
		t.Fatalf("the console should record the answer, got %q", got)
	}
}

func TestDenialSummaryNamesWhatWasBlocked(t *testing.T) {
	if denialSummary(nil) != "" {
		t.Fatal("no denials, no note")
	}
	s := denialSummary([]llm.PermissionDenial{{ToolName: "Bash", Input: map[string]interface{}{"command": "git push"}},
		{ToolName: llm.ToolExitPlanMode}})
	if !strings.Contains(s, "2 action(s)") || !strings.Contains(s, "Bash `git push`") || !strings.Contains(s, "plan not approved") {
		t.Fatalf("summary = %q", s)
	}
}

func TestBlindConflictResolutionAlwaysAsks(t *testing.T) {
	for cmd, want := range map[string]bool{
		"git checkout --theirs api_spec.yaml":       true,
		"git checkout --ours -- .":                  true,
		"git restore --source=HEAD --theirs a.yaml": true,
		"git merge -X theirs origin/master":         true,
		"git merge --strategy-option=ours origin/m": true,
		"git merge -s ours origin/master":           true,
		"git checkout feature/ours-and-theirs":      false,
		"git merge origin/master":                   false,
		"git log --merge -p api_spec.yaml":          false,
	} {
		if got := isBlindResolution("Bash", map[string]interface{}{"command": cmd}); got != want {
			t.Errorf("isBlindResolution(%q) = %v, want %v", cmd, got, want)
		}
	}

	r, agent := interactionRouter(t, "task_implementation", llm.ApprovalRequest{ToolName: "Bash", ToolUseID: "t1",
		Input: map[string]interface{}{"command": "git checkout --theirs api_spec.yaml"}})
	r.setAllowAll("TASK-P", true)
	go r.executeTaskWithAI("TASK-P")
	id := awaitApproval(t, r, "TASK-P") // allow-all does not answer it
	if a := lastApproval(r, "TASK-P"); a.Warning == "" || a.RuleLabel != "" {
		t.Fatalf("a blind resolution should warn and offer no rule: %+v", a)
	}
	do(r, http.MethodPost, "/api/v1/tasks/TASK-P/approvals/"+id, `{"decision":"deny","message":"merge both sides"}`)
	waitState(t, r, "TASK-P", types.TaskStateWaitingGateApproval)
	if d := agent.decision(); d.Allow || !strings.Contains(d.Message, "merge both sides") {
		t.Fatalf("deny must reach the agent: %+v", d)
	}
}

func TestTurnEndReportsConflictsLeftInTheWorktree(t *testing.T) {
	r, agent := interactionRouter(t, "task_implementation", llm.ApprovalRequest{ToolName: "Read", Input: map[string]interface{}{}})
	r.setAllowAll("TASK-P", true)
	dir := r.resolveWorkDir(r.taskSnapshot("TASK-P"))
	_ = os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n<<<<<<< HEAD\nx\n=======\ny\n>>>>>>> origin/master\n"), 0o644)
	r.executeTaskWithAI("TASK-P")
	_ = agent
	r.console.mu.Lock()
	defer r.console.mu.Unlock()
	for _, e := range r.console.get("TASK-P").entries {
		if strings.Contains(e.Content, "unfinished conflict") && strings.Contains(e.Content, "main.go:") {
			return
		}
	}
	t.Fatal("the console must warn about conflict markers left in the worktree")
}

// The incident's second half: the PR flow committed a half-merged api_spec.yaml, markers and all.
func TestPullRequestIsBlockedWhileAConflictIsUnfinished(t *testing.T) {
	r, _ := interactionRouter(t, "task_implementation", llm.ApprovalRequest{})
	dir := r.resolveWorkDir(r.taskSnapshot("TASK-P"))
	_ = os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n<<<<<<< HEAD\nx\n=======\ny\n>>>>>>> origin/master\n"), 0o644)
	drafts := r.buildPRDrafts(context.Background(), r.taskSnapshot("TASK-P"))
	if len(drafts) != 1 || drafts[0].CanCreate || !strings.Contains(drafts[0].Blocker, "conflict markers at main.go:") {
		t.Fatalf("a pull request must not be opened over conflict markers: %+v", drafts[0])
	}
}
