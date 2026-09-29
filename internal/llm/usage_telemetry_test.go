package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/config"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

type fixedClient struct{}

func (fixedClient) Complete(ctx context.Context, req *LLMRequest) (*LLMResponse, error) {
	return &LLMResponse{Content: "ok", Provider: "stub", TokenUsage: types.TokenUsage{PromptTokens: 5, CompletionTokens: 5}}, nil
}
func (c fixedClient) Stream(ctx context.Context, req *LLMRequest, ch chan<- types.ThoughtChunk) (*LLMResponse, error) {
	return c.Complete(ctx, req)
}
func (fixedClient) CountTokens(req *LLMRequest) int64 { return 0 }

func TestOpenAIDriverExtractsCachedPromptTokens(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":1200,"completion_tokens":300,"total_tokens":1500,
			"prompt_tokens_details":{"cached_tokens":1024}}}`))
	}))
	defer srv.Close()

	// Placeholder credential for a local httptest server; not a real key.
	d := NewOpenAIDriver("sk-placeholder-for-httptest", srv.URL)
	resp, err := d.Complete(context.Background(), &LLMRequest{Model: "gpt-4o", Messages: []Message{{Role: RoleUser, Content: "x"}}})
	if err != nil {
		t.Fatal(err)
	}
	if resp.TokenUsage.PromptTokens != 1200 || resp.TokenUsage.CachedTokens != 1024 || resp.TokenUsage.CompletionTokens != 300 {
		t.Fatalf("usage = %+v", resp.TokenUsage)
	}
}

func TestClientFactoryUsageObserverFiresOnSuccessfulModel(t *testing.T) {
	f := NewClientFactory(config.GetDefaultConfig())
	f.OverrideProvider("claude", fixedClient{})
	var observed []string
	f.SetUsageObserver(func(modelStr string, resp *LLMResponse) {
		observed = append(observed, modelStr)
	})
	_, used, err := f.ExecuteWithFallbackChain(context.Background(),
		[]string{"claude/claude-3-5-sonnet"}, &LLMRequest{Messages: []Message{{Role: RoleUser, Content: "hi"}}}, nil)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if len(observed) != 1 || observed[0] != used {
		t.Fatalf("observer calls = %v, used = %s", observed, used)
	}
}
