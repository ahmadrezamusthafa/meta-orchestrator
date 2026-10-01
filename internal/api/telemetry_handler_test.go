package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/llm"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/shadow"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/telemetry"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// stubProvider keeps API tests hermetic: no live LLM calls.
type stubProvider struct{}

func (stubProvider) Complete(ctx context.Context, req *llm.LLMRequest) (*llm.LLMResponse, error) {
	return &llm.LLMResponse{Content: "ok", Provider: "stub", Model: req.Model,
		TokenUsage: types.TokenUsage{PromptTokens: 1200, CompletionTokens: 300, CachedTokens: 200, TotalTokens: 1500}}, nil
}
func (s stubProvider) Stream(ctx context.Context, req *llm.LLMRequest, ch chan<- types.ThoughtChunk) (*llm.LLMResponse, error) {
	return s.Complete(ctx, req)
}
func (stubProvider) CountTokens(req *llm.LLMRequest) int64 { return 0 }

func hermeticRouter(t *testing.T) *Router {
	r := NewRouter(RouterConfig{RootDir: t.TempDir()})
	for _, p := range []string{"claude", "antigravity", "openai", "opencode"} {
		r.clientFactory.OverrideProvider(p, stubProvider{})
	}
	r.seedFixtureTasks()
	r.reconcileIdleRunning()
	return r
}

func TestTaskExecutionRecordsTelemetryAndServesSummary(t *testing.T) {
	r := hermeticRouter(t)

	r.mu.Lock()
	r.tasks["TASK-T1"] = &types.Task{ID: "TASK-T1", Title: "Add CRUD endpoints for invoices", CurrentStageID: "task_implementation",
		State: types.TaskStatePending, AssignedRepos: []string{"backend-core"}, Metadata: map[string]string{"complexity": "LOW"}}
	r.mu.Unlock()

	r.executeTaskWithAI("TASK-T1")

	runs := r.telemetry.store.Runs(telemetry.Filter{})
	var rec telemetry.RunRecord
	for _, run := range runs {
		if run.TaskID == "TASK-T1" {
			rec = run
		}
	}
	if rec.TaskID == "" {
		t.Fatalf("no telemetry run recorded for executed task; runs=%d", len(runs))
	}
	if rec.Category != "crud" || rec.Repo != "backend-core" || rec.Method == "" || rec.Model == "" || rec.DurationUS <= 0 {
		t.Fatalf("run = %+v", rec)
	}
	if !rec.Success || !rec.FirstPass || rec.PromptTokens != 1200 || rec.CachedTokens != 200 || rec.CostUSD <= 0 {
		t.Fatalf("run accounting = %+v", rec)
	}

	r.mu.RLock()
	meta := r.tasks["TASK-T1"].Metadata
	r.mu.RUnlock()
	if meta["ttr_seconds"] == "" || meta["task_type"] != "crud" || meta["phase_durations_us"] == "" {
		t.Fatalf("task metadata missing telemetry: %v", meta)
	}

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/telemetry/summary?window=24h", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("summary → %d %s", w.Code, w.Body.String())
	}
	var s telemetry.Summary
	_ = json.Unmarshal(w.Body.Bytes(), &s)
	if s.Totals.Runs < 1 || len(s.Repos) == 0 {
		t.Fatalf("summary = %+v", s)
	}

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/telemetry/trends?window=7d", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"bucket":"day"`) {
		t.Fatalf("trends → %d %s", w.Code, w.Body.String())
	}
}

func TestBenchmarksServeDefaultMatrixAndWeightLockAPI(t *testing.T) {
	r := hermeticRouter(t)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/benchmarks", nil))
	var bench struct {
		TotalCells int                 `json:"total_cells"`
		Measured   int                 `json:"measured_cells"`
		Source     string              `json:"source"`
		Matrix     []shadow.MatrixCell `json:"matrix"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &bench)
	// The shipped default-policy matrix is not evidence: nothing is reported as measured.
	if bench.TotalCells != 32 || bench.Measured != 0 || len(bench.Matrix) != 0 || bench.Source != shadow.SourceDefault {
		t.Fatalf("benchmarks = %d cells, %d measured, source %q", bench.TotalCells, bench.Measured, bench.Source)
	}

	lock := func(body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/router/weights/lock", bytes.NewBufferString(body)))
		return rec
	}
	if rec := lock(`{"task_type":"crud","tier":"tier2","locked_by":"admin","reason":"cost"}`); rec.Code != http.StatusOK {
		t.Fatalf("lock → %d %s", rec.Code, rec.Body.String())
	}
	if d := r.strategyRouter.RouteForTask("INTAKE_PRD", "HIGH", "crud", nil); d.Tier != "tier2" {
		t.Fatalf("admin lock not applied to routing: %+v", d)
	}
	if rec := lock(`{"task_type":"crud","tier":"tier9"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid tier → %d", rec.Code)
	}
	if rec := lock(`{"tier":"tier1"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("missing task type → %d", rec.Code)
	}

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/router/weights", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"locked":true`) {
		t.Fatalf("weights → %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/router/weights", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("recalibrate → %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/shadow/status", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"enabled":false`) {
		t.Fatalf("shadow status → %d %s", w.Code, w.Body.String())
	}
}

func TestReplayExpectationsFromAssertions(t *testing.T) {
	got := replayExpectations(shadow.ReplaySpec{Title: "Add invoice export",
		ATDDAssertions: []shadow.Assertion{{Content: "Feature: x\n  Scenario: exports invoices as CSV\n  Then the download contains totals"}}})
	if len(got) != 3 || got[0] != "exports" || got[1] != "invoices" {
		t.Fatalf("expectations = %v", got)
	}
	if got := replayExpectations(shadow.ReplaySpec{Title: "Refund webhook retries"}); len(got) == 0 || got[0] != "refund" {
		t.Fatalf("title fallback = %v", got)
	}
}

func TestBenchmarksReportWhetherRoutingAppliesEachCell(t *testing.T) {
	r := hermeticRouter(t)
	res := func(stage, cx string, n, fp int) shadow.CellResult {
		return shadow.CellResult{Cell: shadow.Cell{Stage: stage, Complexity: cx, Method: "ReAct", Model: "claude/claude-sonnet-5-5", Tier: "tier2"},
			Samples: n, FirstPasses: fp, FPVR: float64(fp) / float64(n), AvgCostUSD: 0.01, AvgTTRSeconds: 10}
	}
	b := shadow.NewMatrixBuilder(shadow.BuilderConfig{Weights: shadow.DefaultBuilderConfig().Weights, MinSamples: 1, MinFPVR: 0.5})
	m := b.Build("s1", []shadow.CellResult{res("task_implementation", "LOW", 10, 10), res("e2e_validation", "MEDIUM", 2, 2)}, time.Now())
	if err := r.telemetry.matrix.Update(m, ""); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/benchmarks", nil))
	var bench struct {
		Measured int `json:"measured_cells"`
		Applied  int `json:"applied_cells"`
		Matrix   []struct {
			StageID        string `json:"stage_id"`
			BenchmarkStage string `json:"benchmark_stage"`
			Complexity     string `json:"complexity"`
			Applied        bool   `json:"applied"`
			Reason         string `json:"not_applied_reason"`
		} `json:"matrix"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &bench)
	// e2e_validation/MEDIUM also routes uat_verification/MEDIUM, so it appears twice.
	if bench.Measured != 3 || bench.Applied != 1 {
		t.Fatalf("measured=%d applied=%d: %s", bench.Measured, bench.Applied, w.Body.String())
	}
	for _, c := range bench.Matrix {
		switch c.StageID {
		case "task_implementation":
			if !c.Applied {
				t.Fatalf("10/10 cell not applied: %+v", c)
			}
		case "uat_verification":
			if c.BenchmarkStage != "e2e_validation" || c.Applied || c.Reason != "2 samples (needs 3)" {
				t.Fatalf("uat cell = %+v", c)
			}
		}
	}
}
