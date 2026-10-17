package worker

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// Job is one unit of work run by the pool. ctx is cancelled when the pool stops.
type Job func(ctx context.Context) error

var (
	// ErrQueueFull is returned by Submit when every worker is busy and the queue has no room.
	ErrQueueFull = errors.New("worker pool queue is full")
	// ErrStopped is returned by Submit after Stop, and passed to done for queued jobs that never ran.
	ErrStopped = errors.New("worker pool is stopped")
)

type queuedJob struct {
	job  Job
	done func(error)
}

// Pool runs jobs on a bounded number of workers. A panicking job fails with an error instead of
// crashing the process.
type Pool struct {
	maxWorkers    int
	queue         chan queuedJob
	activeWorkers int32
	maxActiveSeen int32
	wg            sync.WaitGroup
	ctx           context.Context
	cancel        context.CancelFunc
	mu            sync.RWMutex // guards stopped against Submit racing the queue close
	stopped       bool
}

// NewPool creates a pool of maxWorkers workers with room for queueCapacity waiting jobs.
func NewPool(maxWorkers int, queueCapacity int) *Pool {
	if maxWorkers <= 0 {
		maxWorkers = 10
	}
	if queueCapacity < 0 {
		queueCapacity = 0
	}
	ctx, cancel := context.WithCancel(context.Background())
	p := &Pool{
		maxWorkers: maxWorkers,
		queue:      make(chan queuedJob, queueCapacity),
		ctx:        ctx,
		cancel:     cancel,
	}
	for i := 0; i < p.maxWorkers; i++ {
		p.wg.Add(1)
		go p.workerLoop(NewTaskExecutionWorker(i))
	}
	return p
}

func (p *Pool) workerLoop(w *TaskExecutionWorker) {
	defer p.wg.Done()
	for item := range p.queue {
		if p.ctx.Err() != nil {
			if item.done != nil {
				item.done(ErrStopped)
			}
			continue
		}
		current := atomic.AddInt32(&p.activeWorkers, 1)
		for {
			peak := atomic.LoadInt32(&p.maxActiveSeen)
			if current <= peak || atomic.CompareAndSwapInt32(&p.maxActiveSeen, peak, current) {
				break
			}
		}
		err := w.Execute(p.ctx, item.job, 0)
		atomic.AddInt32(&p.activeWorkers, -1)
		if item.done != nil {
			item.done(err)
		}
	}
}

// Submit queues a job without blocking. done, if not nil, receives the job's error (a recovered
// panic included) after it runs, or ErrStopped if the pool stops before it starts.
func (p *Pool) Submit(job Job, done func(error)) error {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.stopped {
		return ErrStopped
	}
	select {
	case p.queue <- queuedJob{job: job, done: done}:
		return nil
	default:
		return ErrQueueFull
	}
}

// Size is the number of workers.
func (p *Pool) Size() int { return p.maxWorkers }

// ActiveWorkers returns current active workers.
func (p *Pool) ActiveWorkers() int {
	return int(atomic.LoadInt32(&p.activeWorkers))
}

// MaxActiveSeen returns peak concurrent workers observed.
func (p *Pool) MaxActiveSeen() int {
	return int(atomic.LoadInt32(&p.maxActiveSeen))
}

// Stop refuses new jobs, cancels the context of running ones, fails queued ones with ErrStopped
// and waits up to timeout for the workers to finish. It reports whether they all finished in time.
// Safe to call more than once.
func (p *Pool) Stop(timeout time.Duration) bool {
	p.mu.Lock()
	if !p.stopped {
		p.stopped = true
		p.cancel()
		close(p.queue)
	}
	p.mu.Unlock()

	finished := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(finished)
	}()
	select {
	case <-finished:
		return true
	case <-time.After(timeout):
		return false
	}
}
