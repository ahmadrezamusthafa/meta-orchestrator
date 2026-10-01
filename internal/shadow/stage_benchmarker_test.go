package shadow

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/llm"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// echoClient repeats its prompt back, the laziest possible answer.
type echoClient struct {
	mu      sync.Mutex
	prompts map[string]string // system prompt role → user prompt
	fail    string            // role whose call errors
}

func (c *echoClient) Complete(ctx context.Context, req *llm.LLMRequest) (*llm.LLMResponse, error) {
	system, user := req.Messages[0].Content, req.Messages[len(req.Messages)-1].Content
	c.mu.Lock()
	if c.prompts == nil {
		c.prompts = map[string]string{}
	}
	for _, role := range []string{"supervisor_plan", "frontend_agent", "backend_agent", "supervisor_merge", "react_agent"} {
		if strings.Contains(system, "the "+role+" ") {
			c.prompts[role] = user
			if role == c.fail {
				c.mu.Unlock()
				return nil, errors.New("provider unavailable")
			}
		}
	}
	c.mu.Unlock()
	return &llm.LLMResponse{Content: "echo: " + user,
		TokenUsage: types.TokenUsage{PromptTokens: 100, CompletionTokens: 10, EstimatedCostUSD: 0.001}}, nil
}

func (c *echoClient) Stream(ctx context.Context, req *llm.LLMRequest, ch chan<- types.ThoughtChunk) (*llm.LLMResponse, error) {
	return c.Complete(ctx, req)
}
func (c *echoClient) CountTokens(req *llm.LLMRequest) int64 { return 0 }

func benchWith(c llm.ProviderClient) *StageBenchmarker {
	return NewStageBenchmarker(func(string) (llm.ProviderClient, string, error) { return c, "m", nil }, nil, nil)
}

// Acceptance keywords are a hidden rubric: echoing the prompt must not pass verification.
func TestStageBenchmarkerDoesNotLeakAcceptanceKeywords(t *testing.T) {
	fx := Fixture{ID: "fx", Complexity: "LOW", Title: "Guard webhook", Description: "Return an error on empty payloads.",
		Expect: []string{"rollback", "idempotent"}}
	obs, err := benchWith(&echoClient{}).Execute(context.Background(), Cell{Stage: "task_implementation", Complexity: "LOW", Method: "ReAct"}, fx, "r")
	if err != nil {
		t.Fatal(err)
	}
	if obs.FirstPass {
		t.Fatal("echoing the prompt passed first time: acceptance keywords leaked into the prompt")
	}
	// The verifier's feedback names the missing criteria, so the retry can recover.
	if !obs.Passed || obs.Iterations != 2 {
		t.Fatalf("obs = %+v, want pass on the second attempt after feedback", obs)
	}
}

func TestExpectVerifierMatchesWholeWords(t *testing.T) {
	cases := []struct {
		expect []string
		output string
		pass   bool
	}{
		{[]string{"ci"}, "We made a design decision.", false},
		{[]string{"ci"}, "Adds a CI job.", true},
		{[]string{"nil"}, "Plain vanilla handler.", false},
		{[]string{"nil"}, "Return 400 on a nil payload.", true},
		{[]string{"test", "endpoint"}, "Added endpoints with unit tests.", true},
		{[]string{"v2"}, "Publish the v2 schema.", true},
		{[]string{"migration"}, "Run migrations in CI.", true},
	}
	for _, c := range cases {
		got, _ := ExpectVerifier(context.Background(), Cell{}, Fixture{Expect: c.expect}, c.output)
		if got != c.pass {
			t.Errorf("expect %v in %q = %v, want %v", c.expect, c.output, got, c.pass)
		}
	}
}

// Supervisor sub-agents run as one phase: each sees the plan, neither sees the other.
func TestStageBenchmarkerRunsSupervisorSubAgentsInParallel(t *testing.T) {
	c := &echoClient{}
	fx := Fixture{ID: "fx", Complexity: "HIGH", Title: "Version contract", Expect: []string{"contract"}}
	obs, err := benchWith(c).Execute(context.Background(), Cell{Stage: "task_implementation", Complexity: "HIGH", Method: "Supervisor"}, fx, "r")
	if err != nil {
		t.Fatal(err)
	}
	for _, role := range []string{"frontend_agent", "backend_agent"} {
		p := c.prompts[role]
		if !strings.Contains(p, "[supervisor_plan output]") {
			t.Fatalf("%s did not see the plan: %q", role, p)
		}
		if strings.Contains(p, "[frontend_agent output]") || strings.Contains(p, "[backend_agent output]") {
			t.Fatalf("%s saw a sibling's output: %q", role, p)
		}
	}
	merge := c.prompts["supervisor_merge"]
	if !strings.Contains(merge, "[frontend_agent output]") || !strings.Contains(merge, "[backend_agent output]") {
		t.Fatalf("merge did not see both sub-agents: %q", merge)
	}
	if len(obs.Steps) != 4 || obs.Steps[1].Role != "frontend_agent" || obs.Steps[2].Role != "backend_agent" {
		t.Fatalf("steps = %+v", obs.Steps)
	}
}

// An errored run still reports what it spent and how long it took.
func TestStageBenchmarkerErroredRunKeepsUsage(t *testing.T) {
	c := &echoClient{fail: "backend_agent"}
	fx := Fixture{ID: "fx", Complexity: "HIGH", Title: "t", Expect: []string{"x"}}
	obs, err := benchWith(c).Execute(context.Background(), Cell{Method: "Supervisor"}, fx, "r")
	if err == nil {
		t.Fatal("want error")
	}
	if obs.PromptTokens != 200 || obs.CostUSD == 0 || obs.DurationUS < 0 {
		t.Fatalf("obs = %+v, want plan + frontend usage recorded", obs)
	}
}
