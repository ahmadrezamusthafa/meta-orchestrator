package shadow

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/telemetry"
)

// StageDef is one SDLC stage of the benchmark matrix.
type StageDef struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Index int    `json:"index"`
}

// ComplexityDef is one complexity stratum.
type ComplexityDef struct {
	ID          string `json:"id"`
	Description string `json:"description"`
}

// MethodDef is one execution methodology.
type MethodDef struct {
	ID          string `json:"id"`
	Description string `json:"description"`
}

// ModelDef is one candidate model ("provider/model") and its tier.
type ModelDef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Tier string `json:"tier"`
}

// Fixture is a representative task (or a replayed real ticket) executed in each cell.
type Fixture struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Complexity  string   `json:"complexity"`
	Category    string   `json:"category"`
	Stages      []string `json:"stages,omitempty"` // empty = all stages
	Repo        string   `json:"repo,omitempty"`
	Expect      []string `json:"expect"` // acceptance keywords the output must contain
}

// ExecutionDef bounds the sweep.
type ExecutionDef struct {
	MaxParallelCells  int      `json:"max_parallel_cells"`
	Repetitions       int      `json:"repetitions"`
	MaxIterations     int      `json:"max_iterations"`
	CellTimeoutS      int      `json:"cell_timeout_s"`
	CPUCeilingPercent float64  `json:"cpu_ceiling_percent"`
	ReplayStages      []string `json:"replay_stages"` // stages swept when replaying a real ticket
}

// MatrixConfig is configs/shadow_matrix.json.
type MatrixConfig struct {
	Stages       []StageDef      `json:"stages"`
	Complexities []ComplexityDef `json:"complexities"`
	Methods      []MethodDef     `json:"methods"`
	Models       []ModelDef      `json:"models"`
	Fixtures     []Fixture       `json:"fixtures"`
	Execution    ExecutionDef    `json:"execution"`
}

// LoadMatrixConfig reads and validates a matrix configuration file.
func LoadMatrixConfig(path string) (MatrixConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return MatrixConfig{}, err
	}
	var cfg MatrixConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return MatrixConfig{}, fmt.Errorf("invalid shadow matrix %s: %w", path, err)
	}
	return cfg, cfg.Validate()
}

// Validate checks the configuration is internally consistent.
func (c MatrixConfig) Validate() error {
	if len(c.Stages) == 0 || len(c.Complexities) == 0 || len(c.Methods) == 0 || len(c.Models) == 0 {
		return fmt.Errorf("shadow matrix needs stages, complexities, methods and models")
	}
	cx := map[string]bool{}
	for _, d := range c.Complexities {
		cx[d.ID] = true
	}
	for _, f := range c.Fixtures {
		if f.ID == "" || !cx[f.Complexity] {
			return fmt.Errorf("fixture %q has unknown complexity %q", f.ID, f.Complexity)
		}
		if len(f.Expect) == 0 {
			return fmt.Errorf("fixture %q has no acceptance expectations", f.ID)
		}
	}
	return nil
}

// FixturesFor returns fixtures for a complexity, optionally restricted to a stage.
func (c MatrixConfig) FixturesFor(complexity, stage string) []Fixture {
	return filterFixtures(c.Fixtures, complexity, stage)
}

func filterFixtures(all []Fixture, complexity, stage string) []Fixture {
	var out []Fixture
	for _, f := range all {
		if !strings.EqualFold(f.Complexity, complexity) {
			continue
		}
		if stage != "" && len(f.Stages) > 0 && !containsFold(f.Stages, stage) {
			continue
		}
		out = append(out, f)
	}
	return out
}

func containsFold(list []string, s string) bool {
	for _, v := range list {
		if strings.EqualFold(v, s) {
			return true
		}
	}
	return false
}

// Cell is one permutation of the benchmark matrix.
type Cell struct {
	Stage      string `json:"stage_id"`
	Complexity string `json:"complexity"`
	Method     string `json:"method"`
	Model      string `json:"model"`
	Tier       string `json:"tier"`
}

// Key identifies the cell (tier is derived from the model).
func (c Cell) Key() string {
	return strings.Join([]string{c.Stage, c.Complexity, c.Method, c.Model}, "|")
}

// TraceStep is one LLM call inside a cell execution.
type TraceStep struct {
	Role             string `json:"role"`
	Attempt          int    `json:"attempt"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
	ToolCalls        int    `json:"tool_calls"`
	DurationUS       int64  `json:"duration_us"`
	Verified         *bool  `json:"verified,omitempty"`
	Note             string `json:"note,omitempty"`
}

// Observation is the outcome of executing one fixture in one cell.
type Observation struct {
	FirstPass        bool        `json:"first_pass"`
	Passed           bool        `json:"passed"`
	Iterations       int         `json:"iterations"`
	PromptTokens     int64       `json:"prompt_tokens"`
	CompletionTokens int64       `json:"completion_tokens"`
	CachedTokens     int64       `json:"cached_tokens"`
	CostUSD          float64     `json:"cost_usd"`
	DurationUS       int64       `json:"duration_us"`
	ToolCalls        int         `json:"tool_calls"`
	Steps            []TraceStep `json:"steps"`
}

// CellExecutor runs one fixture in one cell.
type CellExecutor interface {
	Execute(ctx context.Context, cell Cell, fx Fixture, runID string) (Observation, error)
}

// CellTrace is the recorded trace of one repetition.
type CellTrace struct {
	RunID      string      `json:"run_id"`
	FixtureID  string      `json:"fixture_id"`
	Repetition int         `json:"repetition"`
	FirstPass  bool        `json:"first_pass"`
	Passed     bool        `json:"passed"`
	Tokens     int64       `json:"tokens"`
	CostUSD    float64     `json:"cost_usd"`
	DurationUS int64       `json:"duration_us"`
	Steps      []TraceStep `json:"steps"`
	Error      string      `json:"error,omitempty"`
}

// CellResult aggregates the isolated metrics of one cell.
type CellResult struct {
	Cell          Cell        `json:"cell"`
	Samples       int         `json:"samples"`
	FirstPasses   int         `json:"first_passes"`
	FPVR          float64     `json:"fpvr"`      // first-pass verification rate, 0..1
	PassRate      float64     `json:"pass_rate"` // eventual pass rate within MaxIterations, 0..1
	AvgTokens     float64     `json:"avg_tokens"`
	AvgCostUSD    float64     `json:"avg_cost_usd"`
	AvgTTRSeconds float64     `json:"avg_ttr_seconds"`
	AvgToolCalls  float64     `json:"avg_tool_calls"`
	AvgIterations float64     `json:"avg_iterations"`
	Errors        int         `json:"errors"`
	LastError     string      `json:"last_error,omitempty"`
	Traces        []CellTrace `json:"traces"`
}

// Sweep selects the matrix subset to execute. Empty dimensions mean "all configured".
type Sweep struct {
	ID           string    `json:"id"`
	Stages       []string  `json:"stages"`
	Complexities []string  `json:"complexities"`
	Methods      []string  `json:"methods"`
	Models       []string  `json:"models"`
	Fixtures     []Fixture `json:"fixtures,omitempty"` // override (e.g. a replayed ticket)
	Repetitions  int       `json:"repetitions,omitempty"`
}

// SweepResult is the outcome of a sweep.
type SweepResult struct {
	ID         string       `json:"id"`
	StartedAt  time.Time    `json:"started_at"`
	FinishedAt time.Time    `json:"finished_at"`
	Cells      []CellResult `json:"cells"`
}

// MatrixRunner executes benchmark sweeps with bounded parallelism.
type MatrixRunner struct {
	cfg      MatrixConfig
	executor CellExecutor
	sinks    []telemetry.RunSink
	now      func() time.Time
}

// NewMatrixRunner creates a runner.
func NewMatrixRunner(cfg MatrixConfig, executor CellExecutor) *MatrixRunner {
	return &MatrixRunner{cfg: cfg, executor: executor, now: time.Now}
}

// Config returns the runner's matrix configuration.
func (r *MatrixRunner) Config() MatrixConfig { return r.cfg }

// AddRunSink records every repetition as a shadow telemetry run (e.g. the telemetry datastore).
func (r *MatrixRunner) AddRunSink(s telemetry.RunSink) { r.sinks = append(r.sinks, s) }

func pick(selected []string, all []string) []string {
	if len(selected) == 0 {
		return all
	}
	return selected
}

// Run executes every permutation of the sweep and returns per-cell results in deterministic order.
func (r *MatrixRunner) Run(ctx context.Context, sw Sweep) (SweepResult, error) {
	if err := ctx.Err(); err != nil {
		return SweepResult{}, err
	}
	if sw.ID == "" {
		sw.ID = fmt.Sprintf("sweep-%d", r.now().UnixNano())
	}
	var allStages, allCx, allMethods, allModels []string
	for _, s := range r.cfg.Stages {
		allStages = append(allStages, s.ID)
	}
	for _, c := range r.cfg.Complexities {
		allCx = append(allCx, c.ID)
	}
	for _, m := range r.cfg.Methods {
		allMethods = append(allMethods, m.ID)
	}
	tiers := map[string]string{}
	for _, m := range r.cfg.Models {
		allModels = append(allModels, m.ID)
		tiers[m.ID] = m.Tier
	}
	reps := sw.Repetitions
	if reps <= 0 {
		reps = r.cfg.Execution.Repetitions
	}
	if reps <= 0 {
		reps = 1
	}

	var cells []Cell
	for _, st := range pick(sw.Stages, allStages) {
		for _, cx := range pick(sw.Complexities, allCx) {
			for _, m := range pick(sw.Methods, allMethods) {
				for _, model := range pick(sw.Models, allModels) {
					cells = append(cells, Cell{Stage: st, Complexity: cx, Method: m, Model: model, Tier: tiers[model]})
				}
			}
		}
	}

	parallel := r.cfg.Execution.MaxParallelCells
	if parallel <= 0 {
		parallel = 1
	}
	results := make([]CellResult, len(cells))
	sem := make(chan struct{}, parallel)
	var wg sync.WaitGroup
	start := r.now()
	for i, cell := range cells {
		select {
		case <-ctx.Done():
			wg.Wait()
			return SweepResult{}, ctx.Err()
		case sem <- struct{}{}:
		}
		wg.Add(1)
		go func(i int, cell Cell) {
			defer wg.Done()
			defer func() { <-sem }()
			results[i] = r.runCell(ctx, sw, cell, reps)
		}(i, cell)
	}
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return SweepResult{}, err
	}
	return SweepResult{ID: sw.ID, StartedAt: start, FinishedAt: r.now(), Cells: results}, nil
}

func (r *MatrixRunner) runCell(ctx context.Context, sw Sweep, cell Cell, reps int) CellResult {
	res := CellResult{Cell: cell, Traces: []CellTrace{}}
	fixtures := filterFixtures(r.cfg.Fixtures, cell.Complexity, cell.Stage)
	if len(sw.Fixtures) > 0 {
		fixtures = filterFixtures(sw.Fixtures, cell.Complexity, cell.Stage)
	}
	if len(fixtures) == 0 {
		res.LastError = "no fixture for complexity " + cell.Complexity
		return res
	}

	cellCtx := ctx
	if t := r.cfg.Execution.CellTimeoutS; t > 0 {
		var cancel context.CancelFunc
		cellCtx, cancel = context.WithTimeout(ctx, time.Duration(t)*time.Second)
		defer cancel()
	}

	var passes int
	var tokens, durUS int64
	var cost float64
	var tools, iters int
	for rep := 0; rep < reps; rep++ {
		for _, fx := range fixtures {
			runID := fmt.Sprintf("%s:%s:%s:%d", sw.ID, strings.ReplaceAll(cell.Key(), "|", ":"), fx.ID, rep)
			obs, err := r.executor.Execute(cellCtx, cell, fx, runID)
			trace := CellTrace{RunID: runID, FixtureID: fx.ID, Repetition: rep, FirstPass: obs.FirstPass, Passed: obs.Passed,
				Tokens: obs.PromptTokens + obs.CompletionTokens, CostUSD: obs.CostUSD, DurationUS: obs.DurationUS, Steps: obs.Steps}
			if err != nil {
				res.Errors++
				res.LastError = err.Error()
				trace.Error = err.Error()
				res.Traces = append(res.Traces, trace)
				continue
			}
			res.Traces = append(res.Traces, trace)
			res.Samples++
			if obs.FirstPass {
				res.FirstPasses++
			}
			if obs.Passed {
				passes++
			}
			tokens += obs.PromptTokens + obs.CompletionTokens
			cost += obs.CostUSD
			durUS += obs.DurationUS
			tools += obs.ToolCalls
			iters += obs.Iterations

			rec := telemetry.RunRecord{
				RunMeta: telemetry.RunMeta{TaskID: runID, Model: cell.Model, Tier: cell.Tier, Method: cell.Method,
					Repo: fx.Repo, Category: fx.Category, Complexity: cell.Complexity, StageID: cell.Stage,
					RouterStrategy: "shadow_benchmark", Shadow: true},
				Success: obs.Passed, FirstPass: obs.FirstPass, TestIterations: obs.Iterations,
				FailureLoops: obs.Iterations - boolInt(obs.Passed), DurationUS: obs.DurationUS,
				PhaseDurationsUS: map[string]int64{cell.Stage: obs.DurationUS}, ToolCalls: obs.ToolCalls,
				PromptTokens: obs.PromptTokens, CompletionTokens: obs.CompletionTokens, CachedTokens: obs.CachedTokens,
				CostUSD: obs.CostUSD, FinishedAt: r.now(),
			}
			rec.StartedAt = rec.FinishedAt.Add(-time.Duration(obs.DurationUS) * time.Microsecond)
			for _, s := range r.sinks {
				_ = s.RecordRun(rec)
			}
		}
	}
	if n := float64(res.Samples); n > 0 {
		res.FPVR = float64(res.FirstPasses) / n
		res.PassRate = float64(passes) / n
		res.AvgTokens = float64(tokens) / n
		res.AvgCostUSD = cost / n
		res.AvgTTRSeconds = float64(durUS) / 1e6 / n
		res.AvgToolCalls = float64(tools) / n
		res.AvgIterations = float64(iters) / n
	}
	return res
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// SortCells orders results by stage, complexity, method and model.
func SortCells(cells []CellResult) {
	sort.SliceStable(cells, func(i, j int) bool { return cells[i].Cell.Key() < cells[j].Cell.Key() })
}
