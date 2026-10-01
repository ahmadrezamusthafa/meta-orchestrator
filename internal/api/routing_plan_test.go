package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/llm"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/router"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// gradingStub answers the complexity prompt with a fixed grading.
type gradingStub struct{ answer string }

func (s *gradingStub) Complete(ctx context.Context, req *llm.LLMRequest) (*llm.LLMResponse, error) {
	return &llm.LLMResponse{Content: s.answer}, nil
}
func (s *gradingStub) Stream(ctx context.Context, req *llm.LLMRequest, ch chan<- types.ThoughtChunk) (*llm.LLMResponse, error) {
	return s.Complete(ctx, req)
}
func (s *gradingStub) CountTokens(req *llm.LLMRequest) int64 { return 0 }

func planRouter(t *testing.T, answer string) *Router {
	r := NewRouter(RouterConfig{RootDir: t.TempDir()})
	for _, p := range []string{"claude", "antigravity", "openai", "opencode"} {
		r.clientFactory.OverrideProvider(p, &gradingStub{answer: answer})
	}
	return r
}

func TestAnalyzeProposesCheapPlanForLowComplexity(t *testing.T) {
	r := planRouter(t, `{"complexity":"LOW","task_type":"docs","rationale":"README wording only.","signals":["readme"]}`)
	w := do(r, http.MethodPost, "/api/v1/tasks/analyze", `{"title":"Reword README intro","assigned_repos":["web"]}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body)
	}
	var resp analyzeResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Assessment.Source != router.SourceAI || resp.Assessment.Complexity != "LOW" {
		t.Fatalf("assessment = %+v", resp.Assessment)
	}
	if len(resp.Plan) != len(stagePipeline) || len(resp.Methods) != 4 || len(resp.Models) == 0 {
		t.Fatalf("plan=%d methods=%d models=%d", len(resp.Plan), len(resp.Methods), len(resp.Models))
	}
	for _, s := range resp.Plan {
		if s.Tier == router.TierReasoning {
			t.Fatalf("LOW task routes %s to Tier 1: %+v", s.StageID, s)
		}
	}

	// The operator can re-grade; the plan is rebuilt without asking the AI.
	w = do(r, http.MethodPost, "/api/v1/tasks/analyze", `{"title":"Reword README intro","complexity":"HIGH","assigned_repos":["web"]}`)
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Assessment.Source != router.SourceOperator || resp.Plan[0].Tier != router.TierReasoning {
		t.Fatalf("operator re-grade: %+v / %+v", resp.Assessment, resp.Plan[0])
	}
}

func TestAnalyzeFallsBackWhenModelAnswerIsUnusable(t *testing.T) {
	r := planRouter(t, "sounds easy")
	w := do(r, http.MethodPost, "/api/v1/tasks/analyze", `{"title":"Add a CI pipeline job"}`)
	var resp analyzeResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Assessment.Source != router.SourceHeuristic || resp.Assessment.Complexity != "SYSTEM" || resp.Assessment.Fallback == "" {
		t.Fatalf("assessment = %+v", resp.Assessment)
	}
}

func TestConfirmedPlanOverrideDrivesStageRouting(t *testing.T) {
	r := planRouter(t, "")
	model := r.strategyRouter.ModelOptions()[0].Model
	body := `{"title":"Add invoice endpoint","assigned_repos":["backend"],"complexity":"MEDIUM","complexity_source":"ai",
		"complexity_rationale":"One service.","routing_plan":[{"stage_id":"prd_discovery","method":"Superpower","model":"` + model + `","overridden":true}]}`
	w := do(r, http.MethodPost, "/api/v1/tasks", body)
	if w.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", w.Code, w.Body)
	}
	var task types.Task
	_ = json.Unmarshal(w.Body.Bytes(), &task)
	if task.Metadata["routing_confirmed"] != "true" || task.Metadata["complexity"] != "MEDIUM" || task.Metadata["complexity_source"] != "ai" {
		t.Fatalf("metadata = %v", task.Metadata)
	}
	d := r.routeTask(&task, "MEDIUM", "crud")
	if d.Method != "superpower" || d.Model != model || d.Strategy != "user_override" {
		t.Fatalf("decision = %+v", d)
	}
	if !strings.Contains(d.Reasoning, "Operator choice") {
		t.Fatalf("reasoning = %q", d.Reasoning)
	}
}

func TestCreateWithoutPlanStoresUnconfirmedProposal(t *testing.T) {
	r := planRouter(t, "")
	w := do(r, http.MethodPost, "/api/v1/tasks", `{"title":"Fix typo in README","assigned_repos":["web"]}`)
	var task types.Task
	_ = json.Unmarshal(w.Body.Bytes(), &task)
	if task.Metadata["routing_confirmed"] != "false" || task.Metadata["complexity"] != "LOW" || len(task.RoutingPlan) != len(stagePipeline) {
		t.Fatalf("task = %+v", task)
	}
}

func TestRoutingPlanEditValidates(t *testing.T) {
	r := planRouter(t, "")
	w := do(r, http.MethodPost, "/api/v1/tasks", `{"title":"Add endpoint","assigned_repos":["api"]}`)
	var task types.Task
	_ = json.Unmarshal(w.Body.Bytes(), &task)

	for _, bad := range []string{
		`{"routing_plan":[{"stage_id":"prd_discovery","method":"yolo"}]}`,
		`{"routing_plan":[{"stage_id":"nope","method":"react"}]}`,
		`{"routing_plan":[{"stage_id":"prd_discovery","method":"react","model":"made/up"}]}`,
	} {
		if w := do(r, http.MethodPut, "/api/v1/tasks/"+task.ID+"/routing", bad); w.Code != http.StatusBadRequest {
			t.Fatalf("%s accepted: %d", bad, w.Code)
		}
	}
	w = do(r, http.MethodPut, "/api/v1/tasks/"+task.ID+"/routing",
		`{"complexity":"high","routing_plan":[{"stage_id":"prd_discovery","method":"BMAD","overridden":true}]}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body)
	}
	_ = json.Unmarshal(w.Body.Bytes(), &task)
	if task.Metadata["complexity"] != "HIGH" || task.Metadata["routing_confirmed"] != "true" || task.RouteFor("prd_discovery").Method != "bmad" {
		t.Fatalf("task = %+v", task)
	}
}

// Built-in models always carry catalog metadata: a stale chain item or a custom registration
// with the wrong price must not change what cost-optimized routing sorts on.
func TestCatalogIsSourceOfTruthForBuiltinModels(t *testing.T) {
	stale := []router.PriorityModelItem{{Provider: "claude", Model: "claude-sonnet-5-5", Name: "Next-Gen Frontier", CostPer1k: 0.015, LatencyMs: 250}}
	got := withCatalogMeta(stale)[0]
	if got.CostPer1k != 0.002 || got.Name != "Claude Sonnet 5.5" || got.LatencyMs != 140 {
		t.Fatalf("healed item = %+v", got)
	}
	registerCustomModel("claude", "claude-haiku-4-5", "Wrong", 9, 9)
	for _, m := range getAllAvailableModels() {
		if m.ModelID == "claude-haiku-4-5" && (m.CostPer1k != 0.001 || m.ModelName != "Claude Haiku 4.5") {
			t.Fatalf("custom registration overwrote the catalog: %+v", m)
		}
	}
}

func TestRouterPreviewIncludesFullMatrix(t *testing.T) {
	r := planRouter(t, "")
	w := do(r, http.MethodPost, "/api/v1/router/preview", `{"mode":"cost_optimized"}`)
	var resp struct {
		Matrix []router.MatrixCell `json:"matrix"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Matrix) != len(stagePipeline)*4 {
		t.Fatalf("matrix cells = %d", len(resp.Matrix))
	}
	methods := map[string]bool{}
	for _, c := range resp.Matrix {
		methods[c.Decision.Method] = true
		if c.Decision.TierSource != "mode" {
			t.Fatalf("cost-optimized cell %s/%s tier source = %q", c.StageID, c.Complexity, c.Decision.TierSource)
		}
	}
	// Chain modes order models only; the method still follows stage and complexity.
	if len(methods) < 2 {
		t.Fatalf("every cell uses the same method %v", methods)
	}
}
