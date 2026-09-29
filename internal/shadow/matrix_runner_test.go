package shadow

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/llm"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/telemetry"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// scriptedClient answers with the fixture keywords when strong, and only after
// receiving verifier feedback when weak (so its first pass fails).
type scriptedClient struct {
	model  string
	strong bool
	calls  int64
}

func (c *scriptedClient) Complete(ctx context.Context, req *llm.LLMRequest) (*llm.LLMResponse, error) {
	atomic.AddInt64(&c.calls, 1)
	var last string
	for _, m := range req.Messages {
		last = m.Content
	}
	content := "draft output"
	if c.strong || strings.Contains(last, "Verification failed") {
		content = "draft output with endpoint and migration and schema"
	}
	return &llm.LLMResponse{Content: content, Model: c.model, Provider: "stub",
		ToolCalls:  []llm.ToolCall{{ID: "t1", Name: "read_file"}},
		TokenUsage: types.TokenUsage{PromptTokens: 1000, CompletionTokens: 200, TotalTokens: 1200}}, nil
}

func (c *scriptedClient) Stream(ctx context.Context, req *llm.LLMRequest, ch chan<- types.ThoughtChunk) (*llm.LLMResponse, error) {
	return c.Complete(ctx, req)
}
func (c *scriptedClient) CountTokens(req *llm.LLMRequest) int64 { return 0 }

type runCollector struct {
	mu   sync.Mutex
	runs []telemetry.RunRecord
}

func (r *runCollector) RecordRun(rec telemetry.RunRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.runs = append(r.runs, rec)
	return nil
}

func testMatrixConfig() MatrixConfig {
	return MatrixConfig{
		Stages:       []StageDef{{ID: "atdd_creation"}, {ID: "task_implementation"}, {ID: "e2e_validation"}},
		Complexities: []ComplexityDef{{ID: "LOW"}, {ID: "MEDIUM"}, {ID: "HIGH"}, {ID: "SYSTEM"}},
		Methods:      []MethodDef{{ID: "BMAD"}, {ID: "ReAct"}},
		Models: []ModelDef{
			{ID: "stub/strong", Tier: "tier1"},
			{ID: "stub/weak", Tier: "tier3"},
		},
		Fixtures: []Fixture{
			{ID: "fx-low", Complexity: "LOW", Category: "crud", Title: "Add endpoint", Expect: []string{"endpoint"}},
			{ID: "fx-med", Complexity: "MEDIUM", Category: "migration", Title: "Schema migration", Expect: []string{"migration", "schema"}},
			{ID: "fx-sys", Complexity: "SYSTEM", Category: "infra", Title: "CI pipeline", Expect: []string{"pipeline"}},
		},
		Execution: ExecutionDef{MaxParallelCells: 4, Repetitions: 2, MaxIterations: 3},
	}
}

func TestMatrixSweepTwoStagesTwoComplexities(t *testing.T) {
	strong := &scriptedClient{model: "strong", strong: true}
	weak := &scriptedClient{model: "weak"}
	clients := map[string]llm.ProviderClient{"stub/strong": strong, "stub/weak": weak}

	tracker := telemetry.NewTokenTracker(telemetry.NewPricingTable(), nil)
	sink := &runCollector{}
	bench := NewStageBenchmarker(func(model string) (llm.ProviderClient, string, error) {
		c, ok := clients[model]
		if !ok {
			return nil, "", fmt.Errorf("no client %s", model)
		}
		return c, model, nil
	}, tracker, nil)
	runner := NewMatrixRunner(testMatrixConfig(), bench)
	runner.AddRunSink(sink)

	res, err := runner.Run(context.Background(), Sweep{
		ID:           "sweep-1",
		Stages:       []string{"atdd_creation", "task_implementation"},
		Complexities: []string{"LOW", "MEDIUM"},
	})
	if err != nil {
		t.Fatal(err)
	}

	// 2 stages × 2 complexities × 2 methods × 2 models = 16 permutations, each completed.
	if len(res.Cells) != 16 {
		t.Fatalf("cells = %d, want 16", len(res.Cells))
	}
	seen := map[string]bool{}
	for _, c := range res.Cells {
		key := c.Cell.Key()
		if seen[key] {
			t.Fatalf("duplicate cell %s", key)
		}
		seen[key] = true
		if c.Samples != 2 || c.Errors != 0 {
			t.Fatalf("cell %s samples=%d errors=%d (%v)", key, c.Samples, c.Errors, c.LastError)
		}
		if len(c.Traces) != 2 || len(c.Traces[0].Steps) == 0 {
			t.Fatalf("cell %s traces = %+v", key, c.Traces)
		}
	}
	for _, stage := range []string{"atdd_creation", "task_implementation"} {
		for _, cx := range []string{"LOW", "MEDIUM"} {
			for _, m := range []string{"BMAD", "ReAct"} {
				for _, model := range []string{"stub/strong", "stub/weak"} {
					if !seen[Cell{Stage: stage, Complexity: cx, Method: m, Model: model}.Key()] {
						t.Fatalf("missing permutation %s/%s/%s/%s", stage, cx, m, model)
					}
				}
			}
		}
	}

	// Metrics are isolated per cell: the strong model passes first time, the weak one never does,
	// and BMAD (4 role hand-offs) costs more tokens than ReAct (1 call) for the same model.
	get := func(stage, cx, method, model string) CellResult {
		for _, c := range res.Cells {
			if c.Cell == (Cell{Stage: stage, Complexity: cx, Method: method, Model: model, Tier: c.Cell.Tier}) {
				return c
			}
		}
		t.Fatalf("cell not found")
		return CellResult{}
	}
	s := get("task_implementation", "LOW", "ReAct", "stub/strong")
	w := get("task_implementation", "LOW", "ReAct", "stub/weak")
	if s.FPVR != 1 || w.FPVR != 0 || w.AvgIterations <= s.AvgIterations {
		t.Fatalf("strong fpvr=%v iter=%v, weak fpvr=%v iter=%v", s.FPVR, s.AvgIterations, w.FPVR, w.AvgIterations)
	}
	if w.PassRate != 1 {
		t.Fatalf("weak model should eventually pass after feedback, pass rate = %v", w.PassRate)
	}
	b := get("task_implementation", "LOW", "BMAD", "stub/strong")
	if b.AvgTokens <= s.AvgTokens {
		t.Fatalf("BMAD tokens %v should exceed ReAct %v", b.AvgTokens, s.AvgTokens)
	}
	if s.Cell.Tier != "tier1" || w.Cell.Tier != "tier3" {
		t.Fatalf("tiers = %s / %s", s.Cell.Tier, w.Cell.Tier)
	}

	// Comparative traces are recorded as shadow runs with per-cell identity.
	if len(sink.runs) != 32 {
		t.Fatalf("recorded runs = %d, want 32 (16 cells × 2 reps)", len(sink.runs))
	}
	ids := map[string]bool{}
	for _, r := range sink.runs {
		if !r.Shadow || r.StageID == "" || r.Method == "" || r.Model == "" {
			t.Fatalf("run not tagged as shadow cell: %+v", r.RunMeta)
		}
		if ids[r.TaskID] {
			t.Fatalf("run id collision %s", r.TaskID)
		}
		ids[r.TaskID] = true
		if r.TotalTokens() == 0 {
			t.Fatalf("run %s missing token accounting", r.TaskID)
		}
	}
}

func TestMatrixSweepUsesReplayFixturesAndSkipsUnknownModel(t *testing.T) {
	cfg := testMatrixConfig()
	cfg.Models = append(cfg.Models, ModelDef{ID: "stub/missing", Tier: "tier2"})
	strong := &scriptedClient{model: "strong", strong: true}
	bench := NewStageBenchmarker(func(model string) (llm.ProviderClient, string, error) {
		if model == "stub/missing" {
			return nil, "", fmt.Errorf("provider not configured")
		}
		return strong, model, nil
	}, nil, nil)
	runner := NewMatrixRunner(cfg, bench)

	replay := Fixture{ID: "TASK-9", Complexity: "HIGH", Category: "crud", Title: "Replay", Expect: []string{"endpoint"}}
	res, err := runner.Run(context.Background(), Sweep{ID: "s2", Stages: []string{"task_implementation"},
		Complexities: []string{"HIGH"}, Methods: []string{"ReAct"}, Fixtures: []Fixture{replay}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Cells) != 3 {
		t.Fatalf("cells = %d", len(res.Cells))
	}
	for _, c := range res.Cells {
		if c.Cell.Model == "stub/missing" {
			if c.Errors == 0 || c.Samples != 0 || c.LastError == "" {
				t.Fatalf("missing model should record errors, got %+v", c)
			}
			continue
		}
		if c.Samples != 2 || c.Traces[0].FixtureID != "TASK-9" {
			t.Fatalf("replay fixture not used: %+v", c)
		}
	}
}

func TestMatrixSweepHonoursCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runner := NewMatrixRunner(testMatrixConfig(), NewStageBenchmarker(func(string) (llm.ProviderClient, string, error) {
		return &scriptedClient{strong: true}, "m", nil
	}, nil, nil))
	if _, err := runner.Run(ctx, Sweep{Stages: []string{"task_implementation"}, Complexities: []string{"LOW"}}); err == nil {
		t.Fatal("expected context error")
	}
}

func TestShippedShadowMatrixConfig(t *testing.T) {
	cfg, err := LoadMatrixConfig("../../configs/shadow_matrix.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Stages) != 9 || len(cfg.Complexities) != 4 || len(cfg.Methods) != 4 {
		t.Fatalf("stages=%d complexities=%d methods=%d", len(cfg.Stages), len(cfg.Complexities), len(cfg.Methods))
	}
	for _, m := range []string{"BMAD", "Supervisor", "ReAct", "Superpower"} {
		found := false
		for _, d := range cfg.Methods {
			found = found || d.ID == m
		}
		if !found {
			t.Fatalf("method %s missing", m)
		}
	}
	if len(cfg.Models) < 4 {
		t.Fatalf("models = %d", len(cfg.Models))
	}
	for _, cx := range []string{"LOW", "MEDIUM", "HIGH", "SYSTEM"} {
		if len(cfg.FixturesFor(cx, "")) == 0 {
			t.Fatalf("no fixture for complexity %s", cx)
		}
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
}
