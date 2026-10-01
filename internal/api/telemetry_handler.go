package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/llm"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/router"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/router/feedback"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/shadow"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/telemetry"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
	"github.com/redis/go-redis/v9"
)

// TelemetryConfig configures the Phase 4 telemetry, feedback and shadow benchmarking subsystems.
type TelemetryConfig struct {
	// DataDir persists telemetry tables as JSONL; empty keeps them in memory.
	DataDir string
	// PricingFile optionally overrides default per-model prices.
	PricingFile string
	// Redis backs the dynamic routing weight table; nil uses an in-memory table.
	Redis *redis.Client
	// MatrixPath is configs/best_methods_matrix.json; empty uses <RootDir>/configs/best_methods_matrix.json.
	MatrixPath string
	// ShadowMatrixPath is configs/shadow_matrix.json; empty uses <RootDir>/configs/shadow_matrix.json.
	ShadowMatrixPath string
	// ReportDir receives benchmark reports; empty uses <RootDir>/.sdlc/benchmarks.
	ReportDir string
	// EnableShadow starts the shadow benchmark daemon. Off by default: every sweep makes paid LLM calls.
	EnableShadow bool
	// CalibrationWindow is the history the calibrator learns from (default 30 days).
	CalibrationWindow time.Duration
}

type telemetrySubsystem struct {
	store      *telemetry.Datastore
	tracker    *telemetry.TokenTracker
	ttr        *telemetry.TTRCollector
	calibrator *feedback.Calibrator
	matrix     *shadow.MatrixStore
	daemon     *shadow.Daemon
	publisher  *shadow.Publisher
	runner     *shadow.MatrixRunner
	window     time.Duration
	lastCal    feedback.CalibrationResult
	lastCalAt  time.Time
}

func (r *Router) initTelemetry() {
	tc := r.cfg.Telemetry
	publish := func(ev *types.OrchestratorEvent) {
		if r.cfg.WSHub != nil {
			r.cfg.WSHub.BroadcastEvent(ev)
		}
	}

	store := telemetry.NewMemoryDatastore()
	if tc.DataDir != "" {
		if ds, err := telemetry.OpenDatastore(tc.DataDir); err == nil {
			store = ds
		} else {
			fmt.Printf("[Telemetry] Falling back to in-memory datastore: %v\n", err)
		}
	}
	pricing := telemetry.NewPricingTable()
	if tc.PricingFile != "" {
		if err := pricing.LoadFromFile(tc.PricingFile); err != nil {
			fmt.Printf("[Telemetry] Pricing overrides not loaded: %v\n", err)
		}
	}
	tracker := telemetry.NewTokenTracker(pricing, publish)
	tracker.AddSink(store)
	ttr := telemetry.NewTTRCollector()
	ttr.AddSink(store)
	ttr.SetTokenSource(tracker.TaskUsage)

	var weights feedback.WeightStore = feedback.NewMemoryWeightStore()
	if tc.Redis != nil {
		weights = feedback.NewRedisWeightStore(tc.Redis, "")
	}
	cal := feedback.NewCalibrator(weights, feedback.DefaultCalibratorConfig())
	_ = cal.Load(context.Background())

	matrixPath := tc.MatrixPath
	if matrixPath == "" && r.cfg.RootDir != "" {
		matrixPath = filepath.Join(r.cfg.RootDir, "configs", "best_methods_matrix.json")
	}
	matrix := shadow.NewMatrixStore(matrixPath, publish)
	if matrixPath != "" {
		if _, err := os.Stat(matrixPath); err == nil {
			go matrix.Watch(context.Background(), 5*time.Second) // hot-reload manual edits
		}
	}

	window := tc.CalibrationWindow
	if window <= 0 {
		window = 30 * 24 * time.Hour
	}
	sub := &telemetrySubsystem{store: store, tracker: tracker, ttr: ttr, calibrator: cal, matrix: matrix, window: window}

	if r.strategyRouter != nil {
		r.strategyRouter.SetTierAdvisor(cal)
		r.strategyRouter.SetMethodAdvisor(matrix)
	}

	if tc.EnableShadow {
		r.initShadow(sub, tc, publish)
	}
	r.telemetry = sub
}

func (r *Router) initShadow(sub *telemetrySubsystem, tc TelemetryConfig, publish telemetry.Publisher) {
	cfgPath := tc.ShadowMatrixPath
	if cfgPath == "" {
		cfgPath = filepath.Join(r.cfg.RootDir, "configs", "shadow_matrix.json")
	}
	mcfg, err := shadow.LoadMatrixConfig(cfgPath)
	if err != nil {
		fmt.Printf("[Shadow] Disabled: %v\n", err)
		return
	}
	bench := shadow.NewStageBenchmarker(func(model string) (llm.ProviderClient, string, error) {
		return r.clientFactory.GetClient(model)
	}, sub.tracker, nil)
	bench.SetMaxIterations(mcfg.Execution.MaxIterations)
	runner := shadow.NewMatrixRunner(mcfg, bench)
	runner.AddRunSink(sub.store)

	reportDir := tc.ReportDir
	if reportDir == "" {
		reportDir = filepath.Join(r.cfg.RootDir, ".sdlc", "benchmarks")
	}
	sub.runner = runner
	sub.publisher = &shadow.Publisher{Builder: shadow.NewMatrixBuilder(shadow.DefaultBuilderConfig()),
		Reporter: shadow.NewReporter(), Store: sub.matrix, ReportDir: reportDir}

	var sandbox shadow.Sandbox
	if _, err := exec.LookPath("docker"); err == nil {
		sandbox = shadow.NewDockerSandbox(shadow.SandboxConfig{WorkRoot: filepath.Join(r.cfg.RootDir, "workspaces", ".shadow")}, nil)
	}
	ceiling := mcfg.Execution.CPUCeilingPercent
	sub.daemon = shadow.NewDaemon(shadow.DaemonConfig{Workers: 1, CPUCeilingPercent: ceiling}, nil,
		func(ctx context.Context, job shadow.Job) error { return r.runShadowJob(ctx, sub, sandbox, job) })
	sub.daemon.Start(context.Background())
	fmt.Printf("[Shadow] Benchmark daemon started (CPU ceiling %.0f%%, sandbox=%v)\n", ceiling, sandbox != nil)
}

func (r *Router) runShadowJob(ctx context.Context, sub *telemetrySubsystem, sandbox shadow.Sandbox, job shadow.Job) error {
	spec, err := shadow.CaptureReplay(&job.Task, job.Baseline, r.repoPaths(job.Task.AssignedRepos), nil)
	if err != nil {
		return err
	}
	if sandbox != nil {
		h, err := sandbox.Provision(ctx, job.ID, spec)
		if err != nil {
			return err
		}
		defer sandbox.Teardown(context.Background(), h)
	}
	cfg := sub.runner.Config()
	complexity := shadow.CanonicalComplexity(spec.Complexity)
	fx := shadow.Fixture{ID: spec.SourceTaskID, Title: spec.Title, Description: spec.Description,
		Complexity: complexity, Category: spec.Category, Expect: replayExpectations(spec)}
	if len(job.Task.AssignedRepos) > 0 {
		fx.Repo = job.Task.AssignedRepos[0]
	}
	stages := cfg.Execution.ReplayStages
	if len(stages) == 0 {
		stages = []string{"task_implementation"}
	}
	sweep, err := sub.runner.Run(ctx, shadow.Sweep{ID: job.ID, Stages: stages, Complexities: []string{complexity}, Fixtures: []shadow.Fixture{fx}})
	if err != nil {
		return err
	}
	_, _, err = sub.publisher.Publish(ctx, sweep)
	return err
}

var (
	scenarioLine = regexp.MustCompile(`(?im)^\s*(scenario|then|it|test)[:\s(]+['"]?([^'"\n]+)`)
	wordRe       = regexp.MustCompile(`[A-Za-z][A-Za-z0-9_-]{4,}`)
)

// replayExpectations derives acceptance keywords for a replayed ticket: words from ATDD
// scenario/assertion lines when captured, otherwise salient words from the title.
func replayExpectations(spec shadow.ReplaySpec) []string {
	var source []string
	for _, a := range spec.ATDDAssertions {
		for _, m := range scenarioLine.FindAllStringSubmatch(a.Content, -1) {
			source = append(source, m[2])
		}
	}
	if len(source) == 0 {
		source = []string{spec.Title}
	}
	stop := map[string]bool{"should": true, "which": true, "with": true, "their": true, "there": true, "about": true, "after": true}
	seen := map[string]bool{}
	var out []string
	for _, s := range source {
		for _, w := range wordRe.FindAllString(s, -1) {
			lw := strings.ToLower(w)
			if stop[lw] || seen[lw] {
				continue
			}
			seen[lw] = true
			out = append(out, lw)
			if len(out) == 3 {
				return out
			}
		}
	}
	if len(out) == 0 {
		out = []string{strings.ToLower(spec.StageID)}
	}
	return out
}

func (r *Router) repoPaths(repos []string) map[string]string {
	paths := map[string]string{}
	if r.cfg.ProjectManager == nil {
		return paths
	}
	for _, p := range r.cfg.ProjectManager.List() {
		for _, repo := range p.Repos {
			for _, want := range repos {
				if repo.Name == want && repo.Path != "" {
					paths[want] = repo.Path
				}
			}
		}
	}
	return paths
}

// taskRunMeta builds the telemetry identity of a task execution.
func taskRunMeta(task *types.Task, decision *router.RoutingDecision, complexity, taskType string) telemetry.RunMeta {
	repo := ""
	if len(task.AssignedRepos) > 0 {
		repo = task.AssignedRepos[0]
	}
	return telemetry.RunMeta{TaskID: task.ID, Model: decision.Model, Tier: decision.Tier, Method: decision.Method,
		Repo: repo, Category: taskType, Complexity: strings.ToUpper(complexity), StageID: task.CurrentStageID,
		RouterStrategy: decision.Strategy, RuleID: decision.RuleID}
}

// recalibrate feeds recent production history back into routing weights.
func (r *Router) recalibrate(ctx context.Context) (feedback.CalibrationResult, error) {
	sub := r.telemetry
	runs := sub.store.Runs(telemetry.Filter{Since: time.Now().Add(-sub.window)})
	res, err := sub.calibrator.Calibrate(ctx, runs)
	if err != nil {
		return res, err
	}
	r.mu.Lock()
	sub.lastCal, sub.lastCalAt = res, time.Now()
	r.mu.Unlock()
	return res, nil
}

// onTaskCompleted hands a completed production task to the shadow daemon. Callers may hold r.mu.
func (r *Router) onTaskCompleted(task *types.Task) {
	if r.telemetry == nil || r.telemetry.daemon == nil || task == nil {
		return
	}
	var baseline telemetry.RunRecord
	runs := r.telemetry.store.Runs(telemetry.Filter{})
	for i := len(runs) - 1; i >= 0; i-- {
		if runs[i].TaskID == task.ID {
			baseline = runs[i]
			break
		}
	}
	r.telemetry.daemon.OnTaskCompleted(task, baseline)
}

// benchmarkCellView is one measured benchmark cell placed on a pipeline stage.
type benchmarkCellView struct {
	shadow.MatrixCell
	StageID          string `json:"stage_id"`        // pipeline stage the measurement routes
	BenchmarkStage   string `json:"benchmark_stage"` // stage the benchmark measured (shared, e.g. UAT uses E2E)
	Applied          bool   `json:"applied"`         // the router uses this winner
	NotAppliedReason string `json:"not_applied_reason,omitempty"`
}

// handleBenchmarks serves shadow-benchmark measurements per pipeline stage and complexity. Only
// measured cells are listed — the routing policy for the rest is the router preview — and each says
// whether routing applies it, so the matrix never shows policy as if it were evidence.
func (r *Router) handleBenchmarks(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		r.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	store := r.telemetry.matrix
	m := store.Current()
	cells := []benchmarkCellView{}
	applied := 0
	for _, st := range stagePipeline {
		bst := shadow.CanonicalStage(st)
		for _, cx := range router.Complexities {
			c, ok := m.Cell(bst, cx)
			if !ok || m.Source != shadow.SourceShadowBenchmark || c.Source == shadow.SourceDefault {
				continue
			}
			v := benchmarkCellView{MatrixCell: c, StageID: st, BenchmarkStage: bst}
			v.Applied, v.NotAppliedReason = store.Applicability(m, c)
			if v.Applied {
				applied++
			}
			cells = append(cells, v)
		}
	}
	minSamples, minFPVR := store.Thresholds()
	source := m.Source
	if source == "" {
		source = shadow.SourceDefault // no matrix file yet: nothing measured
	}
	resp := map[string]interface{}{
		"total_cells":    len(stagePipeline) * len(router.Complexities),
		"measured_cells": len(cells),
		"applied_cells":  applied,
		"stages":         stagePipeline,
		"complexities":   router.Complexities,
		"min_samples":    minSamples,
		"min_fpvr":       minFPVR,
		"shadow_enabled": r.telemetry.daemon != nil,
		"source":         source,
		"weights":        m.Weights,
		"matrix":         cells,
	}
	if m.Source == shadow.SourceShadowBenchmark {
		resp["generated_at"], resp["sweep_id"] = m.GeneratedAt, m.SweepID
	}
	r.writeJSON(w, http.StatusOK, resp)
}

type weightLockRequest struct {
	TaskType string `json:"task_type"`
	Tier     string `json:"tier"`
	LockedBy string `json:"locked_by"`
	Reason   string `json:"reason"`
	Unlock   bool   `json:"unlock"`
}

// handleRouterWeights: GET lists the dynamic weight table; POST recalibrates from telemetry now.
func (r *Router) handleRouterWeights(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	switch req.Method {
	case http.MethodGet:
		weights, err := r.telemetry.calibrator.Weights(ctx)
		if err != nil {
			r.writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		r.mu.RLock()
		last, lastAt := r.telemetry.lastCal, r.telemetry.lastCalAt
		r.mu.RUnlock()
		sort.Slice(weights, func(i, j int) bool { return weights[i].TaskType < weights[j].TaskType })
		r.writeJSON(w, http.StatusOK, map[string]interface{}{
			"weights": weights, "last_calibrated_at": lastAt, "promotions": last.Promotions, "downgrades": last.Report.Downgrades,
		})
	case http.MethodPost:
		res, err := r.recalibrate(ctx)
		if err != nil {
			r.writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		r.writeJSON(w, http.StatusOK, res)
	default:
		r.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// handleRouterWeightLock: POST {task_type, tier, locked_by, reason} pins a task type; {unlock:true} releases it.
func (r *Router) handleRouterWeightLock(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		r.writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var body weightLockRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil || strings.TrimSpace(body.TaskType) == "" {
		r.writeError(w, http.StatusBadRequest, "task_type is required")
		return
	}
	var err error
	if body.Unlock {
		err = r.telemetry.calibrator.Unlock(req.Context(), body.TaskType)
	} else {
		if body.LockedBy == "" {
			body.LockedBy = "admin"
		}
		err = r.telemetry.calibrator.Lock(req.Context(), body.TaskType, body.Tier, body.LockedBy, body.Reason)
	}
	if err != nil {
		r.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	r.writeJSON(w, http.StatusOK, map[string]interface{}{"status": "ok", "task_type": body.TaskType, "unlocked": body.Unlock})
}

// handleShadowStatus reports shadow daemon counters and the active matrix identity.
func (r *Router) handleShadowStatus(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		r.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	m := r.telemetry.matrix.Current()
	resp := map[string]interface{}{
		"enabled":             r.telemetry.daemon != nil,
		"matrix_source":       m.Source,
		"matrix_generated_at": m.GeneratedAt,
		"matrix_cells":        len(m.Cells),
	}
	if r.telemetry.daemon != nil {
		resp["stats"] = r.telemetry.daemon.Stats()
	}
	r.writeJSON(w, http.StatusOK, resp)
}
