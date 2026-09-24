package worker

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

func TestWorkerPoolConcurrency(t *testing.T) {
	const maxWorkers = 5
	const totalTasks = 20

	var completedCount int32

	pool := NewPool(maxWorkers, 50, func(ctx context.Context, task *types.Task) error {
		// Simulate task work
		time.Sleep(20 * time.Millisecond)
		atomic.AddInt32(&completedCount, 1)
		return nil
	})
	defer pool.Stop(2 * time.Second)

	// Submit 20 tasks
	for i := 0; i < totalTasks; i++ {
		task := &types.Task{
			ID:    fmt.Sprintf("task-%d", i),
			Title: fmt.Sprintf("Job %d", i),
		}
		if err := pool.Submit(task); err != nil {
			t.Fatalf("failed to submit task %d: %v", i, err)
		}
	}

	// Wait for completion
	deadline := time.Now().Add(3 * time.Second)
	for atomic.LoadInt32(&completedCount) < int32(totalTasks) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}

	completed := atomic.LoadInt32(&completedCount)
	if completed != int32(totalTasks) {
		t.Errorf("expected %d completed tasks, got %d", totalTasks, completed)
	}

	peakWorkers := pool.MaxActiveSeen()
	if peakWorkers > maxWorkers {
		t.Errorf("peak active workers (%d) exceeded configured limit (%d)", peakWorkers, maxWorkers)
	}
	if peakWorkers == 0 {
		t.Errorf("expected positive peak worker count")
	}
}
