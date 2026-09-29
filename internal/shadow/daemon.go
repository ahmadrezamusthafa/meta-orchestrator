package shadow

import (
	"context"
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/telemetry"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// Job is one queued shadow benchmark of a completed production task.
type Job struct {
	ID         string              `json:"id"`
	Task       types.Task          `json:"task"` // deep-enough copy taken at completion time
	Baseline   telemetry.RunRecord `json:"baseline"`
	EnqueuedAt time.Time           `json:"enqueued_at"`
}

// JobRunner executes a shadow job (capture replay → sandbox → matrix sweep).
type JobRunner func(ctx context.Context, job Job) error

// LoadProbe reports host CPU utilisation as a percentage of total capacity.
type LoadProbe func(ctx context.Context) (float64, error)

// DaemonConfig tunes the shadow daemon.
type DaemonConfig struct {
	Workers           int           `json:"workers"`
	QueueSize         int           `json:"queue_size"`
	CPUCeilingPercent float64       `json:"cpu_ceiling_percent"` // do not launch while host CPU is above this
	ThrottleInterval  time.Duration `json:"throttle_interval"`
	JobTimeout        time.Duration `json:"job_timeout"`
}

// DaemonStats counts daemon activity.
type DaemonStats struct {
	Queued    int `json:"queued"`
	Dropped   int `json:"dropped"`
	Throttled int `json:"throttled"`
	Running   int `json:"running"`
	Completed int `json:"completed"`
	Failed    int `json:"failed"`
}

// Daemon listens for task completions and runs shadow jobs on its own bounded queue,
// fully separate from the production worker pool. Enqueueing never blocks the caller.
type Daemon struct {
	cfg    DaemonConfig
	probe  LoadProbe
	runJob JobRunner
	queue  chan Job

	mu     sync.Mutex
	seen   map[string]bool
	stats  DaemonStats
	cancel context.CancelFunc
	wg     sync.WaitGroup
	now    func() time.Time
}

// NewDaemon creates a daemon. probe may be nil (HostLoadProbe is used).
func NewDaemon(cfg DaemonConfig, probe LoadProbe, runner JobRunner) *Daemon {
	if cfg.Workers <= 0 {
		cfg.Workers = 1
	}
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = 64
	}
	if cfg.CPUCeilingPercent <= 0 {
		cfg.CPUCeilingPercent = 70
	}
	if cfg.ThrottleInterval <= 0 {
		cfg.ThrottleInterval = 5 * time.Second
	}
	if cfg.JobTimeout <= 0 {
		cfg.JobTimeout = 30 * time.Minute
	}
	if probe == nil {
		probe = HostLoadProbe(nil)
	}
	return &Daemon{cfg: cfg, probe: probe, runJob: runner, queue: make(chan Job, cfg.QueueSize),
		seen: map[string]bool{}, now: time.Now}
}

// OnTaskCompleted enqueues a shadow job for a completed production task. It returns false
// when the task is not eligible, already queued, or the queue is full (the job is dropped).
func (d *Daemon) OnTaskCompleted(task *types.Task, baseline telemetry.RunRecord) bool {
	if task == nil || task.State != types.TaskStateCompleted {
		return false
	}
	if task.Metadata["shadow"] == "true" || baseline.Shadow {
		return false
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.seen[task.ID] {
		return false
	}
	cp := *task
	cp.Metadata = make(map[string]string, len(task.Metadata))
	for k, v := range task.Metadata {
		cp.Metadata[k] = v
	}
	cp.AssignedRepos = append([]string(nil), task.AssignedRepos...)
	job := Job{ID: fmt.Sprintf("shadow-%s-%d", task.ID, d.now().UnixNano()), Task: cp, Baseline: baseline, EnqueuedAt: d.now()}
	select {
	case d.queue <- job:
		d.seen[task.ID] = true
		d.stats.Queued++
		return true
	default:
		d.stats.Dropped++
		return false
	}
}

// Start launches the worker goroutines.
func (d *Daemon) Start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	d.mu.Lock()
	d.cancel = cancel
	d.mu.Unlock()
	for i := 0; i < d.cfg.Workers; i++ {
		d.wg.Add(1)
		go d.worker(ctx)
	}
}

// Stop cancels running jobs and waits for workers to exit.
func (d *Daemon) Stop() {
	d.mu.Lock()
	cancel := d.cancel
	d.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	d.wg.Wait()
}

// Stats returns a snapshot of daemon counters.
func (d *Daemon) Stats() DaemonStats {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.stats
}

func (d *Daemon) worker(ctx context.Context) {
	defer d.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-d.queue:
			if !d.waitForHeadroom(ctx) {
				return
			}
			d.execute(ctx, job)
		}
	}
}

// waitForHeadroom blocks until host CPU is under the ceiling. Returns false on shutdown.
func (d *Daemon) waitForHeadroom(ctx context.Context) bool {
	for {
		load, err := d.probe(ctx)
		if err == nil && load <= d.cfg.CPUCeilingPercent {
			return true
		}
		d.mu.Lock()
		d.stats.Throttled++
		d.mu.Unlock()
		select {
		case <-ctx.Done():
			return false
		case <-time.After(d.cfg.ThrottleInterval):
		}
	}
}

func (d *Daemon) execute(ctx context.Context, job Job) {
	d.mu.Lock()
	d.stats.Running++
	d.mu.Unlock()

	jobCtx, cancel := context.WithTimeout(ctx, d.cfg.JobTimeout)
	err := d.runJob(jobCtx, job)
	cancel()

	d.mu.Lock()
	d.stats.Running--
	delete(d.seen, job.Task.ID)
	if err != nil {
		d.stats.Failed++
	} else {
		d.stats.Completed++
	}
	d.mu.Unlock()
}

// HostLoadProbe approximates host CPU utilisation from the 1-minute load average divided by
// the CPU count (Linux /proc/loadavg, macOS sysctl vm.loadavg).
func HostLoadProbe(runner telemetry.CommandRunner) LoadProbe {
	if runner == nil {
		runner = telemetry.ExecRunner
	}
	return func(ctx context.Context) (float64, error) {
		var out []byte
		var err error
		if runtime.GOOS == "darwin" {
			out, err = runner(ctx, "sysctl", "-n", "vm.loadavg")
		} else {
			out, err = runner(ctx, "cat", "/proc/loadavg")
		}
		if err != nil {
			return 0, err
		}
		return parseLoadAvg(string(out), runtime.NumCPU())
	}
}

func parseLoadAvg(s string, cpus int) (float64, error) {
	fields := strings.Fields(strings.NewReplacer("{", " ", "}", " ").Replace(s))
	if len(fields) == 0 {
		return 0, fmt.Errorf("empty load average")
	}
	one, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, fmt.Errorf("invalid load average %q: %w", s, err)
	}
	if cpus <= 0 {
		cpus = 1
	}
	return one / float64(cpus) * 100, nil
}
