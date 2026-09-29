package shadow

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/config"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/router"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

var buildTime = time.Date(2026, 9, 28, 9, 30, 0, 0, time.UTC)

func result(stage, cx, method, model, tier string, samples, firstPasses int, cost, ttr, tokens float64) CellResult {
	return CellResult{Cell: Cell{Stage: stage, Complexity: cx, Method: method, Model: model, Tier: tier},
		Samples: samples, FirstPasses: firstPasses, FPVR: float64(firstPasses) / float64(samples),
		AvgCostUSD: cost, AvgTTRSeconds: ttr, AvgTokens: tokens}
}

// syntheticTelemetry encodes the plan's expected winners:
// Planning → BMAD, Implementation LOW → ReAct, Implementation HIGH → Supervisor, SYSTEM → Superpower.
func syntheticTelemetry() []CellResult {
	return []CellResult{
		// prd_discovery / MEDIUM: BMAD is far more reliable; ReAct cheaper but fails often.
		result("prd_discovery", "MEDIUM", "BMAD", "claude/claude-3-5-sonnet-20241022", "tier1", 10, 9, 0.30, 90, 40000),
		result("prd_discovery", "MEDIUM", "ReAct", "openai/gpt-4o-mini", "tier2", 10, 4, 0.02, 20, 6000),
		result("prd_discovery", "MEDIUM", "Supervisor", "claude/claude-3-5-sonnet-20241022", "tier1", 10, 7, 0.40, 120, 50000),
		// task_implementation / LOW: everyone passes; ReAct on a tier2 model is cheapest and fastest.
		result("task_implementation", "LOW", "ReAct", "openai/gpt-4o-mini", "tier2", 10, 10, 0.01, 8, 4500),
		result("task_implementation", "LOW", "BMAD", "claude/claude-3-5-sonnet-20241022", "tier1", 10, 10, 0.25, 60, 30000),
		result("task_implementation", "LOW", "Supervisor", "openai/gpt-4o", "tier1", 10, 10, 0.18, 45, 22000),
		// task_implementation / HIGH: Supervisor coordinates multi-repo best.
		result("task_implementation", "HIGH", "Supervisor", "claude/claude-3-5-sonnet-20241022", "tier1", 10, 9, 0.50, 200, 70000),
		result("task_implementation", "HIGH", "ReAct", "openai/gpt-4o-mini", "tier2", 10, 3, 0.05, 60, 12000),
		result("task_implementation", "HIGH", "BMAD", "claude/claude-3-5-sonnet-20241022", "tier1", 10, 8, 0.60, 260, 90000),
		// task_implementation / SYSTEM: Superpower controls the environment.
		result("task_implementation", "SYSTEM", "Superpower", "claude/claude-3-5-sonnet-20241022", "tier1", 10, 9, 0.35, 150, 45000),
		result("task_implementation", "SYSTEM", "ReAct", "openai/gpt-4o-mini", "tier2", 10, 4, 0.04, 70, 9000),
		// Under-sampled candidate must never win.
		result("task_implementation", "SYSTEM", "BMAD", "openai/gpt-4o", "tier1", 1, 1, 0.01, 5, 1000),
	}
}

func cellFor(m BestMethodsMatrix, stage, cx string) (MatrixCell, bool) {
	for _, c := range m.Cells {
		if c.StageID == stage && c.Complexity == cx {
			return c, true
		}
	}
	return MatrixCell{}, false
}

func TestMatrixBuilderSelectsWinningMethodPerCell(t *testing.T) {
	m := NewMatrixBuilder(DefaultBuilderConfig()).Build("sweep-x", syntheticTelemetry(), buildTime)
	want := map[[2]string]string{
		{"prd_discovery", "MEDIUM"}:       "BMAD",
		{"task_implementation", "LOW"}:    "ReAct",
		{"task_implementation", "HIGH"}:   "Supervisor",
		{"task_implementation", "SYSTEM"}: "Superpower",
	}
	for k, method := range want {
		c, ok := cellFor(m, k[0], k[1])
		if !ok {
			t.Fatalf("missing cell %v", k)
		}
		if c.OptimalMethod != method {
			t.Fatalf("%v winner = %s (%s), want %s; cell=%+v", k, c.OptimalMethod, c.WinningModel, method, c)
		}
	}
	low, _ := cellFor(m, "task_implementation", "LOW")
	if low.WinningModel != "openai/gpt-4o-mini" || low.ModelTier != "tier2" || low.FPVRPercent != 100 || low.Samples != 10 {
		t.Fatalf("LOW cell = %+v", low)
	}
	if low.AvgTokens != 4500 || low.AvgDurationS != 8 || low.AvgCostUSD != 0.01 {
		t.Fatalf("LOW metrics = %+v", low)
	}
	// Score = w1·FPVR − w2·NormCost − w3·NormTTR; ReAct is cheapest and fastest → norm 0 → score = w1.
	if math.Abs(low.Score-DefaultBuilderConfig().Weights.FPVR) > 1e-9 {
		t.Fatalf("LOW score = %v", low.Score)
	}
	if !containsFold(low.ParetoFront, "ReAct@openai/gpt-4o-mini") || containsFold(low.ParetoFront, "BMAD@claude/claude-3-5-sonnet-20241022") {
		t.Fatalf("pareto front = %v (BMAD is dominated by ReAct)", low.ParetoFront)
	}
	sys, _ := cellFor(m, "task_implementation", "SYSTEM")
	if sys.OptimalMethod == "BMAD" {
		t.Fatal("under-sampled candidate won")
	}
	if m.Source != SourceShadowBenchmark || m.SweepID != "sweep-x" || len(m.Cells) != 4 {
		t.Fatalf("matrix = source %s sweep %s cells %d", m.Source, m.SweepID, len(m.Cells))
	}
}

func TestScoreFormula(t *testing.T) {
	w := ScoreWeights{FPVR: 1, Cost: 0.5, TTR: 0.25}
	if got := w.Score(0.9, 0.5, 1.0); math.Abs(got-(0.9-0.25-0.25)) > 1e-12 {
		t.Fatalf("score = %v", got)
	}
}

func TestMatrixMergeKeepsPreviousCellsForUnsweptTuples(t *testing.T) {
	b := NewMatrixBuilder(DefaultBuilderConfig())
	prev := b.Build("old", syntheticTelemetry(), buildTime.Add(-time.Hour))
	next := b.Build("new", []CellResult{
		result("task_implementation", "LOW", "BMAD", "m1", "tier1", 5, 5, 0.001, 1, 100),
	}, buildTime)
	merged := MergeMatrix(prev, next)
	if len(merged.Cells) != 4 {
		t.Fatalf("merged cells = %d", len(merged.Cells))
	}
	low, _ := cellFor(merged, "task_implementation", "LOW")
	if low.OptimalMethod != "BMAD" {
		t.Fatalf("new sweep did not replace LOW cell: %+v", low)
	}
	prd, _ := cellFor(merged, "prd_discovery", "MEDIUM")
	if prd.OptimalMethod != "BMAD" || merged.SweepID != "new" {
		t.Fatalf("previous cell lost: %+v", prd)
	}
}

func TestWriteAndLoadBestMethodsMatrixFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "configs", "best_methods_matrix.json")
	m := NewMatrixBuilder(DefaultBuilderConfig()).Build("s", syntheticTelemetry(), buildTime)
	if err := WriteMatrixFile(path, m); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	var generic map[string]interface{}
	if err := json.Unmarshal(raw, &generic); err != nil {
		t.Fatalf("written matrix is not valid JSON: %v", err)
	}
	cells := generic["cells"].([]interface{})
	first := cells[0].(map[string]interface{})
	for _, k := range []string{"stage_id", "complexity", "optimal_method", "winning_model", "model_tier", "fpvr_percent", "avg_tokens", "avg_duration_s", "avg_cost_usd", "score", "samples"} {
		if _, ok := first[k]; !ok {
			t.Fatalf("cell missing %q: %v", k, first)
		}
	}
	back, err := LoadMatrixFile(path)
	if err != nil || len(back.Cells) != len(m.Cells) || !back.GeneratedAt.Equal(m.GeneratedAt) {
		t.Fatalf("round trip = %+v %v", back, err)
	}
}

func TestBenchmarkReportMarkdown(t *testing.T) {
	sweep := SweepResult{ID: "sweep-x", StartedAt: buildTime.Add(-10 * time.Minute), FinishedAt: buildTime, Cells: syntheticTelemetry()}
	m := NewMatrixBuilder(DefaultBuilderConfig()).Build(sweep.ID, sweep.Cells, buildTime)
	dir := t.TempDir()
	path, err := NewReporter().Write(dir, sweep, m, nil)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != "20260928T093000Z_stage_method_benchmark.md" {
		t.Fatalf("report path = %s", path)
	}
	data, _ := os.ReadFile(path)
	md := string(data)
	for _, want := range []string{
		"# Stage × Method Benchmark Report",
		"| Stage | LOW | MEDIUM | HIGH | SYSTEM |",
		"| task_implementation | **ReAct**",
		"95% CI",
		"| ReAct | openai/gpt-4o-mini | tier2 | 10 | 100.0% [72.2–100.0] |",
		"+2400.0%", // BMAD cost differential vs ReAct winner in LOW (0.25 vs 0.01)
		"Score = 1.00·FPVR − 0.30·NormalizedCost − 0.10·NormalizedTTR",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("report missing %q:\n%s", want, md)
		}
	}
}

func TestMatrixStoreHotReloadEmitsEventAndRouterAdopts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "best_methods_matrix.json")
	var mu sync.Mutex
	var events []*types.OrchestratorEvent
	store := NewMatrixStore(path, func(ev *types.OrchestratorEvent) {
		mu.Lock()
		events = append(events, ev)
		mu.Unlock()
	})

	cfg := config.GetDefaultConfig()
	cfg.ModelTiers.Tier1Reasoning = "claude/claude-3-5-sonnet-20241022"
	cfg.ModelTiers.Tier2CodeGen = "openai/gpt-4o-mini"
	r := router.NewRouter(cfg)
	// The chain is the allow-list, so both tier models must be in it for the shift to be observable.
	r.SetSettings(router.RouterSettings{PriorityChain: []router.PriorityModelItem{
		{Provider: "claude", Model: "claude-3-5-sonnet-20241022", Enabled: true},
		{Provider: "openai", Model: "gpt-4o-mini", Enabled: true},
	}})
	r.SetMethodAdvisor(store)

	before := r.Route("CODEGEN_IMPLEMENT", "HIGH", nil)
	if before.Method != "bmad" {
		t.Fatalf("baseline HIGH implementation method = %s", before.Method)
	}

	m := NewMatrixBuilder(DefaultBuilderConfig()).Build("s", syntheticTelemetry(), buildTime)
	if err := store.Update(m, filepath.Join(dir, "report.md")); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	if len(events) != 1 || events[0].Type != types.EventRouterMatrixUpdated {
		t.Fatalf("events = %+v", events)
	}
	mu.Unlock()

	after := r.Route("CODEGEN_IMPLEMENT", "HIGH", nil)
	if after.Method != "supervisor" {
		t.Fatalf("router did not adopt matrix winner without restart: %+v", after)
	}
	low := r.Route("task_implementation", "LOW", nil)
	if low.Method != "react" || low.Tier != "tier2" || low.Model != "openai/gpt-4o-mini" {
		t.Fatalf("LOW implementation = %+v", low)
	}
	// Default-source matrices never override routing.
	def := DefaultMatrix(buildTime)
	if err := store.Update(def, ""); err != nil {
		t.Fatal(err)
	}
	if d := r.Route("CODEGEN_IMPLEMENT", "HIGH", nil); d.Method != "bmad" {
		t.Fatalf("default matrix overrode routing: %+v", d)
	}

	// External edits to the file are picked up by the watcher.
	if err := WriteMatrixFile(path, m); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(2 * time.Second)
	_ = os.Chtimes(path, future, future)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go store.Watch(ctx, 5*time.Millisecond)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if d := r.Route("CODEGEN_IMPLEMENT", "HIGH", nil); d.Method == "supervisor" {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("watcher did not hot-reload the edited matrix file")
}

func TestShippedBestMethodsMatrixIsValid(t *testing.T) {
	m, err := LoadMatrixFile("../../configs/best_methods_matrix.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Cells) != 36 {
		t.Fatalf("cells = %d, want 36 (9 stages × 4 complexities)", len(m.Cells))
	}
	seen := map[string]bool{}
	for _, c := range m.Cells {
		seen[c.StageID+"|"+c.Complexity] = true
		if c.OptimalMethod == "" || c.ModelTier == "" {
			t.Fatalf("incomplete cell %+v", c)
		}
	}
	if len(seen) != 36 {
		t.Fatalf("duplicate cells: %d distinct", len(seen))
	}
}
