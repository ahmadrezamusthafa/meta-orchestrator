package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/llm"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/router"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/telemetry"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// Task execution lifecycle (single source of truth for task state):
//
//	PENDING / SUSPENDED / FAILED / WAITING_GATE_APPROVAL(reject) --run--> RUNNING   (only while an agent turn is live)
//	RUNNING --success--> WAITING_GATE_APPROVAL --approve--> next stage RUNNING … last stage → COMPLETED
//	RUNNING --error--> FAILED        RUNNING --pause/esc--> SUSPENDED
//	any --unmet deps--> WAITING_DEPENDENCY --deps completed--> RUNNING (automatic)
//
// Every transition is written to the task console with the next action the operator can take.

// stagePipeline is the default SDLC stage order.
var stagePipeline = []string{
	"prd_discovery", "atdd_creation", "techdoc_rfc", "task_breakdown",
	"task_implementation", "e2e_validation", "uat_verification", "signoff_merge",
}

var stageInstructions = map[string]string{
	"prd_discovery":    "Clarify the requirement: goals, scope and non-goals, acceptance criteria, assumptions and open questions.",
	"repo_discovery":   "Identify the repositories, modules and files this task touches and why.",
	"atdd_creation":    atddStageInstruction,
	"techdoc_rfc":      "Write a concise technical design: approach, components, data/API changes, risks and alternatives considered.",
	"task_breakdown":   "Break the work into small ordered sub-tasks, each with the repository, files and how it will be verified.",
	"red_verification": "Explain which acceptance tests are expected to fail before implementation and why.",
	"task_implementation": "Implement the change. Show concrete code changes per file (unified diff where possible) and how to verify them. " +
		"Organize the report under the headings \"## Summary\" (what changed and why), \"## How to test\" (steps a reviewer follows) and " +
		"\"## Risks\" (impact and rollback) — they become the standard pull request description.",
	"e2e_validation":   "Describe the end-to-end validation: scenarios, commands or scripts to run, and expected results.",
	"uat_verification": uatStageInstruction,
	"signoff_merge":    "Prepare the sign-off: summary of changes, risks, rollout notes and a pull request description.",
}

func stageIndexOf(stage string) int {
	for i, s := range stagePipeline {
		if s == stage {
			return i
		}
	}
	return -1
}

// nextStage returns the stage after current, or "" when current is the last one.
func nextStage(current string) string {
	i := stageIndexOf(current)
	if i < 0 || i+1 >= len(stagePipeline) {
		return ""
	}
	return stagePipeline[i+1]
}

func stageLabel(stage string) string {
	if i := stageIndexOf(stage); i >= 0 {
		return fmt.Sprintf("%s (stage %d of %d)", stage, i+1, len(stagePipeline))
	}
	return stage
}

var (
	errTaskNotFound = errors.New("task not found")
	errTaskBusy     = errors.New("an agent turn is already running for this task")
	errTaskDone     = errors.New("task is already completed")
	errNoRepos      = errors.New("assign at least one repository before running this stage — it changes code")
)

// codeStages change repository files: they need assigned repositories and, inside an isolated
// worktree, may edit files. Every other stage is read-only analysis.
var codeStages = map[string]bool{"atdd_creation": true, "task_implementation": true, "e2e_validation": true}

// depsBlockedError reports unmet prerequisite tasks.
type depsBlockedError struct{ Unmet []string }

func (e *depsBlockedError) Error() string {
	return "waiting for prerequisite task(s): " + strings.Join(e.Unmet, ", ")
}

// stageRun is a claimed, ready-to-execute stage.
type stageRun struct {
	task     *types.Task // snapshot at claim time
	turnID   string
	ctx      context.Context
	cancel   context.CancelFunc
	trigger  string
	feedback string // gate rejection feedback, or the operator's message for the "message" trigger
	lastErr  string // why the previous run of this stage ended, if it failed
	model    string // operator-selected model; empty uses the router
}

// claimStage validates and marks a task RUNNING for its current stage. Only one claim can hold a
// task's console at a time, so RUNNING always means an agent turn is live.
func (r *Router) claimStage(taskID, trigger, feedback string) (*stageRun, error) {
	r.mu.Lock()
	task, ok := r.tasks[taskID]
	if !ok {
		r.mu.Unlock()
		return nil, errTaskNotFound
	}
	if task.State == types.TaskStateCompleted {
		r.mu.Unlock()
		return nil, errTaskDone
	}
	if codeStages[task.CurrentStageID] && len(task.AssignedRepos) == 0 {
		r.mu.Unlock()
		return nil, errNoRepos
	}
	var unmet []string
	for _, dep := range task.Dependencies {
		if d, ok := r.tasks[dep]; !ok || d.State != types.TaskStateCompleted {
			unmet = append(unmet, dep)
		}
	}
	if len(unmet) > 0 {
		wasWaiting := task.State == types.TaskStateWaitingDependency
		r.setTaskState(task, types.TaskStateWaitingDependency)
		if task.Metadata == nil {
			task.Metadata = map[string]string{}
		}
		task.Metadata["unmet_dependencies"] = strings.Join(unmet, ",")
		proc := r.getOrCreateTaskProcessLocked(taskID)
		proc.Status = "PAUSED"
		proc.CurrentStep = "Waiting for " + strings.Join(unmet, ", ")
		snap := cloneTask(task)
		r.mu.Unlock()
		if !wasWaiting {
			r.addEntry(taskID, types.ConsoleEntry{Kind: types.ConsoleKindSystem, Content: fmt.Sprintf(
				"Waiting for %s to complete. This task starts automatically once they are done.", strings.Join(unmet, ", "))})
		}
		r.broadcastTask(snap)
		return nil, &depsBlockedError{Unmet: unmet}
	}

	turnID := r.console.nextID("turn")
	ctx, cancel := context.WithTimeout(context.Background(), agentTurnTimeout)
	if err := r.beginTurn(taskID, turnID, cancel); err != nil {
		r.mu.Unlock()
		cancel()
		return nil, errTaskBusy
	}
	lastErr := task.Metadata["last_error"]
	delete(task.Metadata, "unmet_dependencies")
	delete(task.Metadata, "last_error")
	r.setTaskState(task, types.TaskStateRunning)
	task.UpdatedAt = time.Now()
	proc := r.getOrCreateTaskProcessLocked(taskID)
	proc.Status = "RUNNING"
	proc.StartedAt = time.Now()
	proc.CurrentStep = "Running " + stageLabel(task.CurrentStageID)
	snap := cloneTask(task)
	r.mu.Unlock()

	r.broadcastTask(snap)
	return &stageRun{task: snap, turnID: turnID, ctx: ctx, cancel: cancel, trigger: trigger, feedback: feedback, lastErr: lastErr}, nil
}

// agentTurnTimeout backstops one stage or chat turn. The driver stops a turn first — after
// llm.DefaultAgentIdleTimeout without output (waits on operator approvals excluded) or at
// llm.DefaultAgentTurnLimit — with an error that says which; this margin keeps that message.
var agentTurnTimeout = llm.DefaultAgentTurnLimit + 5*time.Minute

// startStage claims the task's current stage and executes it in the background.
func (r *Router) startStage(taskID, trigger, feedback string) error {
	run, err := r.claimStage(taskID, trigger, feedback)
	if errors.Is(err, errNoRepos) {
		if t := r.taskSnapshot(taskID); t != nil {
			r.addEntry(taskID, types.ConsoleEntry{Kind: types.ConsoleKindSystem, Content: fmt.Sprintf(
				"%s changes code, so it needs repositories. Assign them in Workspace & Repos, then run the stage.", t.CurrentStageID)})
		}
	}
	if err != nil {
		return err
	}
	go r.executeStage(run)
	return nil
}

// stageIdleStates are the states in which a typed message continues the current stage rather than
// starting a side conversation.
var stageIdleStates = map[types.TaskState]bool{types.TaskStatePending: true, types.TaskStateSuspended: true, types.TaskStateFailed: true}

// messageRunsStage reports whether an operator message should continue the task's current stage:
// the stage is idle and could start right now (repositories assigned, prerequisites done).
func (r *Router) messageRunsStage(taskID string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t := r.tasks[taskID]
	if t == nil || !stageIdleStates[t.State] || (codeStages[t.CurrentStageID] && len(t.AssignedRepos) == 0) {
		return false
	}
	for _, dep := range t.Dependencies {
		if d, ok := r.tasks[dep]; !ok || d.State != types.TaskStateCompleted {
			return false
		}
	}
	return true
}

// startStageWithMessage continues the current stage in the agent's session with the operator's
// message as its instruction, so typing in the console moves the stage forward like Continue does.
func (r *Router) startStageWithMessage(taskID, message, model string) (turnID, entryID string, err error) {
	run, err := r.claimStage(taskID, "message", message)
	if err != nil {
		return "", "", err
	}
	run.model = model
	user := r.addEntry(taskID, types.ConsoleEntry{Kind: types.ConsoleKindUser, TurnID: run.turnID, Content: message})
	go r.executeStage(run)
	return run.turnID, user.ID, nil
}

// executeTaskWithAI runs the task's current stage synchronously (used by tests and callers that
// already run in their own goroutine).
func (r *Router) executeTaskWithAI(taskID string) {
	if run, err := r.claimStage(taskID, "run", ""); err == nil {
		r.executeStage(run)
	}
}

// executeStage runs one claimed stage through the router and settles the resulting state.
func (r *Router) executeStage(run *stageRun) {
	defer run.cancel()
	task := run.task
	taskID := task.ID

	complexity := task.Metadata["complexity"]
	if complexity == "" {
		complexity = "MEDIUM"
	}
	taskType := task.Metadata["task_type"]
	if taskType == "" {
		taskType = router.ClassifyTaskType(task.Title, task.Description)
	}
	decision := r.routeTask(task, complexity, taskType)
	if run.model != "" {
		decision.Model, decision.FallbackChain, decision.Reasoning = run.model, []string{run.model}, "operator-selected model"
	}

	r.mu.Lock()
	if t, ok := r.tasks[taskID]; ok {
		if t.Metadata == nil {
			t.Metadata = map[string]string{}
		}
		t.Metadata["router_strategy"] = decision.Strategy
		t.Metadata["active_model"] = decision.Model // routed pick; replaced by the served model once the stage answers
		t.Metadata["router_reasoning"] = decision.Reasoning
		t.Metadata["task_type"] = taskType
		t.SelectedMethod = decision.Method
	}
	proc := r.getOrCreateTaskProcessLocked(taskID)
	proc.Command = fmt.Sprintf("stage %s · %s · %s", task.CurrentStageID, decision.Model, decision.Method)
	proc.ActiveAgent = decision.Method
	r.mu.Unlock()

	r.addEntry(taskID, types.ConsoleEntry{Kind: types.ConsoleKindSystem, TurnID: run.turnID, Content: fmt.Sprintf(
		"▶ Running %s — %s. Routed by %s: %s", stageLabel(task.CurrentStageID), triggerText(run.trigger), decision.Strategy, decision.Reasoning)})

	if r.budgetTracker != nil {
		r.budgetTracker.SetTaskBudget(taskID, decision.TokenBudget)
	}
	if r.telemetry != nil {
		r.telemetry.ttr.StartRun(taskRunMeta(task, decision, complexity, taskType))
	}

	r.console.mu.Lock()
	c := r.console.get(taskID)
	history := chatHistory(c.entries)
	sessionID := c.sessionID
	canContinue := sessionID != "" && c.sessionStage == task.CurrentStageID
	r.console.mu.Unlock()

	msgs := append([]llm.Message{{Role: llm.RoleSystem, Content: taskSystemPrompt(task, decision.Method)}}, history...)
	message := run.trigger == "message"
	continuing := (run.trigger == "resume" || message) && canContinue
	feedback := run.feedback
	if message {
		feedback = "" // the operator's message is not rejection feedback
	}
	prompt := stagePrompt(task, feedback)
	if message {
		prompt = messagePrompt(task, run.feedback, sessionID != "", canContinue, prompt)
		if sessionID != "" {
			r.addEntry(taskID, types.ConsoleEntry{Kind: types.ConsoleKindSystem, TurnID: run.turnID, Content: fmt.Sprintf(
				"Continuing session %s on %s with your message.", shortID(sessionID), task.CurrentStageID)})
		}
		if !canContinue && task.CurrentStageID == "uat_verification" {
			prompt += "\n\n" + r.uatStageContext(task)
		}
	} else if run.trigger == "resume" && canContinue {
		// Continue the interrupted conversation instead of restarting the stage from its brief.
		prompt = continuePrompt(task, run.lastErr)
		r.addEntry(taskID, types.ConsoleEntry{Kind: types.ConsoleKindSystem, TurnID: run.turnID, Content: fmt.Sprintf(
			"Continuing session %s where the last run stopped.", shortID(sessionID))})
	} else {
		if run.trigger == "resume" && sessionID != "" {
			// The session last served another stage or the chat (e.g. the card was moved back): keep
			// the conversation, but brief the agent on this stage.
			prompt = sessionStagePrompt(task, prompt)
			r.addEntry(taskID, types.ConsoleEntry{Kind: types.ConsoleKindSystem, TurnID: run.turnID, Content: fmt.Sprintf(
				"Continuing session %s on %s — the agent keeps everything it already knows from this conversation.",
				shortID(sessionID), task.CurrentStageID)})
		}
		if task.CurrentStageID == "uat_verification" {
			prompt += "\n\n" + r.uatStageContext(task)
		}
	}
	skills, missing := r.stageSkills(task)
	if len(missing) > 0 {
		r.addEntry(taskID, types.ConsoleEntry{Kind: types.ConsoleKindSystem, TurnID: run.turnID, Content: fmt.Sprintf(
			"⚠ Skipping skill %s — disabled or no longer installed. Re-enable it on the Skills page, or remove it from this stage in the Routing tab.",
			strings.Join(missing, ", "))})
	}
	if len(skills) > 0 {
		if !continuing {
			// A continued session already holds the skills from the stage's first turn.
			prompt += skillsBrief(skills)
		}
		r.addEntry(taskID, types.ConsoleEntry{Kind: types.ConsoleKindSystem, TurnID: run.turnID, Content: fmt.Sprintf(
			"Skills for this stage: %s. Their instructions are in the agent's brief and their folders are readable to it; usage is reported when the stage finishes.",
			strings.Join(skillNames(skills), ", "))})
	}
	msgs = append(msgs, llm.Message{Role: llm.RoleUser, Content: prompt})

	workDir := r.resolveWorkDir(task)
	r.mu.Lock()
	r.getOrCreateTaskProcessLocked(taskID).WorkingDir = workDir
	r.mu.Unlock()
	// Code stages edit freely only inside the task's own worktree. Every other run starts read-only
	// (plan mode); if the agent needs to act it asks through its plan, and once the operator
	// approves, gated actions still ask — so nothing is ever silently refused.
	permission := modePlan
	planExit := r.editMode(taskID, workDir)
	if codeStages[task.CurrentStageID] && planExit == modeAcceptEdits {
		permission = modeAcceptEdits
		r.addEntry(taskID, types.ConsoleEntry{Kind: types.ConsoleKindSystem, TurnID: run.turnID, Content: fmt.Sprintf(
			"The agent may edit files in the task worktree (%s); commands and other actions ask for your approval here. Review edits in the Changes tab.", workDir)})
	} else if codeStages[task.CurrentStageID] {
		r.addEntry(taskID, types.ConsoleEntry{Kind: types.ConsoleKindSystem, TurnID: run.turnID, Content: "No task worktree is available, so the agent starts read-only and describes the changes. " +
			"If it asks to make them, approving its plan lets it act here, with every change still asking you first."})
	}
	msgs[0].Content += "\n" + permissionBrief(permission)
	resp, used, execErr := r.runAgentTurn(run.ctx, agentTurn{Source: "execute", TurnID: run.turnID, Task: task, Decision: decision,
		TaskType: taskType, Messages: msgs, SessionID: sessionID, WorkDir: workDir, AddDirs: skillDirs(skills), MaxTokens: 8192,
		PermissionMode: permission, Approver: r.approverFor(taskID, run.turnID, planExit)})
	r.endTurn(taskID, run.turnID, resp, used)
	r.notePRSync(taskID, run.turnID)
	if resp != nil && len(skills) > 0 {
		r.addEntry(taskID, types.ConsoleEntry{Kind: types.ConsoleKindSystem, TurnID: run.turnID, Content: skillUsageReport(skills, resp.ToolCalls)})
	}

	r.settleStage(taskID, task.CurrentStageID, resp, used, execErr)
}

func triggerText(trigger string) string {
	switch trigger {
	case "resume":
		return "resumed by operator"
	case "gate_approved":
		return "previous stage approved"
	case "gate_rejected":
		return "re-running with your feedback"
	case "dependency_cleared":
		return "prerequisites completed"
	case "reset":
		return "restarted after reset"
	case "guidance":
		return "re-running with your guidance"
	case "message":
		return "continuing with your message"
	default:
		return "started by operator"
	}
}

// stagePrompt is the user turn for a stage, including any operator feedback.
func stagePrompt(t *types.Task, feedback string) string {
	instr := stageInstructions[t.CurrentStageID]
	if instr == "" {
		instr = "Complete this stage of the task and report the result."
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Stage: %s\n%s\n\nTask: %s\n", stageLabel(t.CurrentStageID), instr, t.Title)
	if t.Description != "" {
		fmt.Fprintf(&b, "Description: %s\n", t.Description)
	}
	if g := t.Metadata["operator_guidance"]; g != "" {
		fmt.Fprintf(&b, "\nOperator guidance:\n%s\n", g)
	}
	if feedback != "" {
		fmt.Fprintf(&b, "\nThe operator rejected the previous output of this stage with this feedback — address it:\n%s\n", feedback)
	}
	b.WriteString("\nEnd with a short \"Summary\" section the operator can review before approving this stage.")
	return b.String()
}

// continuePrompt resumes a stage whose previous turn was paused or failed part-way. The agent
// keeps its session, so it only needs to be told to carry on — not handed the stage brief again.
func continuePrompt(t *types.Task, lastErr string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Continue %s from where you left off.", stageLabel(t.CurrentStageID))
	if lastErr != "" {
		fmt.Fprintf(&b, " Your previous run stopped with this error: %s.", lastErr)
	} else {
		b.WriteString(" Your previous run was interrupted.")
	}
	b.WriteString(" Check what you already did (including files changed in the working directory) and do not redo finished work.")
	if g := t.Metadata["operator_guidance"]; g != "" {
		fmt.Fprintf(&b, "\n\nOperator guidance:\n%s\n", g)
	}
	b.WriteString("\n\nEnd with a short \"Summary\" section covering the whole stage, which the operator can review before approving it.")
	return b.String()
}

// messagePrompt is the stage turn for an operator message typed while the stage was idle. A
// session that already works on this stage just gets the message; otherwise the agent is briefed
// on the stage first.
func messagePrompt(t *types.Task, msg string, hasSession, sameStage bool, brief string) string {
	var b strings.Builder
	switch {
	case sameStage:
		fmt.Fprintf(&b, "The operator says:\n%s\n\nAct on it as part of %s, continuing from where you are.", msg, stageLabel(t.CurrentStageID))
	case hasSession:
		fmt.Fprintf(&b, "%s\n\nThe operator says:\n%s", sessionStagePrompt(t, brief), msg)
	default:
		fmt.Fprintf(&b, "%s\n\nThe operator adds:\n%s", brief, msg)
	}
	b.WriteString("\n\nWhen this stage's work is done, end with a short \"Summary\" section covering the whole stage, which the operator can review before approving it.")
	return b.String()
}

// sessionStagePrompt briefs a continued conversation on a stage it has not run yet.
func sessionStagePrompt(t *types.Task, brief string) string {
	return fmt.Sprintf("Continue in this conversation, now on %s. Use what you already know from it and do not redo finished "+
		"work (check the working directory). Retry anything left unfinished, including actions that were refused permission "+
		"earlier: permission requests now reach the operator in the console.\n\n%s", stageLabel(t.CurrentStageID), brief)
}

// settleStage moves the task to its post-turn state and tells the operator what happens next.
func (r *Router) settleStage(taskID, stage string, resp *llm.LLMResponse, model string, execErr error) {
	var runRec telemetry.RunRecord
	var haveRun bool
	if r.telemetry != nil {
		runRec, haveRun = r.telemetry.ttr.Finish(taskID, execErr == nil)
		go func() { _, _ = r.recalibrate(context.Background()) }()
	}

	if execErr == nil && resp != nil {
		r.saveStageDocument(taskID, stage, model, resp.Content)
		if stage == "uat_verification" && strings.TrimSpace(resp.Content) != "" {
			defer r.startUATGuide(taskID) // after the gate state below is settled
		}
	}

	var note string
	r.mu.Lock()
	task, ok := r.tasks[taskID]
	if !ok {
		r.mu.Unlock()
		return
	}
	proc := r.getOrCreateTaskProcessLocked(taskID)
	proc.DurationSeconds = int64(time.Since(proc.StartedAt).Seconds())
	if haveRun {
		runRec.ApplyToTask(task)
	}
	if r.telemetry != nil {
		task.TokenUsage = r.telemetry.tracker.TaskUsage(taskID)
	} else if resp != nil {
		task.TokenUsage.PromptTokens += resp.TokenUsage.PromptTokens
		task.TokenUsage.CompletionTokens += resp.TokenUsage.CompletionTokens
		task.TokenUsage.CachedTokens += resp.TokenUsage.CachedTokens
		task.TokenUsage.TotalTokens += resp.TokenUsage.TotalTokens
		task.TokenUsage.EstimatedCostUSD += resp.TokenUsage.EstimatedCostUSD
	}
	if resp != nil && r.budgetTracker != nil {
		_, _ = r.budgetTracker.RecordUsage(taskID, resp.TokenUsage.PromptTokens, resp.TokenUsage.CompletionTokens, resp.TokenUsage.EstimatedCostUSD)
	}

	switch {
	case errors.Is(execErr, context.Canceled):
		// Pause already set SUSPENDED; an Esc interrupt lands here too.
		if task.State == types.TaskStateRunning {
			r.setTaskState(task, types.TaskStateSuspended)
		}
		proc.Status = "PAUSED"
		proc.CurrentStep = "Paused during " + stage
		note = fmt.Sprintf("⏸ Paused during %s. Resume to continue the agent's session where it stopped.", stage)
	case execErr != nil:
		r.setTaskState(task, types.TaskStateFailed)
		task.Metadata["last_error"] = execErr.Error()
		proc.Status = "FAILED"
		proc.CurrentStep = "Failed: " + stage
		note = fmt.Sprintf("✗ %s failed: %v\nContinue to pick up the agent's session where it stopped, or ask the agent about the failure below.", stage, execErr)
		if timedOut(execErr) {
			note = fmt.Sprintf("⏱ %s stopped: %v. The agent's session and the files it changed are kept — Continue picks up where it stopped.", stage, execErr)
		}
	default:
		r.setTaskState(task, types.TaskStateWaitingGateApproval)
		task.Metadata["active_model"] = servedModel(model, resp)
		proc.Status = "IDLE"
		proc.CurrentStep = "Awaiting review of " + stage
		if next := nextStage(stage); next != "" {
			note = fmt.Sprintf("✓ %s finished. Review the output above, then approve to continue to %s — or reject with feedback to re-run it.", stage, next)
		} else {
			note = fmt.Sprintf("✓ %s finished. This is the final stage — approve to complete the task, or reject with feedback.", stage)
		}
	}
	task.UpdatedAt = time.Now()
	snap := cloneTask(task)
	r.mu.Unlock()

	kind := types.ConsoleKindSystem
	if execErr != nil && !errors.Is(execErr, context.Canceled) {
		kind = types.ConsoleKindError
	}
	r.addEntry(taskID, types.ConsoleEntry{Kind: kind, Content: note})
	r.broadcastTask(snap)
}

// timedOut reports whether a turn ended because it ran out of time rather than because it failed.
func timedOut(err error) bool {
	return errors.Is(err, llm.ErrAgentIdle) || errors.Is(err, llm.ErrTurnTimeLimit) || errors.Is(err, context.DeadlineExceeded)
}

// pauseTask interrupts a live turn (if any) and suspends the task.
func (r *Router) pauseTask(taskID string) (*types.Task, error) {
	r.console.mu.Lock()
	cancel := r.console.get(taskID).cancel
	r.console.mu.Unlock()

	r.mu.Lock()
	task, ok := r.tasks[taskID]
	if !ok {
		r.mu.Unlock()
		return nil, errTaskNotFound
	}
	if task.State == types.TaskStateCompleted {
		r.mu.Unlock()
		return cloneTask(task), errTaskDone
	}
	r.setTaskState(task, types.TaskStateSuspended)
	task.UpdatedAt = time.Now()
	proc := r.getOrCreateTaskProcessLocked(taskID)
	proc.Status = "PAUSED"
	snap := cloneTask(task)
	r.mu.Unlock()

	if cancel != nil {
		cancel() // settleStage records the interruption
	} else {
		r.addEntry(taskID, types.ConsoleEntry{Kind: types.ConsoleKindSystem, Content: "⏸ Paused. Resume to run " + snap.CurrentStageID + "."})
	}
	r.broadcastTask(snap)
	return snap, nil
}

// interruptAndWait cancels a live turn and waits (bounded) for it to settle.
func (r *Router) interruptAndWait(taskID string, timeout time.Duration) {
	r.console.mu.Lock()
	cancel := r.console.get(taskID).cancel
	r.console.mu.Unlock()
	if cancel == nil {
		return
	}
	cancel()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		r.console.mu.Lock()
		busy := r.console.get(taskID).busy
		r.console.mu.Unlock()
		if !busy {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// reviewStage applies an operator gate decision. Approve advances (or completes) and runs the
// next stage; reject re-runs the stage with the feedback.
func (r *Router) reviewStage(taskID string, approved bool, feedback string) (*types.Task, error) {
	r.mu.Lock()
	task, ok := r.tasks[taskID]
	if !ok {
		r.mu.Unlock()
		return nil, errTaskNotFound
	}
	if task.State != types.TaskStateWaitingGateApproval {
		state := task.State
		r.mu.Unlock()
		return nil, fmt.Errorf("task is %s — there is no stage output waiting for review", state)
	}
	stage := task.CurrentStageID
	feedback = strings.TrimSpace(feedback)

	if !approved {
		r.mu.Unlock()
		if feedback == "" {
			return nil, errors.New("tell the agent what to change: feedback is required to reject a stage")
		}
		r.addEntry(taskID, types.ConsoleEntry{Kind: types.ConsoleKindUser, Content: "Rejected " + stage + ": " + feedback})
		if err := r.startStage(taskID, "gate_rejected", feedback); err != nil {
			return nil, err
		}
		return r.taskSnapshot(taskID), nil
	}

	next := nextStage(stage)
	if next == "" {
		r.setTaskState(task, types.TaskStateCompleted)
		task.UpdatedAt = time.Now()
		proc := r.getOrCreateTaskProcessLocked(taskID)
		proc.Status = "COMPLETED"
		proc.CurrentStep = "Completed"
		r.onTaskCompleted(task)
		snap := cloneTask(task)
		r.mu.Unlock()
		r.addEntry(taskID, types.ConsoleEntry{Kind: types.ConsoleKindSystem, Content: "✓ Task completed. All stages approved."})
		r.broadcastTask(snap)
		r.startUnblockedDependents(taskID)
		return snap, nil
	}
	task.CurrentStageID = next
	task.CurrentStageIndex = stageIndexOf(next)
	r.mu.Unlock()
	r.addEntry(taskID, types.ConsoleEntry{Kind: types.ConsoleKindSystem, Content: fmt.Sprintf("Approved %s → moving to %s.", stage, next)})
	if err := r.startStage(taskID, "gate_approved", ""); err != nil {
		return r.taskSnapshot(taskID), err
	}
	return r.taskSnapshot(taskID), nil
}

// startUnblockedDependents starts tasks that were waiting only on completedID.
func (r *Router) startUnblockedDependents(completedID string) {
	r.mu.RLock()
	var ready []string
	for id, t := range r.tasks {
		if t.State != types.TaskStateWaitingDependency {
			continue
		}
		needs, allDone := false, true
		for _, d := range t.Dependencies {
			if d == completedID {
				needs = true
			}
			if dt, ok := r.tasks[d]; !ok || dt.State != types.TaskStateCompleted {
				allDone = false
			}
		}
		if needs && allDone {
			ready = append(ready, id)
		}
	}
	r.mu.RUnlock()
	for _, id := range ready {
		_ = r.startStage(id, "dependency_cleared", "")
	}
}

// reconcileIdleRunning fixes tasks marked RUNNING with no live agent turn (e.g. seeded or left over
// from a previous daemon process) so the board never shows work that is not happening.
func (r *Router) reconcileIdleRunning() {
	r.mu.Lock()
	var fixed []*types.Task
	for id, t := range r.tasks {
		if t.State != types.TaskStateRunning {
			continue
		}
		r.console.mu.Lock()
		busy := r.console.get(id).busy
		r.console.mu.Unlock()
		if busy {
			continue
		}
		r.setTaskState(t, types.TaskStateSuspended)
		if p, ok := r.taskProcesses[id]; ok {
			p.Status = "PAUSED"
			p.CurrentStep = "Not running"
		}
		fixed = append(fixed, cloneTask(t))
	}
	r.mu.Unlock()
	for _, t := range fixed {
		r.addEntry(t.ID, types.ConsoleEntry{Kind: types.ConsoleKindSystem, Content: fmt.Sprintf(
			"No agent is running this task (the orchestrator started without an active session). Resume to run %s.", t.CurrentStageID)})
	}
}

func (r *Router) taskSnapshot(taskID string) *types.Task {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if t, ok := r.tasks[taskID]; ok {
		return cloneTask(t)
	}
	return nil
}

func (r *Router) broadcastTask(t *types.Task) {
	if r.cfg.WSHub == nil || t == nil {
		return
	}
	r.cfg.WSHub.BroadcastEvent(&types.OrchestratorEvent{Type: types.EventTaskStatus, TaskID: t.ID, StageID: t.CurrentStageID,
		Timestamp: time.Now(), Payload: t})
}

// respondStageStart maps a startStage result to an HTTP response.
func (r *Router) respondStageStart(w http.ResponseWriter, taskID string, err error) {
	var blocked *depsBlockedError
	switch {
	case err == nil:
		r.writeJSON(w, http.StatusOK, map[string]interface{}{"status": "execution_dispatched", "task_id": taskID, "task": r.taskSnapshot(taskID)})
	case errors.As(err, &blocked):
		r.writeJSON(w, http.StatusConflict, map[string]interface{}{"status": "blocked_dependency", "task_id": taskID,
			"error": err.Error(), "unmet": blocked.Unmet, "task": r.taskSnapshot(taskID)})
	case errors.Is(err, errTaskBusy):
		r.writeJSON(w, http.StatusConflict, map[string]interface{}{"status": "busy", "task_id": taskID, "error": err.Error()})
	case errors.Is(err, errNoRepos):
		r.writeJSON(w, http.StatusConflict, map[string]interface{}{"status": "needs_repositories", "task_id": taskID,
			"error": err.Error(), "task": r.taskSnapshot(taskID)})
	case errors.Is(err, errTaskNotFound):
		r.writeError(w, http.StatusNotFound, err.Error())
	default:
		r.writeJSON(w, http.StatusConflict, map[string]interface{}{"status": "not_started", "task_id": taskID, "error": err.Error()})
	}
}

// atddStageInstruction asks for readable Given/When/Then cases plus the same cases as data, in
// the columns of the team's ATDD sheet, so the UAT stage and guide can cover every case marked
// UAT or Sanity.
const atddStageInstruction = `Write acceptance test cases in Given/When/Then form that cover every acceptance criterion, including negative and edge cases.
Then emit the cases as JSON in a fenced block whose language is atdd-cases, one object per atomic case:

` + "```" + `atdd-cases
[{"id": "ABC-TC-001", "requirement": "US-1", "reference": "US-1 / FR-1", "priority": "P1", "test_level": "SIT",
  "title": "Plain-language statement of what the case proves", "platform": "WEB", "repository": "billing-frontend",
  "uat": true, "sanity": false,
  "preconditions": ["I am signed in to Subscription Backyard as a Super Admin."],
  "steps": ["1. Open …", "2. Click …"], "expected_results": ["1. …", "2. …"],
  "reference_acceptance_criteria": "US-1 [Scenario: …]\nGiven: …\nWhen: …\nThen: …"}]
` + "```" + `

Rules: priority P0 (money, permissions, data loss, regression guards), P1 (core happy path, primary validations) or P2;
test_level SIT (end-to-end journey) or SUT (one rule or one API contract); platform WEB for screen-driven cases, API for operation-level cases;
repository is the assigned repository the case runs against; uat true for every reviewer-facing UI case;
sanity true for the smoke / critical-path cases that prove the release is healthy.
Write steps and expected results in complete plain language a business tester can follow — name the screen, menu, button and field; no bare requirement codes.`

// uatStageInstruction asks for a readable UAT checklist plus a machine-readable plan that the
// orchestrator replays in a browser per application to capture a screenshot per step (see
// uat_handler.go). The ATDD cases the plan must cover are appended by uatStageContext.
const uatStageInstruction = `Prepare the User Acceptance Test (UAT) for business testers (finance, sales and billing operations) who will verify the release by hand in each application.
Read the frontend code in the repositories to find the real routes, menu names, field labels and button texts, so every step matches the screen exactly.

1. Write a short UAT checklist: objective, preconditions, test data, and the scenarios — sanity checks first, then UAT scenarios grouped by application.
2. Then emit the exact plan as JSON in a fenced block whose language is uat-plan:

` + "```" + `uat-plan
{"feature": "…", "objective": "What the release should do, in one or two sentences a tester understands",
 "preconditions": ["Feature flag … is enabled for the test user (engineer)"], "test_data": ["A test company with an active subscription …"],
 "scenarios": [{"id": "S1", "title": "…", "app": "subscription_backyard", "covers": ["ABC-TC-001"], "sanity": true,
   "executable_by": "Finance/Ops", "preconditions": ["…"], "acceptance_criterion": "Given … When … Then …",
   "expected_result": "The single observable fact that proves the scenario passed",
   "steps": [{"action": "goto", "target": "/path", "where": "Subscription Backyard › Proforma Invoices", "description": "Open the Proforma Invoice list", "expected": "The list of Proforma Invoices is shown"},
             {"action": "fill", "target": "label=Field label", "value": "…", "where": "Create Proforma Invoice form", "description": "…", "expected": "…"},
             {"action": "click", "target": "role=button[name=\"Save\"]", "where": "…", "description": "…", "expected": "…"}]}]}
` + "```" + `

Plan rules:
- Cover every ATDD case listed below: each id must appear in the covers list of at least one scenario. Use one scenario per case, or one scenario for cases that share the same setup and screens.
- app is one of the application ids listed below; a scenario runs in exactly one application.
- the first browser step of every scenario is a goto to the screen it starts on; never start with a click, fill or expect_text on a page that is not open yet.
- goto targets are real routes from the frontend router in the repository, written as the full address-bar path including the router's base path (e.g. /billing/proforma-invoices/${UAT_PI_ID}); never invent a route. Record ids come from ${UAT_…} placeholders named after the test record (list each in test_data), never from values an earlier api step would return.
- actions: goto (target = path inside that application, see above), click, fill, select, check, press (value = key), wait (target = selector, or value = milliseconds), expect_text (value = text that must appear), api (target = "METHOD /path", value = request body; for API-only sanity cases run by an engineer), manual (a check the browser cannot do, e.g. an email or a background job result).
- targets: prefer label=…, placeholder=…, testid=…, text=… or role=button[name="…"]; use CSS only as a last resort.
- every step has "where" (screen and menu path), a plain-language description of exactly what to do, and the expected result the tester should see. Never skip an intermediate action (sign in, navigation, opening the record, saving, refreshing, waiting for a background job).
- executable_by: "Finance/Ops" when every step is on a screen, "Engineer" when any step needs console, API, database or flag access, "Engineer + Finance/Ops" when an engineer prepares and a business user verifies.
- the test data every scenario needs (record ids, serials, payment UIDs, dates) is prepared by one script, not by hand. After the plan, emit a second fenced block whose language is "uat-seed <language>" (e.g. uat-seed ruby) containing a script an engineer runs once in the UAT environment:
  - first line is a comment "# run: <exact command>" for that repository (e.g. bundle exec rails runner tmp/uat_seed.rb);
  - it aborts when it detects a production environment (the way this repository detects it) before it loads or touches any data, and never contains credentials (read any it needs from ENV);
  - it is a dry run unless UAT_SEED_APPLY=1 is set: without it, it runs every lookup and query it will rely on (read-only), reports what it would create on stderr, and writes nothing — so a broken query fails before any record exists. Put both commands in the header: the dry run first, then UAT_SEED_APPLY=1;
  - it creates each record in exactly the state the scenario needs, using the repository's own models, services or factories, and is safe to re-run (reuse its own tagged records and re-apply each state change only when needed);
  - every record it creates is keyed by a data-set tag read from ENV UAT_SEED_TAG (default 01): the same tag reuses the set, a new tag creates a fresh set — never reset or delete records to reuse them. When a reused record is no longer in the state its scenario needs (a UAT run or the screenshot capture paid, voided or changed it), stop and ask for a new UAT_SEED_TAG;
  - it only creates or changes records it owns (a tagged external reference and a fake tenant); it checks ownership before find_or_create, and never uses delete_all, destroy_all, update_all or raw DELETE/UPDATE;
  - every call must match the code as it is: open the definition of each class, method, scope and keyword argument you use and follow its real signature and return value. In joined queries, qualify every raw SQL column with its table (apps.name, not name) and do not merge another model's scope into a join unless its SQL is table-qualified — MySQL rejects ambiguous columns. Wait for work the code does in background jobs instead of assuming it finished;
  - on failure it prints which records it already created, then exits non-zero;
  - it prints UAT_SEED_TAG=<tag> first, then exactly one NAME=value line per ${…} placeholder the plan uses (dates included), and nothing else on stdout.
- use obviously fake test data. Never include real customer data, credentials or tokens; for a login use ${UAT_USERNAME} and ${UAT_PASSWORD}.`
