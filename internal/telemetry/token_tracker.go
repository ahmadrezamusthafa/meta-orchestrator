package telemetry

import (
	"context"
	"sync"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/llm"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// Publisher broadcasts orchestrator events (e.g. ws.Hub.BroadcastEvent).
type Publisher func(event *types.OrchestratorEvent)

// CallMeta carries the breakdown dimensions attached to every LLM call.
type CallMeta struct {
	TaskID   string `json:"task_id"`
	StageID  string `json:"stage_id,omitempty"`
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Tier     string `json:"tier"`
	Method   string `json:"method"`
	Profile  string `json:"profile,omitempty"`
	Repo     string `json:"repo"`
	Category string `json:"category"`
	Shadow   bool   `json:"shadow,omitempty"` // true for shadow-benchmark replays
}

// TokenEvent is the payload of a telemetry.tokens.consumed event.
type TokenEvent struct {
	CallMeta
	PromptTokens     int64     `json:"prompt_tokens"`
	CompletionTokens int64     `json:"completion_tokens"`
	CachedTokens     int64     `json:"cached_tokens"`
	CostUSD          float64   `json:"cost_usd"`
	Timestamp        time.Time `json:"timestamp"`
}

// TokenSink persists token events (e.g. the telemetry datastore).
type TokenSink interface {
	RecordTokens(ev TokenEvent) error
}

// TokenTracker accounts token consumption and cost per task and fans events out to sinks.
type TokenTracker struct {
	mu      sync.Mutex
	pricing *PricingTable
	publish Publisher
	sinks   []TokenSink
	perTask map[string]*types.TokenUsage
	now     func() time.Time
}

// NewTokenTracker creates a tracker. publish may be nil.
func NewTokenTracker(pricing *PricingTable, publish Publisher) *TokenTracker {
	if pricing == nil {
		pricing = NewPricingTable()
	}
	return &TokenTracker{
		pricing: pricing,
		publish: publish,
		perTask: make(map[string]*types.TokenUsage),
		now:     time.Now,
	}
}

// AddSink registers a persistence sink.
func (t *TokenTracker) AddSink(s TokenSink) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.sinks = append(t.sinks, s)
}

// Pricing exposes the tracker's pricing table.
func (t *TokenTracker) Pricing() *PricingTable { return t.pricing }

// Record accounts one completion's usage. Cost comes from the pricing table when the model
// is priced; otherwise the driver-reported estimate is kept.
func (t *TokenTracker) Record(meta CallMeta, usage types.TokenUsage) TokenEvent {
	cost := usage.EstimatedCostUSD
	if _, ok := t.pricing.Lookup(meta.Model); ok {
		cost = t.pricing.Cost(meta.Model, usage.PromptTokens, usage.CompletionTokens, usage.CachedTokens)
	}

	ev := TokenEvent{
		CallMeta:         meta,
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
		CachedTokens:     usage.CachedTokens,
		CostUSD:          cost,
		Timestamp:        t.now(),
	}

	t.mu.Lock()
	u, ok := t.perTask[meta.TaskID]
	if !ok {
		u = &types.TokenUsage{}
		t.perTask[meta.TaskID] = u
	}
	u.PromptTokens += ev.PromptTokens
	u.CompletionTokens += ev.CompletionTokens
	u.CachedTokens += ev.CachedTokens
	u.TotalTokens += ev.PromptTokens + ev.CompletionTokens
	u.EstimatedCostUSD += ev.CostUSD
	sinks := append([]TokenSink(nil), t.sinks...)
	t.mu.Unlock()

	for _, s := range sinks {
		_ = s.RecordTokens(ev)
	}
	if t.publish != nil {
		t.publish(&types.OrchestratorEvent{
			Type:      types.EventTelemetryTokens,
			TaskID:    meta.TaskID,
			StageID:   meta.StageID,
			Timestamp: ev.Timestamp,
			Payload:   ev,
		})
	}
	return ev
}

// TaskUsage returns accumulated usage for a task.
func (t *TokenTracker) TaskUsage(taskID string) types.TokenUsage {
	t.mu.Lock()
	defer t.mu.Unlock()
	if u, ok := t.perTask[taskID]; ok {
		return *u
	}
	return types.TokenUsage{}
}

// Instrument wraps a provider client so every successful completion is recorded.
func (t *TokenTracker) Instrument(client llm.ProviderClient, meta CallMeta) llm.ProviderClient {
	return &instrumentedClient{inner: client, tracker: t, meta: meta}
}

type instrumentedClient struct {
	inner   llm.ProviderClient
	tracker *TokenTracker
	meta    CallMeta
}

func (c *instrumentedClient) Complete(ctx context.Context, req *llm.LLMRequest) (*llm.LLMResponse, error) {
	resp, err := c.inner.Complete(ctx, req)
	if err == nil && resp != nil {
		c.observe(resp)
	}
	return resp, err
}

func (c *instrumentedClient) Stream(ctx context.Context, req *llm.LLMRequest, ch chan<- types.ThoughtChunk) (*llm.LLMResponse, error) {
	resp, err := c.inner.Stream(ctx, req, ch)
	if err == nil && resp != nil {
		c.observe(resp)
	}
	return resp, err
}

func (c *instrumentedClient) CountTokens(req *llm.LLMRequest) int64 { return c.inner.CountTokens(req) }

func (c *instrumentedClient) observe(resp *llm.LLMResponse) {
	meta := c.meta
	if resp.Model != "" && meta.Model == "" {
		meta.Model = resp.Model
	}
	if resp.Provider != "" && meta.Provider == "" {
		meta.Provider = resp.Provider
	}
	ev := c.tracker.Record(meta, resp.TokenUsage)
	resp.TokenUsage.EstimatedCostUSD = ev.CostUSD
}
