package telemetry

import (
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/fsm"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// RunMeta identifies a task run and the routing choices it was executed with.
type RunMeta struct {
	TaskID         string `json:"task_id"`
	Provider       string `json:"provider"`
	Model          string `json:"model"`
	Tier           string `json:"tier"`
	Method         string `json:"method"`
	Repo           string `json:"repo"`
	Category       string `json:"category"`
	Complexity     string `json:"complexity"`
	StageID        string `json:"stage_id,omitempty"` // set for single-stage (e.g. shadow) runs
	RouterStrategy string `json:"router_strategy"`    // "best_practice", "custom", ...
	RuleID         string `json:"rule_id,omitempty"`  // matched custom rule, if any
	Project        string `json:"project,omitempty"`
	Shadow         bool   `json:"shadow,omitempty"`
}

// RunRecord is the finished telemetry of one task run (one telemetry_runs row).
type RunRecord struct {
	RunMeta
	Success          bool             `json:"success"`
	FirstPass        bool             `json:"first_pass"`
	TestIterations   int              `json:"test_iterations"`
	FailureLoops     int              `json:"failure_loops"`
	FrustrationHalt  bool             `json:"frustration_halt"`
	DurationUS       int64            `json:"duration_us"`
	PhaseDurationsUS map[string]int64 `json:"phase_durations_us"`
	PeakCPUPercent   float64          `json:"peak_cpu_percent"`
	PeakMemoryMB     float64          `json:"peak_memory_mb"`
	ResourceSpikes   int              `json:"resource_spikes"`
	ToolCalls        int              `json:"tool_calls"`
	PromptTokens     int64            `json:"prompt_tokens"`
	CompletionTokens int64            `json:"completion_tokens"`
	CachedTokens     int64            `json:"cached_tokens"`
	CostUSD          float64          `json:"cost_usd"`
	StartedAt        time.Time        `json:"started_at"`
	FinishedAt       time.Time        `json:"finished_at"`
}

// TotalTokens is the run's token-per-feature figure.
func (r RunRecord) TotalTokens() int64 { return r.PromptTokens + r.CompletionTokens }

// TTRSeconds is the run's time-to-resolution in seconds.
func (r RunRecord) TTRSeconds() float64 { return float64(r.DurationUS) / 1e6 }

// ApplyToTask attaches the run's metrics to the task metadata.
func (r RunRecord) ApplyToTask(task *types.Task) {
	if task == nil {
		return
	}
	if task.Metadata == nil {
		task.Metadata = make(map[string]string)
	}
	phases, _ := json.Marshal(r.PhaseDurationsUS)
	task.Metadata["ttr_seconds"] = strconv.FormatFloat(r.TTRSeconds(), 'f', -1, 64)
	task.Metadata["phase_durations_us"] = string(phases)
	task.Metadata["peak_cpu_percent"] = fmt.Sprintf("%.2f", r.PeakCPUPercent)
	task.Metadata["peak_memory_mb"] = fmt.Sprintf("%.2f", r.PeakMemoryMB)
	task.Metadata["resource_spikes"] = strconv.Itoa(r.ResourceSpikes)
	task.Metadata["test_iterations"] = strconv.Itoa(r.TestIterations)
	task.Metadata["failure_loops"] = strconv.Itoa(r.FailureLoops)
	task.Metadata["first_pass"] = strconv.FormatBool(r.FirstPass)
}

type activeRun struct {
	rec        RunRecord
	stage      string
	stageStart time.Time
	firstTest  *bool
}

// RunSink persists finished runs (e.g. the telemetry datastore).
type RunSink interface {
	RecordRun(rec RunRecord) error
}

// TTRCollector measures time-to-resolution, per-phase latency, test cycles and resources.
type TTRCollector struct {
	mu     sync.Mutex
	runs   map[string]*activeRun
	sinks  []RunSink
	tokens func(taskID string) types.TokenUsage
	now    func() time.Time
}

// NewTTRCollector creates a collector.
func NewTTRCollector() *TTRCollector {
	return &TTRCollector{runs: make(map[string]*activeRun), now: time.Now}
}

// AddSink registers a persistence sink for finished runs.
func (c *TTRCollector) AddSink(s RunSink) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sinks = append(c.sinks, s)
}

// SetTokenSource lets Finish attach the run's accumulated token usage (e.g. TokenTracker.TaskUsage).
func (c *TTRCollector) SetTokenSource(src func(taskID string) types.TokenUsage) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tokens = src
}

// StartRun begins timing a task run. Restarting an active run resets it.
func (c *TTRCollector) StartRun(meta RunMeta) {
	now := c.now()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.runs[meta.TaskID] = &activeRun{
		rec:        RunRecord{RunMeta: meta, PhaseDurationsUS: make(map[string]int64), StartedAt: now},
		stage:      meta.StageID,
		stageStart: now,
	}
}

// TransitionHook returns an fsm.StateTransitionHook that closes the current phase timer.
func (c *TTRCollector) TransitionHook() fsm.StateTransitionHook {
	return func(task *types.Task, fromStage string, toStage string) error {
		if task == nil {
			return nil
		}
		c.Transition(task.ID, fromStage, toStage)
		return nil
	}
}

// Transition records the end of fromStage and the start of toStage.
func (c *TTRCollector) Transition(taskID, fromStage, toStage string) {
	now := c.now()
	c.mu.Lock()
	defer c.mu.Unlock()
	r, ok := c.runs[taskID]
	if !ok {
		return
	}
	stage := r.stage
	if stage == "" {
		stage = fromStage
	}
	if stage != "" {
		r.rec.PhaseDurationsUS[stage] += now.Sub(r.stageStart).Microseconds()
	}
	r.stage = toStage
	r.stageStart = now
}

// RecordTestIteration counts one test execution cycle.
func (c *TTRCollector) RecordTestIteration(taskID string, passed bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	r, ok := c.runs[taskID]
	if !ok {
		return
	}
	r.rec.TestIterations++
	if !passed {
		r.rec.FailureLoops++
	}
	if r.firstTest == nil {
		p := passed
		r.firstTest = &p
	}
}

// RecordToolCall counts one agent tool invocation (tool-call efficiency).
func (c *TTRCollector) RecordToolCall(taskID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if r, ok := c.runs[taskID]; ok {
		r.rec.ToolCalls++
	}
}

// MarkFrustrationHalt flags that the circuit breaker tripped during the run.
func (c *TTRCollector) MarkFrustrationHalt(taskID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if r, ok := c.runs[taskID]; ok {
		r.rec.FrustrationHalt = true
	}
}

// AttachResources merges a container resource summary into the run (peaks are max-merged).
func (c *TTRCollector) AttachResources(taskID string, s ResourceSummary) {
	c.mu.Lock()
	defer c.mu.Unlock()
	r, ok := c.runs[taskID]
	if !ok {
		return
	}
	if s.PeakCPUPercent > r.rec.PeakCPUPercent {
		r.rec.PeakCPUPercent = s.PeakCPUPercent
	}
	if s.PeakMemoryMB > r.rec.PeakMemoryMB {
		r.rec.PeakMemoryMB = s.PeakMemoryMB
	}
	r.rec.ResourceSpikes += len(s.Spikes)
}

// Finish closes the run and emits it to sinks. ok is false if no run was active.
func (c *TTRCollector) Finish(taskID string, success bool) (RunRecord, bool) {
	now := c.now()
	c.mu.Lock()
	r, ok := c.runs[taskID]
	if !ok {
		c.mu.Unlock()
		return RunRecord{}, false
	}
	delete(c.runs, taskID)
	if r.stage != "" {
		r.rec.PhaseDurationsUS[r.stage] += now.Sub(r.stageStart).Microseconds()
	}
	rec := r.rec
	rec.Success = success
	rec.FinishedAt = now
	rec.DurationUS = now.Sub(rec.StartedAt).Microseconds()
	rec.FirstPass = success && !rec.FrustrationHalt && (r.firstTest == nil || *r.firstTest)
	if c.tokens != nil {
		u := c.tokens(taskID)
		rec.PromptTokens, rec.CompletionTokens, rec.CachedTokens, rec.CostUSD =
			u.PromptTokens, u.CompletionTokens, u.CachedTokens, u.EstimatedCostUSD
	}
	sinks := append([]RunSink(nil), c.sinks...)
	c.mu.Unlock()

	for _, s := range sinks {
		_ = s.RecordRun(rec)
	}
	return rec, true
}
