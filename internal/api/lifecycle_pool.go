package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/worker"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

const (
	defaultMaxConcurrentStages = 10
	defaultStageQueueSize      = 100
	// poolStopTimeout bounds how long Close waits for interrupted stages to settle.
	poolStopTimeout = 10 * time.Second
)

// errStageCapacity means the stage could not be queued: every slot is busy and the queue is full,
// or the orchestrator is shutting down.
var errStageCapacity = errors.New("no stage slot is free")

func newStagePool(cfg RouterConfig) *worker.Pool {
	size, queue := cfg.MaxConcurrentStages, cfg.StageQueueSize
	if size <= 0 {
		size = defaultMaxConcurrentStages
	}
	if queue <= 0 {
		queue = defaultStageQueueSize
	}
	return worker.NewPool(size, queue)
}

// dispatchStage hands a claimed stage to the bounded stage pool. When the pool cannot take it, the
// claim is released so the task is not left RUNNING with no agent.
func (r *Router) dispatchStage(run *stageRun) error {
	busy := r.stages.ActiveWorkers() >= r.stages.Size()
	err := r.stages.Submit(func(ctx context.Context) error {
		stop := context.AfterFunc(ctx, run.cancel) // shutdown interrupts the turn like a pause does
		defer stop()
		if err := r.runPreStageHooks(run); err != nil {
			r.endTurn(run.task.ID, run.turnID, nil, "")
			r.settleStage(run.task.ID, run.task.CurrentStageID, nil, "", err, false)
			run.cancel()
			return nil
		}
		r.executeStage(run)
		return nil
	}, func(err error) {
		switch {
		case errors.Is(err, worker.ErrStopped):
			r.releaseClaim(run, "the orchestrator is shutting down")
		case err != nil:
			r.failStageRun(run, err)
		}
	})
	if err != nil {
		reason := fmt.Sprintf("all %d stage slots are busy and the queue is full", r.stages.Size())
		if errors.Is(err, worker.ErrStopped) {
			reason = "the orchestrator is shutting down"
		}
		r.releaseClaim(run, reason)
		return fmt.Errorf("%w: %s — try again shortly", errStageCapacity, reason)
	}
	if busy {
		r.addEntry(run.task.ID, types.ConsoleEntry{Kind: types.ConsoleKindSystem, TurnID: run.turnID, Content: fmt.Sprintf(
			"All %d stage slots are busy: %s is queued and starts when one frees (MO_MAX_CONCURRENT_STAGES sets the limit).",
			r.stages.Size(), run.task.CurrentStageID)})
	}
	return nil
}

// releaseClaim undoes claimStage for a stage that never ran: the task returns to the state it was
// claimed from and the console is free again.
func (r *Router) releaseClaim(run *stageRun, reason string) {
	taskID := run.task.ID
	r.mu.Lock()
	var snap *types.Task
	if task, ok := r.tasks[taskID]; ok && task.State == types.TaskStateRunning && task.CurrentStageID == run.task.CurrentStageID {
		r.setTaskState(task, run.prevState)
		if run.lastErr != "" {
			task.Metadata["last_error"] = run.lastErr
		}
		task.UpdatedAt = time.Now()
		proc := r.getOrCreateTaskProcessLocked(taskID)
		proc.Status = "IDLE"
		proc.CurrentStep = "Not running"
		snap = cloneTask(task)
	}
	r.mu.Unlock()
	r.endTurn(taskID, run.turnID, nil, "")
	run.cancel()
	r.addEntry(taskID, types.ConsoleEntry{Kind: types.ConsoleKindError, TurnID: run.turnID, Content: fmt.Sprintf(
		"%s did not start: %s. Nothing ran; run the stage again when a slot is free.", run.task.CurrentStageID, reason)})
	r.broadcastTask(snap)
}

// failStageRun settles a stage whose job ended in an error outside the agent turn (a recovered
// panic). It never overrides a stage that already settled or a newer turn.
func (r *Router) failStageRun(run *stageRun, err error) {
	taskID := run.task.ID
	r.console.mu.Lock()
	c := r.console.get(taskID)
	otherTurn := c.busy && c.turnID != run.turnID
	r.console.mu.Unlock()
	r.mu.RLock()
	task := r.tasks[taskID]
	unsettled := task != nil && task.State == types.TaskStateRunning && task.CurrentStageID == run.task.CurrentStageID
	r.mu.RUnlock()
	if otherTurn || !unsettled {
		r.addEntry(taskID, types.ConsoleEntry{Kind: types.ConsoleKindError, Content: fmt.Sprintf(
			"Internal error after %s settled: %v", run.task.CurrentStageID, err)})
		return
	}
	r.endTurn(taskID, run.turnID, nil, "")
	r.settleStage(taskID, run.task.CurrentStageID, nil, "", fmt.Errorf("internal error: %w", err), false)
}

// stageStartStatus is the HTTP status for a stage that could not start.
func stageStartStatus(err error) int {
	if errors.Is(err, errStageCapacity) {
		return http.StatusServiceUnavailable
	}
	return http.StatusConflict
}
