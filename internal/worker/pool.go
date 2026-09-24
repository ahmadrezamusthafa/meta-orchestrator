package worker

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// TaskHandler represents the function executing a single orchestrated task.
type TaskHandler func(ctx context.Context, task *types.Task) error

// Pool manages a bounded concurrent worker pool executing asynchronous tasks.
type Pool struct {
	maxWorkers     int
	taskQueue      chan *types.Task
	handler        TaskHandler
	activeWorkers  int32
	maxActiveSeen  int32
	wg             sync.WaitGroup
	ctx            context.Context
	cancel         context.CancelFunc
	shutdownOnce   sync.Once
}

// NewPool creates a worker pool bounded by maxWorkers.
func NewPool(maxWorkers int, queueCapacity int, handler TaskHandler) *Pool {
	if maxWorkers <= 0 {
		maxWorkers = 10
	}
	if queueCapacity <= 0 {
		queueCapacity = 100
	}

	ctx, cancel := context.WithCancel(context.Background())

	p := &Pool{
		maxWorkers:    maxWorkers,
		taskQueue:     make(chan *types.Task, queueCapacity),
		handler:       handler,
		ctx:           ctx,
		cancel:        cancel,
	}

	p.start()
	return p
}

func (p *Pool) start() {
	for i := 0; i < p.maxWorkers; i++ {
		p.wg.Add(1)
		go p.workerLoop(i)
	}
}

func (p *Pool) workerLoop(workerID int) {
	defer p.wg.Done()

	for {
		select {
		case <-p.ctx.Done():
			return

		case task, ok := <-p.taskQueue:
			if !ok {
				return
			}

			currentActive := atomic.AddInt32(&p.activeWorkers, 1)
			// Track max concurrency peak seen
			for {
				maxVal := atomic.LoadInt32(&p.maxActiveSeen)
				if currentActive <= maxVal || atomic.CompareAndSwapInt32(&p.maxActiveSeen, maxVal, currentActive) {
					break
				}
			}

			if p.handler != nil && task != nil {
				_ = p.handler(p.ctx, task)
			}

			atomic.AddInt32(&p.activeWorkers, -1)
		}
	}
}

// Submit enqueues a task for execution in the pool.
func (p *Pool) Submit(task *types.Task) error {
	select {
	case <-p.ctx.Done():
		return fmt.Errorf("worker pool is stopped")
	case p.taskQueue <- task:
		return nil
	default:
		return fmt.Errorf("worker pool queue is full")
	}
}

// ActiveWorkers returns current active workers.
func (p *Pool) ActiveWorkers() int {
	return int(atomic.LoadInt32(&p.activeWorkers))
}

// MaxActiveSeen returns peak concurrent workers observed.
func (p *Pool) MaxActiveSeen() int {
	return int(atomic.LoadInt32(&p.maxActiveSeen))
}

// Stop gracefully waits for in-flight tasks to complete and stops workers.
func (p *Pool) Stop(timeout time.Duration) {
	p.shutdownOnce.Do(func() {
		close(p.taskQueue)
		p.cancel()
		p.wg.Wait()
	})
}
