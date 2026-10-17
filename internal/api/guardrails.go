package api

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/atdd"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/circuitbreaker"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/llm"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/registry"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/security"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// Guardrails on the stage flow:
//   - the frustration breaker halts a task whose stage keeps failing with the same error
//     (BLOCKED_FRUSTRATION) until the operator changes something;
//   - during atdd_creation production source is write-locked: the agent writes tests only, and the
//     lock lifts once a run of the acceptance tests verifies the red phase, or when the task leaves
//     the stage;
//   - a successful atdd_creation writes a Playwright scaffold for its web cases.

const (
	atddStage = "atdd_creation"
	// defaultFrustrationThreshold applies when the task's method sets no sane limit.
	defaultFrustrationThreshold = 3
	redPhaseScaffold            = "atdd/acceptance.spec.ts"
)

// operatorTriggers are stage runs that follow an operator decision; they clear the failure count.
var operatorTriggers = map[string]bool{"reset": true, "guidance": true, "message": true, "gate_approved": true, "gate_rejected": true}

// initGuardrails builds the guardrails. The methods' iteration limits come from
// configs/methods.json under the root; without it every task uses the default threshold.
func (r *Router) initGuardrails() {
	r.methods = registry.NewMethodRegistry()
	if err := r.methods.LoadFromFile(filepath.Join(r.cfg.RootDir, "configs", "methods.json")); err != nil && !errors.Is(err, os.ErrNotExist) {
		fmt.Printf("[Guardrails] methods.json not loaded, using a frustration threshold of %d: %v\n", defaultFrustrationThreshold, err)
	}
	r.breaker = circuitbreaker.NewCircuitBreaker(defaultFrustrationThreshold, func(p *types.FrustrationHaltPayload) {
		if r.cfg.WSHub != nil {
			r.cfg.WSHub.BroadcastEvent(&types.OrchestratorEvent{Type: types.EventFrustrationHalt, TaskID: p.TaskID, StageID: p.StageID, Payload: p})
		}
	})
	r.writeLocks = security.NewWriteLockManager()
	r.interceptor = security.NewPolyglotSkillInterceptor(r.writeLocks)
	r.redPhase = atdd.NewRedPhaseVerifier(r.writeLocks)
}

// frustrationThreshold is how many identical stage failures in a row block the task: 3, or the
// task method's max_consecutive_iterations when that is stricter. The methods' limits count agent
// iterations, so a looser limit (the shipped ones are 10–30) must not delay the halt.
func (r *Router) frustrationThreshold(method string) int {
	if m, err := r.methods.Get(strings.ToLower(method)); err == nil && m.MaxConsecutiveIterations > 0 &&
		m.MaxConsecutiveIterations < defaultFrustrationThreshold {
		return m.MaxConsecutiveIterations
	}
	return defaultFrustrationThreshold
}

// recordStageOutcome feeds a settled stage into the frustration breaker. A failure that is not an
// interruption or a timeout counts; success clears the count. When the breaker trips the task is
// blocked and the halt is returned for reportFrustrationHalt. Caller holds r.mu.
func (r *Router) recordStageOutcome(task *types.Task, stage string, execErr error) *types.FrustrationHaltPayload {
	switch {
	case execErr == nil:
		r.breaker.Reset(task.ID)
		delete(task.Metadata, "frustration_count")
		delete(task.Metadata, "failing_trace")
		return nil
	case errors.Is(execErr, context.Canceled) || timedOut(execErr):
		return nil
	}
	sig := circuitbreaker.ComputeSignature(stage, execErr.Error())
	tripped, halt := r.breaker.RecordFailure(task.ID, stage, sig, execErr.Error(), r.frustrationThreshold(task.SelectedMethod))
	if !tripped {
		task.Metadata["frustration_count"] = strconv.Itoa(r.breaker.Count(task.ID))
		return nil
	}
	r.setTaskState(task, types.TaskStateBlockedFrustration)
	task.Metadata["frustration_count"] = strconv.Itoa(halt.ConsecutiveFails)
	task.Metadata["failing_trace"] = execErr.Error()
	proc := r.getOrCreateTaskProcessLocked(task.ID)
	proc.Status = "BLOCKED"
	proc.CurrentStep = "Blocked: " + stage + " keeps failing"
	return halt
}

// reportFrustrationHalt tells the operator why the task stopped and how to continue.
func (r *Router) reportFrustrationHalt(halt *types.FrustrationHaltPayload) {
	if halt == nil {
		return
	}
	r.addEntry(halt.TaskID, types.ConsoleEntry{Kind: types.ConsoleKindError, Content: fmt.Sprintf(
		"⛔ Halted: %s failed %d times in a row with the same error, so retrying it unchanged would fail the same way.\n"+
			"Give the agent guidance or type a message with a different approach (the stage re-runs with it), "+
			"or Reset & retry to start a fresh attempt. Continue retries as-is; the counter starts again.",
		halt.StageID, halt.ConsecutiveFails)})
}

// stageGuard enforces the ATDD write lock on one agent turn and records the acceptance-test runs
// the agent makes, for the red-phase verdict. A nil guard does nothing.
type stageGuard struct {
	r      *Router
	taskID string
	turnID string
	mu     sync.Mutex
	cmds   map[string]string // tool use id → test command
	denied map[string]bool   // tool use ids refused permission
	last   *testRun
}

type testRun struct {
	command string
	output  string
	isError bool
}

// beginGuardrails prepares a claimed stage run: an operator decision clears the failure count, and
// atdd_creation runs write-locked in a mode where every file edit reaches the approver (acceptEdits
// would bypass it). Any other stage lifts the lock.
func (r *Router) beginGuardrails(run *stageRun, workDir, permission, planExit string) (string, string, *stageGuard) {
	taskID := run.task.ID
	if operatorTriggers[run.trigger] {
		r.breaker.Reset(taskID)
	}
	if run.task.CurrentStageID != atddStage {
		r.writeLocks.UnlockTask(taskID)
		return permission, planExit, nil
	}
	r.writeLocks.LockTask(taskID, workDir)
	if permission == modeAcceptEdits {
		permission = modeDefault
	}
	if planExit == modeAcceptEdits {
		planExit = modeDefault
	}
	r.addEntry(taskID, types.ConsoleEntry{Kind: types.ConsoleKindSystem, TurnID: run.turnID, Content: "ATDD red phase: production source is " +
		"write-locked. Edits to tests, specs, fixtures and .sdlc files are approved automatically; writes to any other file are refused. " +
		"The lock lifts when a run of the acceptance tests fails on its assertions (red phase verified), or when the task leaves this stage."})
	return permission, planExit, r.newStageGuard(taskID, run.turnID)
}

func (r *Router) newStageGuard(taskID, turnID string) *stageGuard {
	return &stageGuard{r: r, taskID: taskID, turnID: turnID, cmds: map[string]string{}, denied: map[string]bool{}}
}

// chatGuard applies a still-held ATDD write lock to a chat turn on the ATDD stage.
func (r *Router) chatGuard(task *types.Task, turnID, mode string) (string, *stageGuard) {
	if task.CurrentStageID != atddStage || !r.writeLocks.IsLocked(task.ID) {
		return mode, nil
	}
	if mode == modeAcceptEdits {
		mode = modeDefault
	}
	return mode, r.newStageGuard(task.ID, turnID)
}

// brief tells the agent about the write lock.
func (g *stageGuard) brief() string {
	if g == nil {
		return ""
	}
	return "\nWrite lock (ATDD red phase): write only acceptance tests, specs, fixtures and .sdlc files — production source " +
		"is locked and such writes are refused. Run the acceptance tests before you finish: they must fail on their assertions " +
		"because the feature is not implemented yet."
}

// approver wraps next: a write to a protected file is refused, a test-file edit is approved
// without asking, and an approved plan never switches the agent to acceptEdits.
func (g *stageGuard) approver(next llm.Approver) llm.Approver {
	if g == nil || next == nil {
		return next
	}
	return func(ctx context.Context, req llm.ApprovalRequest) llm.ApprovalDecision {
		if err := g.r.interceptor.InterceptToolCall(g.taskID, req.ToolName, req.Input); err != nil {
			g.r.addEntry(g.taskID, types.ConsoleEntry{Kind: types.ConsoleKindSystem, TurnID: g.turnID, Content: fmt.Sprintf(
				"Refused %s `%s`: production source is write-locked during the ATDD red phase.", req.ToolName, approvalSummary(req.ToolName, req.Input))})
			return llm.ApprovalDecision{Message: err.Error() + ". Write the acceptance tests only; production code is written in task_implementation."}
		}
		if security.IsFileWrite(req.ToolName) && g.r.writeLocks.IsLocked(g.taskID) {
			return llm.ApprovalDecision{Allow: true, Mode: modeDefault}
		}
		d := next(ctx, req)
		if d.Mode == modeAcceptEdits {
			d.Mode = modeDefault
		}
		return d
	}
}

// decided records the outcome of every permission prompt of the turn: a refused command never ran,
// so its error result is not a test run.
func (g *stageGuard) decided(req llm.ApprovalRequest, d llm.ApprovalDecision) {
	if g == nil || d.Allow || req.ToolUseID == "" {
		return
	}
	g.mu.Lock()
	g.denied[req.ToolUseID] = true
	g.mu.Unlock()
}

// onTool records the acceptance-test commands the agent runs and how each one ended.
func (g *stageGuard) onTool(ev llm.StreamEvent) {
	if g == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	switch ev.Type {
	case llm.StreamToolUse:
		if cmd, _ := ev.ToolInput["command"].(string); ev.ToolName == "Bash" && atdd.IsTestCommand(cmd) {
			g.cmds[ev.ToolID] = strings.TrimSpace(cmd)
		}
	case llm.StreamToolResult:
		cmd, ok := g.cmds[ev.ToolID]
		if !ok || g.denied[ev.ToolID] {
			return
		}
		g.last = &testRun{command: cmd, output: ev.Text, isError: ev.IsError}
	}
}

var exitCodeLine = regexp.MustCompile(`(?m)^\s*Exit code:?\s*(\d+)`)

// exitCode reads the exit status from a Bash tool result: the CLI reports a failing command as an
// error result headed "Exit code N".
func (t *testRun) exitCode() int {
	if m := exitCodeLine.FindStringSubmatch(t.output); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil {
			return n
		}
	}
	if t.isError {
		return 1
	}
	return 0
}

// settleATDD records the red-phase verdict of a finished ATDD turn and writes the Playwright
// scaffold for its web cases. An interrupted or failed turn gets no verdict.
func (r *Router) settleATDD(g *stageGuard, resp *llm.LLMResponse, execErr error) {
	if g == nil || execErr != nil || resp == nil {
		return
	}
	g.mu.Lock()
	last := g.last
	g.mu.Unlock()

	status, summary := "not_verified", "not verified: the agent did not run the acceptance tests"
	command := ""
	if last != nil {
		command = last.command
		res, err := r.redPhase.VerifyRedPhase(context.Background(), g.taskID, last.exitCode(), last.output)
		switch {
		case res == nil:
			summary = "not verified: " + err.Error()
		case res.Passed:
			status, summary = "verified", res.FailureSummary
		default:
			status, summary = "invalid", res.FailureSummary
		}
	}
	r.mu.Lock()
	if t := r.tasks[g.taskID]; t != nil {
		if t.Metadata == nil {
			t.Metadata = map[string]string{}
		}
		t.Metadata["red_phase"] = status
		t.Metadata["red_phase_summary"] = summary
		if command != "" {
			t.Metadata["red_phase_command"] = command
		} else {
			delete(t.Metadata, "red_phase_command")
		}
	}
	r.mu.Unlock()

	note := "✓ Red phase verified"
	switch status {
	case "verified":
		note += fmt.Sprintf(" by `%s`: %s. Production source is unlocked.", command, summary)
	case "invalid":
		note = fmt.Sprintf("⚠ Red phase invalid (`%s`): %s. Production source stays locked; ask the agent to fix the tests, or reject this stage with feedback.", command, summary)
	default:
		note = "⚠ Red phase " + summary + ". Production source stays locked while the task is on this stage; ask the agent to run them, or reject with feedback."
	}
	r.addEntry(g.taskID, types.ConsoleEntry{Kind: types.ConsoleKindSystem, TurnID: g.turnID, Content: note})
	r.writeATDDScaffold(g.taskID, g.turnID, resp.Content)
}

// writeATDDScaffold writes a Playwright spec for the web cases of the stage's ```atdd-cases block.
func (r *Router) writeATDDScaffold(taskID, turnID, content string) {
	cases, err := atdd.ParseBlock(content)
	if err != nil {
		return // the stage output carries no cases; the UAT stage reports that
	}
	var web []atdd.Case
	for _, c := range cases {
		if atdd.IsWebCase(c) {
			web = append(web, c)
		}
	}
	if len(web) == 0 {
		return
	}
	path, err := r.resolveArtifactPath(taskID, redPhaseScaffold)
	if err != nil {
		return
	}
	title := taskID
	if t := r.taskSnapshot(taskID); t != nil && t.Title != "" {
		title = t.Title
	}
	spec := atdd.NewATDDGenerator().GeneratePlaywrightSpec(title, web)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err == nil {
		err = os.WriteFile(path, []byte(spec), 0o644)
	}
	msg := fmt.Sprintf("Wrote a Playwright scaffold for %d web case(s) to .sdlc/artifacts/%s/%s (Artifacts tab). "+
		"Every step throws \"not implemented\" until it is written, so it cannot pass by accident.", len(web), taskID, redPhaseScaffold)
	if err != nil {
		msg = fmt.Sprintf("⚠ Could not write the Playwright scaffold for %d web case(s): %v", len(web), err)
	}
	r.addEntry(taskID, types.ConsoleEntry{Kind: types.ConsoleKindSystem, TurnID: turnID, Content: msg})
}
