package shadow

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/llm"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/telemetry"
)

// ClientResolver returns a provider client for a "provider/model" id (llm.ClientFactory.GetClient).
type ClientResolver func(model string) (llm.ProviderClient, string, error)

// Verifier decides whether a cell's final output satisfies the fixture. feedback is sent
// back to the model on the next iteration when it fails.
type Verifier func(ctx context.Context, cell Cell, fx Fixture, output string) (passed bool, feedback string)

// ExpectVerifier passes when every fixture expectation appears in the output as a whole word
// (case-insensitive, plural and -ed/-ing forms accepted). Substring matching would let short
// keywords pass by accident: "ci" inside "decision", "nil" inside "vanilla".
func ExpectVerifier(ctx context.Context, cell Cell, fx Fixture, output string) (bool, string) {
	var missing []string
	for _, e := range fx.Expect {
		if !expectRe(e).MatchString(output) {
			missing = append(missing, e)
		}
	}
	if len(missing) == 0 {
		return true, ""
	}
	return false, "Verification failed: output is missing required acceptance criteria: " + strings.Join(missing, ", ")
}

var expectCache sync.Map // keyword → *regexp.Regexp

func expectRe(keyword string) *regexp.Regexp {
	if re, ok := expectCache.Load(keyword); ok {
		return re.(*regexp.Regexp)
	}
	re := regexp.MustCompile(`(?i)(^|[^\pL\pN])` + regexp.QuoteMeta(strings.TrimSpace(keyword)) + `(s|es|ed|ing)?($|[^\pL\pN])`)
	expectCache.Store(keyword, re)
	return re
}

// methodRoles is the call pattern of each methodology: phases run in order, roles within a
// phase run concurrently (Supervisor's sub-agents), and the single role of the last phase
// produces the verified output.
var methodRoles = map[string][][]string{
	"BMAD":       {{"product_manager"}, {"atdd_qa_engineer"}, {"system_architect"}, {"lead_developer"}},
	"Supervisor": {{"supervisor_plan"}, {"frontend_agent", "backend_agent"}, {"supervisor_merge"}},
	"ReAct":      {{"react_agent"}},
	"Superpower": {{"infrastructure_planner"}, {"autonomous_executor"}},
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

// Execute runs the method's phases in order (each seeing prior output), verifies the final
// output, and retries the final role with verifier feedback up to maxIterations.
//
// The acceptance keywords are a hidden rubric: they are not put in the prompt, otherwise any
// model that echoes its instructions passes first time and FPVR measures nothing. A failed
// attempt gets the missing criteria back as feedback, like a failing test report.
func (b *StageBenchmarker) Execute(ctx context.Context, cell Cell, fx Fixture, runID string) (obs Observation, err error) {
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

	started := b.now()
	// Duration and usage are recorded on every return, so an errored run still reports what it spent.
	defer func() { obs.DurationUS = b.now().Sub(started).Microseconds() }()
	task := fmt.Sprintf("SDLC stage: %s\nComplexity: %s\nTask: %s\n%s", cell.Stage, cell.Complexity, fx.Title, fx.Description)

	type result struct {
		out  string
		step TraceStep
		resp *llm.LLMResponse
	}
	call := func(role string, attempt int, messages []llm.Message) (result, error) {
		t0 := b.now()
		resp, err := client.Complete(ctx, &llm.LLMRequest{Model: modelID, Messages: messages, Temperature: 0.2})
		if err != nil {
			return result{}, err
		}
		step := TraceStep{Role: role, Attempt: attempt, PromptTokens: resp.TokenUsage.PromptTokens,
			CompletionTokens: resp.TokenUsage.CompletionTokens, ToolCalls: len(resp.ToolCalls),
			DurationUS: b.now().Sub(t0).Microseconds()}
		return result{out: resp.Content, step: step, resp: resp}, nil
	}
	record := func(r result) {
		obs.Steps = append(obs.Steps, r.step)
		obs.PromptTokens += r.resp.TokenUsage.PromptTokens
		obs.CompletionTokens += r.resp.TokenUsage.CompletionTokens
		obs.CachedTokens += r.resp.TokenUsage.CachedTokens
		obs.CostUSD += r.resp.TokenUsage.EstimatedCostUSD
		obs.ToolCalls += len(r.resp.ToolCalls)
	}
	system := func(role string) llm.Message {
		return llm.Message{Role: llm.RoleSystem, Content: fmt.Sprintf("You are the %s in a %s execution pipeline. %s", role, cell.Method, roleBriefs[role])}
	}

	history := ""
	for _, phase := range roles[:len(roles)-1] {
		// Every role of a phase sees the same history; parallel roles do not see each other.
		results := make([]result, len(phase))
		errs := make([]error, len(phase))
		var wg sync.WaitGroup
		for i, role := range phase {
			wg.Add(1)
			go func(i int, role string) {
				defer wg.Done()
				results[i], errs[i] = call(role, 1, []llm.Message{system(role), {Role: llm.RoleUser, Content: task + history}})
			}(i, role)
		}
		wg.Wait()
		for i, role := range phase {
			if errs[i] != nil {
				continue
			}
			record(results[i])
			history += fmt.Sprintf("\n\n[%s output]\n%s", role, results[i].out)
		}
		for _, e := range errs {
			if e != nil {
				return obs, e
			}
		}
	}

	final := roles[len(roles)-1][0]
	messages := []llm.Message{system(final), {Role: llm.RoleUser, Content: task + history}}
	for attempt := 1; attempt <= b.maxIterations; attempt++ {
		if err := ctx.Err(); err != nil {
			return obs, err
		}
		r, err := call(final, attempt, messages)
		if err != nil {
			return obs, err
		}
		record(r)
		out := r.out
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
	return obs, nil
}
