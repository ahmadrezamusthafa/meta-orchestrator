package worker

import (
	"context"
	"fmt"
	"runtime/debug"
	"time"
)

// TaskExecutionWorker runs pool jobs with panic recovery and an optional timeout.
type TaskExecutionWorker struct {
	id int
}

// NewTaskExecutionWorker creates a worker with an assigned numeric identifier.
func NewTaskExecutionWorker(id int) *TaskExecutionWorker {
	return &TaskExecutionWorker{id: id}
}

// PanicError is the error a job that panicked fails with.
type PanicError struct {
	WorkerID int
	Value    interface{}
	Stack    []byte
}

func (e *PanicError) Error() string {
	return fmt.Sprintf("worker %d recovered from a panic: %v", e.WorkerID, e.Value)
}

// Execute runs job, turning a panic into a *PanicError. A positive timeout bounds the job's context.
func (w *TaskExecutionWorker) Execute(ctx context.Context, job Job, timeout time.Duration) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = &PanicError{WorkerID: w.id, Value: r, Stack: debug.Stack()}
		}
	}()
	if job == nil {
		return nil
	}
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	return job(ctx)
}
