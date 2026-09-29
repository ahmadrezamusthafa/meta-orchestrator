package shadow

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/llm"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/telemetry"
)

// ClientResolver returns a provider client for a "provider/model" id (llm.ClientFactory.GetClient).
type ClientResolver func(model string) (llm.ProviderClient, string, error)

// Verifier decides whether a cell's final output satisfies the fixture. feedback is sent
// back to the model on the next iteration when it fails.
type Verifier func(ctx context.Context, cell Cell, fx Fixture, output string) (passed bool, feedback string)

// ExpectVerifier passes when every fixture expectation appears in the output (case-insensitive).
func ExpectVerifier(ctx context.Context, cell Cell, fx Fixture, output string) (bool, string) {
	lower := strings.ToLower(output)
	var missing []string
	for _, e := range fx.Expect {
		if !strings.Contains(lower, strings.ToLower(e)) {
			missing = append(missing, e)
		}
	}
	if len(missing) == 0 {
		return true, ""
	}
	return false, "Verification failed: output is missing required acceptance criteria: " + strings.Join(missing, ", ")
}

// methodRoles is the call pattern of each methodology; the last role produces the verified output.
var methodRoles = map[string][]string{
	"BMAD":       {"product_manager", "atdd_qa_engineer", "system_architect", "lead_developer"},
	"Supervisor": {"supervisor_plan", "frontend_agent", "backend_agent", "supervisor_merge"},
	"ReAct":      {"react_agent"},
	"Superpower": {"infrastructure_planner", "autonomous_executor"},
}

var roleBriefs = map[string]string{
	"product_manager":        "Clarify scope and acceptance criteria for the request.",
	"atdd_qa_engineer":       "Write failing acceptance tests that encode the criteria.",
	"system_architect":       "Design the minimal technical approach that satisfies the tests.",
	"lead_developer":         "Produce the implementation that makes the acceptance tests pass.",
	"supervisor_plan":        "Split the request into independent sub-tasks per repository.",
	"frontend_agent":         "Deliver the frontend sub-task.",
	"backend_agent":          "Deliver the backend sub-task.",
	"supervisor_merge":       "Merge sub-task results into one coherent change that satisfies the acceptance criteria.",
	"react_agent":            "Reason, act with tools and iterate until the acceptance criteria are satisfied.",
	"infrastructure_planner": "Plan container, CI and system changes step by step.",
	"autonomous_executor":    "Execute the plan and report the resulting change that satisfies the acceptance criteria.",
}

// StageBenchmarker executes a fixture using a method's role pipeline on one model.
type StageBenchmarker struct {
	resolve       ClientResolver
	tracker       *telemetry.TokenTracker
	verify        Verifier
	maxIterations int
	now           func() time.Time
}

// NewStageBenchmarker creates the default CellExecutor. tracker and verify may be nil.
func NewStageBenchmarker(resolve ClientResolver, tracker *telemetry.TokenTracker, verify Verifier) *StageBenchmarker {
	if verify == nil {
		verify = ExpectVerifier
	}
	return &StageBenchmarker{resolve: resolve, tracker: tracker, verify: verify, maxIterations: 3, now: time.Now}
}

// SetMaxIterations caps verification retries per fixture.
func (b *StageBenchmarker) SetMaxIterations(n int) {
	if n > 0 {
		b.maxIterations = n
	}
}

// Execute runs the method's roles in sequence (each seeing prior output), verifies the final
// output, and retries the final role with verifier feedback up to maxIterations.
func (b *StageBenchmarker) Execute(ctx context.Context, cell Cell, fx Fixture, runID string) (Observation, error) {
	roles, ok := methodRoles[cell.Method]
	if !ok {
		return Observation{}, fmt.Errorf("unknown method %q", cell.Method)
	}
	client, modelID, err := b.resolve(cell.Model)
	if err != nil {
		return Observation{}, err
	}
	if b.tracker != nil {
		client = b.tracker.Instrument(client, telemetry.CallMeta{TaskID: runID, StageID: cell.Stage, Model: cell.Model,
			Tier: cell.Tier, Method: cell.Method, Repo: fx.Repo, Category: fx.Category, Shadow: true})
	}

	var obs Observation
	started := b.now()
	task := fmt.Sprintf("SDLC stage: %s\nComplexity: %s\nTask: %s\n%s\nAcceptance criteria keywords: %s",
		cell.Stage, cell.Complexity, fx.Title, fx.Description, strings.Join(fx.Expect, ", "))

	call := func(role string, attempt int, messages []llm.Message) (string, error) {
		t0 := b.now()
		resp, err := client.Complete(ctx, &llm.LLMRequest{Model: modelID, Messages: messages, Temperature: 0.2})
		if err != nil {
			return "", err
		}
		step := TraceStep{Role: role, Attempt: attempt, PromptTokens: resp.TokenUsage.PromptTokens,
			CompletionTokens: resp.TokenUsage.CompletionTokens, ToolCalls: len(resp.ToolCalls),
			DurationUS: b.now().Sub(t0).Microseconds()}
		obs.Steps = append(obs.Steps, step)
		obs.PromptTokens += resp.TokenUsage.PromptTokens
		obs.CompletionTokens += resp.TokenUsage.CompletionTokens
		obs.CachedTokens += resp.TokenUsage.CachedTokens
		obs.CostUSD += resp.TokenUsage.EstimatedCostUSD
		obs.ToolCalls += len(resp.ToolCalls)
		return resp.Content, nil
	}
	system := func(role string) llm.Message {
		return llm.Message{Role: llm.RoleSystem, Content: fmt.Sprintf("You are the %s in a %s execution pipeline. %s", role, cell.Method, roleBriefs[role])}
	}

	history := ""
	for _, role := range roles[:len(roles)-1] {
		out, err := call(role, 1, []llm.Message{system(role), {Role: llm.RoleUser, Content: task + history}})
		if err != nil {
			return obs, err
		}
		history += fmt.Sprintf("\n\n[%s output]\n%s", role, out)
	}

	final := roles[len(roles)-1]
	messages := []llm.Message{system(final), {Role: llm.RoleUser, Content: task + history}}
	for attempt := 1; attempt <= b.maxIterations; attempt++ {
		if err := ctx.Err(); err != nil {
			return obs, err
		}
		out, err := call(final, attempt, messages)
		if err != nil {
			return obs, err
		}
		obs.Iterations = attempt
		passed, feedback := b.verify(ctx, cell, fx, out)
		obs.Steps[len(obs.Steps)-1].Verified = &passed
		if passed {
			obs.Passed = true
			obs.FirstPass = attempt == 1
			break
		}
		obs.Steps[len(obs.Steps)-1].Note = feedback
		messages = append(messages, llm.Message{Role: llm.RoleAssistant, Content: out}, llm.Message{Role: llm.RoleUser, Content: feedback})
	}
	obs.DurationUS = b.now().Sub(started).Microseconds()
	return obs, nil
}
