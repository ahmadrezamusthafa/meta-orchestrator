package worker

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestWorkerPoolConcurrency(t *testing.T) {
	const maxWorkers = 5
	const totalJobs = 20

	var completed int32
	pool := NewPool(maxWorkers, 50)
	defer pool.Stop(2 * time.Second)

	for i := 0; i < totalJobs; i++ {
		err := pool.Submit(func(ctx context.Context) error {
			time.Sleep(20 * time.Millisecond)
			return nil
		}, func(err error) { atomic.AddInt32(&completed, 1) })
		if err != nil {
			t.Fatalf("failed to submit job %d: %v", i, err)
		}
	}

	deadline := time.Now().Add(3 * time.Second)
	for atomic.LoadInt32(&completed) < totalJobs && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := atomic.LoadInt32(&completed); got != totalJobs {
		t.Errorf("expected %d completed jobs, got %d", totalJobs, got)
	}
	if peak := pool.MaxActiveSeen(); peak > maxWorkers || peak == 0 {
		t.Errorf("peak active workers %d outside (0, %d]", peak, maxWorkers)
	}
}

func TestPanickingJobFailsWithoutCrashingThePool(t *testing.T) {
	pool := NewPool(1, 2)
	defer pool.Stop(time.Second)
	errs := make(chan error, 2)
	if err := pool.Submit(func(context.Context) error { panic("boom") }, func(err error) { errs <- err }); err != nil {
		t.Fatal(err)
	}
	if err := pool.Submit(func(context.Context) error { return nil }, func(err error) { errs <- err }); err != nil {
		t.Fatal(err)
	}
	var pe *PanicError
	if err := <-errs; !errors.As(err, &pe) || pe.Value != "boom" {
		t.Fatalf("panic should surface as *PanicError, got %v", err)
	}
	if err := <-errs; err != nil {
		t.Fatalf("the worker must keep serving after a panic, got %v", err)
	}
}

func TestSubmitRefusesWhenQueueIsFull(t *testing.T) {
	pool := NewPool(1, 0)
	defer pool.Stop(time.Second)
	release := make(chan struct{})
	started := make(chan struct{})
	// With no queue room a job is accepted only by an idle worker; wait until one is listening.
	deadline := time.Now().Add(2 * time.Second)
	for pool.Submit(func(context.Context) error { close(started); <-release; return nil }, nil) != nil {
		if time.Now().After(deadline) {
			t.Fatal("an idle worker never accepted the job")
		}
		time.Sleep(time.Millisecond)
	}
	<-started
	if err := pool.Submit(func(context.Context) error { return nil }, nil); !errors.Is(err, ErrQueueFull) {
		t.Fatalf("a busy pool with no queue room must refuse, got %v", err)
	}
	close(release)
}

func TestStopHonoursTimeoutAndCancelsJobs(t *testing.T) {
	pool := NewPool(2, 4)
	cancelled := make(chan struct{})
	_ = pool.Submit(func(ctx context.Context) error { <-ctx.Done(); close(cancelled); return ctx.Err() }, nil)
	stuck := make(chan struct{})
	defer close(stuck)
	_ = pool.Submit(func(ctx context.Context) error { <-stuck; return nil }, nil) // ignores cancellation
	time.Sleep(20 * time.Millisecond)

	start := time.Now()
	if pool.Stop(100 * time.Millisecond) {
		t.Fatal("Stop must report that a job did not finish in time")
	}
	if waited := time.Since(start); waited > time.Second {
		t.Fatalf("Stop ignored its timeout: waited %v", waited)
	}
	select {
	case <-cancelled:
	default:
		t.Fatal("Stop must cancel running jobs' context")
	}
	if err := pool.Submit(func(context.Context) error { return nil }, nil); !errors.Is(err, ErrStopped) {
		t.Fatalf("a stopped pool must refuse jobs, got %v", err)
	}
}

func TestQueuedJobsFailWithErrStoppedOnStop(t *testing.T) {
	pool := NewPool(1, 2)
	release := make(chan struct{})
	started := make(chan struct{})
	_ = pool.Submit(func(ctx context.Context) error { close(started); <-release; return nil }, nil)
	<-started
	got := make(chan error, 1)
	_ = pool.Submit(func(context.Context) error { t.Error("queued job must not run after Stop"); return nil }, func(err error) { got <- err })
	go func() { time.Sleep(20 * time.Millisecond); close(release) }()
	pool.Stop(time.Second)
	if err := <-got; !errors.Is(err, ErrStopped) {
		t.Fatalf("queued job should be told the pool stopped, got %v", err)
	}
}
