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
	"prd_discovery":       "Clarify the requirement: goals, scope and non-goals, acceptance criteria, assumptions and open questions.",
	"repo_discovery":      "Identify the repositories, modules and files this task touches and why.",
	"atdd_creation":       "Write acceptance test cases in Given/When/Then form that cover every acceptance criterion, including edge cases.",
	"techdoc_rfc":         "Write a concise technical design: approach, components, data/API changes, risks and alternatives considered.",
	"task_breakdown":      "Break the work into small ordered sub-tasks, each with the repository, files and how it will be verified.",
	"red_verification":    "Explain which acceptance tests are expected to fail before implementation and why.",
	"task_implementation": "Implement the change. Show concrete code changes per file (unified diff where possible) and how to verify them.",
	"e2e_validation":      "Describe the end-to-end validation: scenarios, commands or scripts to run, and expected results.",
	"uat_verification":    "Prepare the UAT checklist and the evidence the reviewer should see.",
	"signoff_merge":       "Prepare the sign-off: summary of changes, risks, rollout notes and a pull request description.",
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
)

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
	feedback string
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
	ctx, cancel := context.WithTimeout(context.Background(), stageTurnTimeout)
	if err := r.beginTurn(taskID, turnID, cancel); err != nil {
		r.mu.Unlock()
		cancel()
		return nil, errTaskBusy
	}
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
	return &stageRun{task: snap, turnID: turnID, ctx: ctx, cancel: cancel, trigger: trigger, feedback: feedback}, nil
}

const stageTurnTimeout = 15 * time.Minute

// startStage claims the task's current stage and executes it in the background.
func (r *Router) startStage(taskID, trigger, feedback string) error {
	run, err := r.claimStage(taskID, trigger, feedback)
	if err != nil {
		return err
	}
	go r.executeStage(run)
	return nil
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
	decision := r.strategyRouter.RouteForTask(task.CurrentStageID, complexity, taskType, task.AssignedRepos)

	r.mu.Lock()
	if t, ok := r.tasks[taskID]; ok {
		if t.Metadata == nil {
			t.Metadata = map[string]string{}
		}
		t.Metadata["router_strategy"] = decision.Strategy
		t.Metadata["active_model"] = decision.Model
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
	r.console.mu.Unlock()

	msgs := append([]llm.Message{{Role: llm.RoleSystem, Content: taskSystemPrompt(task, decision.Method)}}, history...)
	msgs = append(msgs, llm.Message{Role: llm.RoleUser, Content: stagePrompt(task, run.feedback)})

	workDir := r.resolveWorkDir(task)
	r.mu.Lock()
	r.getOrCreateTaskProcessLocked(taskID).WorkingDir = workDir
	r.mu.Unlock()
	resp, used, execErr := r.runAgentTurn(run.ctx, agentTurn{Source: "execute", TurnID: run.turnID, Task: task, Decision: decision,
		TaskType: taskType, Messages: msgs, SessionID: sessionID, WorkDir: workDir, MaxTokens: 8192})
	r.endTurn(taskID, run.turnID, resp, used)

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
		note = fmt.Sprintf("⏸ Paused during %s. Resume to run this stage again.", stage)
	case execErr != nil:
		r.setTaskState(task, types.TaskStateFailed)
		task.Metadata["last_error"] = execErr.Error()
		proc.Status = "FAILED"
		proc.CurrentStep = "Failed: " + stage
		note = fmt.Sprintf("✗ %s failed: %v\nRun the stage again to retry, or ask the agent about the failure below.", stage, execErr)
	default:
		r.setTaskState(task, types.TaskStateWaitingGateApproval)
		task.Metadata["active_model"] = model
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
	case errors.Is(err, errTaskNotFound):
		r.writeError(w, http.StatusNotFound, err.Error())
	default:
		r.writeJSON(w, http.StatusConflict, map[string]interface{}{"status": "not_started", "task_id": taskID, "error": err.Error()})
	}
}
