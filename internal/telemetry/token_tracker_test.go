package telemetry

import (
	"context"
	"math"
	"sync"
	"testing"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/llm"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

type recordingSink struct {
	mu     sync.Mutex
	events []TokenEvent
}

func (s *recordingSink) RecordTokens(ev TokenEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, ev)
	return nil
}

type stubClient struct {
	usage types.TokenUsage
	model string
}

func (c *stubClient) Complete(ctx context.Context, req *llm.LLMRequest) (*llm.LLMResponse, error) {
	return &llm.LLMResponse{Content: "ok", TokenUsage: c.usage, Provider: "stub", Model: c.model}, nil
}

func (c *stubClient) Stream(ctx context.Context, req *llm.LLMRequest, ch chan<- types.ThoughtChunk) (*llm.LLMResponse, error) {
	return c.Complete(ctx, req)
}

func (c *stubClient) CountTokens(req *llm.LLMRequest) int64 { return 0 }

func almostEqual(a, b float64) bool { return math.Abs(a-b) < 1e-12 }

func TestPricingCostFormula(t *testing.T) {
	p := NewPricingTable()
	p.Set("test-model", ModelPrice{InputPer1M: 3.0, OutputPer1M: 15.0, CachedInputPer1M: 0.30})

	// 10k prompt tokens of which 4k are cache reads, 2k completion.
	got := p.Cost("test-model", 10000, 2000, 4000)
	want := 6000*3.0/1e6 + 4000*0.30/1e6 + 2000*15.0/1e6
	if !almostEqual(got, want) {
		t.Fatalf("cost = %.10f, want %.10f", got, want)
	}

	// Cached tokens larger than prompt are clamped (never negative uncached share).
	if c := p.Cost("test-model", 100, 0, 500); !almostEqual(c, 100*0.30/1e6) {
		t.Fatalf("clamped cost = %.10f", c)
	}
}

func TestPricingLookupNormalizesProviderAndVersion(t *testing.T) {
	p := NewPricingTable()
	cases := []string{
		"claude/claude-3-5-sonnet-20241022",
		"anthropic/claude-3-5-sonnet",
		"claude-3-5-sonnet-latest",
	}
	for _, m := range cases {
		price, ok := p.Lookup(m)
		if !ok {
			t.Fatalf("expected price for %s", m)
		}
		if price.InputPer1M != 3.0 || price.OutputPer1M != 15.0 {
			t.Fatalf("%s resolved to %+v", m, price)
		}
	}
	// gpt-4o-mini must not resolve to gpt-4o (longest-prefix match).
	mini, _ := p.Lookup("openai/gpt-4o-mini")
	full, _ := p.Lookup("openai/gpt-4o")
	if mini.InputPer1M == full.InputPer1M {
		t.Fatalf("gpt-4o-mini resolved to gpt-4o pricing: %+v", mini)
	}
	if _, ok := p.Lookup("unknown-vendor/mystery"); ok {
		t.Fatal("unknown model should not resolve")
	}
	if c := p.Cost("unknown-vendor/mystery", 1000, 1000, 0); c != 0 {
		t.Fatalf("unknown model cost should be 0, got %v", c)
	}
}

func TestTokenTrackerTenMockCompletions(t *testing.T) {
	pricing := NewPricingTable()
	pricing.Set("mock-model", ModelPrice{InputPer1M: 2.5, OutputPer1M: 10.0, CachedInputPer1M: 1.25})

	sink := &recordingSink{}
	var published []*types.OrchestratorEvent
	var pubMu sync.Mutex
	tracker := NewTokenTracker(pricing, func(ev *types.OrchestratorEvent) {
		pubMu.Lock()
		published = append(published, ev)
		pubMu.Unlock()
	})
	tracker.AddSink(sink)

	meta := CallMeta{TaskID: "TASK-1", Provider: "openai", Model: "mock-model", Tier: "tier2",
		Method: "ReAct", Repo: "backend-core", Category: "crud", StageID: "task_implementation"}

	var wantPrompt, wantCompletion, wantCached int64
	var wantCost float64
	for i := 1; i <= 10; i++ {
		prompt, completion, cached := int64(1000*i), int64(250*i), int64(100*i)
		client := &stubClient{model: "mock-model", usage: types.TokenUsage{
			PromptTokens: prompt, CompletionTokens: completion, CachedTokens: cached,
			TotalTokens: prompt + completion,
		}}
		wrapped := tracker.Instrument(client, meta)
		if _, err := wrapped.Complete(context.Background(), &llm.LLMRequest{}); err != nil {
			t.Fatal(err)
		}
		wantPrompt += prompt
		wantCompletion += completion
		wantCached += cached
		wantCost += float64(prompt-cached)*2.5/1e6 + float64(cached)*1.25/1e6 + float64(completion)*10.0/1e6
	}

	if len(sink.events) != 10 {
		t.Fatalf("sink recorded %d events, want 10", len(sink.events))
	}
	if len(published) != 10 {
		t.Fatalf("published %d events, want 10", len(published))
	}
	for _, ev := range published {
		if ev.Type != types.EventTelemetryTokens {
			t.Fatalf("event type = %s", ev.Type)
		}
	}

	u := tracker.TaskUsage("TASK-1")
	if u.PromptTokens != wantPrompt || u.CompletionTokens != wantCompletion || u.CachedTokens != wantCached {
		t.Fatalf("usage = %+v, want prompt=%d completion=%d cached=%d", u, wantPrompt, wantCompletion, wantCached)
	}
	if u.TotalTokens != wantPrompt+wantCompletion {
		t.Fatalf("total = %d", u.TotalTokens)
	}
	if !almostEqual(u.EstimatedCostUSD, wantCost) {
		t.Fatalf("cost = %.10f, want %.10f", u.EstimatedCostUSD, wantCost)
	}
	if sink.events[9].Repo != "backend-core" || sink.events[9].Category != "crud" {
		t.Fatalf("breakdown dimensions lost: %+v", sink.events[9])
	}
}

func TestTokenTrackerOverridesDriverCostWithPricingTable(t *testing.T) {
	pricing := NewPricingTable()
	pricing.Set("m", ModelPrice{InputPer1M: 1, OutputPer1M: 1})
	tracker := NewTokenTracker(pricing, nil)
	ev := tracker.Record(CallMeta{TaskID: "T", Model: "m"}, types.TokenUsage{
		PromptTokens: 1_000_000, CompletionTokens: 1_000_000, EstimatedCostUSD: 999,
	})
	if !almostEqual(ev.CostUSD, 2.0) {
		t.Fatalf("cost = %v, want 2.0 from pricing table", ev.CostUSD)
	}
	// Unknown model keeps driver-reported estimate rather than silently zeroing.
	ev = tracker.Record(CallMeta{TaskID: "T", Model: "nope"}, types.TokenUsage{PromptTokens: 10, EstimatedCostUSD: 0.5})
	if ev.CostUSD != 0.5 {
		t.Fatalf("unknown-model cost = %v, want driver estimate 0.5", ev.CostUSD)
	}
}
