package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/fsm"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/hooks"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// hookWaitOnClose bounds how long Close waits for background hooks after cancelling them.
const hookWaitOnClose = 5 * time.Second

// initHooks loads the operator's declarative hooks (.sdlc/hooks.json) into the engine. Convention
// scripts (.sdlc/hooks/<event>.sh) are found by the engine itself.
func (r *Router) initHooks() {
	r.hookCtx, r.hookCancel = context.WithCancel(context.Background())
	if r.cfg.HookEngine == nil {
		r.cfg.HookEngine = hooks.NewHookEngine(r.cfg.RootDir)
	}
	if r.cfg.RootDir == "" {
		return
	}
	path := filepath.Join(r.cfg.RootDir, ".sdlc", "hooks.json")
	defs, err := hooks.LoadDefinitions(path)
	if err != nil {
		fmt.Printf("[Hooks] Ignoring %s: %v\n", path, err)
		return
	}
	for _, d := range defs {
		r.cfg.HookEngine.RegisterHook(d)
	}
	if len(defs) > 0 {
		fmt.Printf("[Hooks] Loaded %d lifecycle hooks from %s\n", len(defs), path)
	}
}

// fireHooks runs the hooks for event against the task's workspace. Non-blocking failures go to the
// task console; a blocking failure is returned. It never holds Router locks while hooks run.
func (r *Router) fireHooks(ctx context.Context, event hooks.HookEvent, task *types.Task, stage, workDir string, meta map[string]interface{}) error {
	engine := r.cfg.HookEngine
	if engine == nil || task == nil || !engine.HasHooks(event) {
		return nil
	}
	if workDir == "" {
		workDir = r.resolveWorkDir(task)
	}
	warnings, err := engine.TriggerEvent(ctx, event, &hooks.HookExecutionContext{
		TaskID: task.ID, StageID: stage, WorkspacePath: workDir, Metadata: meta})
	for _, w := range warnings {
		r.addEntry(task.ID, types.ConsoleEntry{Kind: types.ConsoleKindError, Content: fmt.Sprintf("⚠ %s hook: %v", event, w)})
	}
	return err
}

// fireHooksAsync runs hooks in the background, reporting even blocking failures to the console.
func (r *Router) fireHooksAsync(event hooks.HookEvent, task *types.Task, stage, workDir string, meta map[string]interface{}, onBlocking func(error)) {
	if r.cfg.HookEngine == nil || task == nil || !r.cfg.HookEngine.HasHooks(event) {
		return
	}
	r.hookWG.Add(1)
	go func() {
		defer r.hookWG.Done()
		if err := r.fireHooks(r.hookCtx, event, task, stage, workDir, meta); err != nil {
			if onBlocking != nil {
				onBlocking(err)
				return
			}
			r.addEntry(task.ID, types.ConsoleEntry{Kind: types.ConsoleKindError, Content: fmt.Sprintf("✗ %s hook: %v", event, err)})
		}
	}()
}

// runPreStageHooks runs before the agent turn, inside the stage's pool job. A blocking failure
// fails the stage.
func (r *Router) runPreStageHooks(run *stageRun) error {
	if r.cfg.HookEngine == nil || !r.cfg.HookEngine.HasHooks(hooks.HookPreStage) {
		return nil
	}
	r.addEntry(run.task.ID, types.ConsoleEntry{Kind: types.ConsoleKindSystem, TurnID: run.turnID, Content: "Running pre-stage hooks for " + run.task.CurrentStageID + "…"})
	return r.fireHooks(run.ctx, hooks.HookPreStage, run.task, run.task.CurrentStageID, "", map[string]interface{}{"trigger": run.trigger})
}

// afterStageSettled fires the hooks that follow a settled stage: on-failure for a failed stage,
// post-stage then on-gate for one awaiting review. A blocking post-stage failure fails the stage
// (if it is still in review); a paused stage fires nothing.
func (r *Router) afterStageSettled(taskID, stage string, execErr error) {
	if r.cfg.HookEngine == nil || errors.Is(execErr, context.Canceled) {
		return
	}
	task := r.taskSnapshot(taskID)
	if task == nil {
		return
	}
	if execErr != nil {
		r.fireHooksAsync(hooks.HookOnFailure, task, stage, "", map[string]interface{}{"error": execErr.Error()}, nil)
		return
	}
	engine := r.cfg.HookEngine
	if !engine.HasHooks(hooks.HookPostStage) && !engine.HasHooks(hooks.HookOnGate) {
		return
	}
	r.hookWG.Add(1)
	go func() {
		defer r.hookWG.Done()
		workDir := r.resolveWorkDir(task)
		if err := r.fireHooks(r.hookCtx, hooks.HookPostStage, task, stage, workDir, nil); err != nil {
			r.failReviewedStage(taskID, stage, err)
			return
		}
		if err := r.fireHooks(r.hookCtx, hooks.HookOnGate, task, stage, workDir, map[string]interface{}{"gate": "awaiting_approval"}); err != nil {
			r.addEntry(taskID, types.ConsoleEntry{Kind: types.ConsoleKindError, Content: fmt.Sprintf("✗ on-gate hook: %v", err)})
		}
	}()
}

// failReviewedStage moves a stage awaiting review to FAILED after a blocking post-stage hook failed.
func (r *Router) failReviewedStage(taskID, stage string, hookErr error) {
	r.mu.Lock()
	task, ok := r.tasks[taskID]
	if !ok || task.CurrentStageID != stage || task.State != types.TaskStateWaitingGateApproval ||
		!fsm.CanTransition(task.State, types.TaskStateFailed) {
		r.mu.Unlock()
		r.addEntry(taskID, types.ConsoleEntry{Kind: types.ConsoleKindError, Content: fmt.Sprintf(
			"✗ post-stage hook failed after %s left review: %v", stage, hookErr)})
		return
	}
	r.setTaskState(task, types.TaskStateFailed)
	task.Metadata["last_error"] = hookErr.Error()
	task.UpdatedAt = time.Now()
	proc := r.getOrCreateTaskProcessLocked(taskID)
	proc.Status = "FAILED"
	proc.CurrentStep = "Failed: " + stage
	snap := cloneTask(task)
	r.mu.Unlock()
	r.addEntry(taskID, types.ConsoleEntry{Kind: types.ConsoleKindError, Content: fmt.Sprintf(
		"✗ %s failed its post-stage check: %v\nFix the cause and Continue to re-run the stage.", stage, hookErr)})
	r.broadcastTask(snap)
	r.fireHooksAsync(hooks.HookOnFailure, snap, stage, "", map[string]interface{}{"error": hookErr.Error()}, nil)
}

// gateDecided fires on-gate for an operator's approve/reject decision.
func (r *Router) gateDecided(task *types.Task, stage string, approved bool) {
	decision := "rejected"
	if approved {
		decision = "approved"
	}
	r.fireHooksAsync(hooks.HookOnGate, task, stage, "", map[string]interface{}{"gate": decision}, nil)
}

// preCommitHooks runs before the pull request step commits pending edits in dir; a blocking
// failure stops the commit.
func (r *Router) preCommitHooks(ctx context.Context, task *types.Task, dir string) error {
	if err := r.fireHooks(ctx, hooks.HookPreCommit, task, task.CurrentStageID, dir, nil); err != nil {
		return fmt.Errorf("pre-commit hook stopped the commit: %w", err)
	}
	return nil
}

// postCommitHooks runs after the pull request step committed pending edits in dir.
func (r *Router) postCommitHooks(task *types.Task, dir string) {
	r.fireHooksAsync(hooks.HookPostCommit, task, task.CurrentStageID, dir, nil, nil)
}

// stopHooks cancels running background hooks and waits (bounded) for them to return.
func (r *Router) stopHooks() {
	if r.hookCancel != nil {
		r.hookCancel()
	}
	done := make(chan struct{})
	go func() { r.hookWG.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(hookWaitOnClose):
	}
}

// handleHooks lists the configured lifecycle hooks (GET /api/v1/hooks).
func (r *Router) handleHooks(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		r.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	list := []hooks.HookDefinition{}
	if r.cfg.HookEngine != nil {
		for _, h := range r.cfg.HookEngine.List() {
			list = append(list, h.Redacted())
		}
	}
	r.writeJSON(w, http.StatusOK, map[string]interface{}{"hooks": list, "events": hooks.Events})
}
