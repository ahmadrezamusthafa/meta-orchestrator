package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// TaskExecutionWorker executes a task within isolated process context and reports telemetry.
type TaskExecutionWorker struct {
	id      int
	handler TaskHandler
}

// NewTaskExecutionWorker creates a worker with an assigned numeric identifier.
func NewTaskExecutionWorker(id int, handler TaskHandler) *TaskExecutionWorker {
	return &TaskExecutionWorker{
		id:      id,
		handler: handler,
	}
}

// Execute runs the handler with timeout protection and panic recovery.
func (w *TaskExecutionWorker) Execute(ctx context.Context, task *types.Task, timeout time.Duration) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("worker %d panicked during task %s: %v", w.id, task.ID, r)
		}
	}()

	if timeout <= 0 {
		timeout = 30 * time.Minute
	}
	taskCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if w.handler != nil {
		return w.handler(taskCtx, task)
	}

	return nil
}
