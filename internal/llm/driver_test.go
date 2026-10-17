package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// fakeLocalModelServer is an OpenAI-compatible chat completions endpoint (as Ollama and vLLM serve).
func fakeLocalModelServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body openAIRequest
		if r.URL.Path != "/v1/chat/completions" || json.NewDecoder(r.Body).Decode(&body) != nil || len(body.Messages) == 0 {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"local answer for ` + body.Model +
			`"},"finish_reason":"stop"}],"usage":{"prompt_tokens":12,"completion_tokens":3}}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestOpenCodeCallsTheLocalServer(t *testing.T) {
	srv := fakeLocalModelServer(t)
	resp, err := NewOpenCodeDriver(srv.URL, "qwen").Complete(context.Background(),
		&LLMRequest{Messages: []Message{{Role: RoleUser, Content: "hi"}}})
	if err != nil || resp.Content != "local answer for qwen" || resp.TokenUsage.TotalTokens != 15 {
		t.Fatalf("complete = %+v, %v", resp, err)
	}
	if _, err := NewOpenCodeDriver(srv.URL+"/v1/", "").Complete(context.Background(), &LLMRequest{}); err != nil {
		t.Fatalf("a base URL ending in /v1 must not be doubled: %v", err)
	}
	srv.Close()
	if resp, err := NewOpenCodeDriver(srv.URL, "").Complete(context.Background(), &LLMRequest{}); err == nil {
		t.Fatalf("an unreachable local server must fail so the chain fails over, got %+v", resp)
	}
}

func TestAllLLMDriversNormalizedContract(t *testing.T) {
	drivers := map[string]ProviderClient{
		"anthropic":   NewAnthropicDriver("test-key", ""),
		"antigravity": NewAntigravityDriver("test-key", ""),
		"openai":      NewOpenAIDriver("test-key", ""),
		"opencode":    NewOpenCodeDriver(fakeLocalModelServer(t).URL, ""),
	}

	testTool := &types.UniversalSkillContract{
		Name:         "run_playwright_e2e",
		Description:  "Run end-to-end tests",
		SourceFormat: types.SkillFormatNative,
	}

	req := &LLMRequest{
		Messages: []Message{
			{Role: RoleSystem, Content: "You are an autonomous AI software engineer."},
			{Role: RoleUser, Content: "Implement OrderController API with ATDD verification."},
		},
		Tools: []*types.UniversalSkillContract{testTool},
	}

	ctx := context.Background()

	for providerName, driver := range drivers {
		// 1. Test Complete
		resp, err := driver.Complete(ctx, req)
		if err != nil {
			t.Fatalf("[%s] Complete failed: %v", providerName, err)
		}
		if resp.Provider != providerName {
			t.Errorf("[%s] expected provider '%s', got '%s'", providerName, providerName, resp.Provider)
		}
		if resp.TokenUsage.TotalTokens <= 0 {
			t.Errorf("[%s] expected positive total tokens, got %d", providerName, resp.TokenUsage.TotalTokens)
		}

		// 2. Test Stream
		chunkCh := make(chan types.ThoughtChunk, 10)
		streamResp, err := driver.Stream(ctx, req, chunkCh)
		close(chunkCh)
		if err != nil {
			t.Fatalf("[%s] Stream failed: %v", providerName, err)
		}
		if streamResp == nil {
			t.Fatalf("[%s] streamResp is nil", providerName)
		}

		var chunksReceived int
		for range chunkCh {
			chunksReceived++
		}
		if chunksReceived == 0 {
			t.Errorf("[%s] expected at least 1 streaming thought chunk, got 0", providerName)
		}

		// 3. Test Token Counting
		tokens := driver.CountTokens(req)
		if tokens <= 0 {
			t.Errorf("[%s] expected positive token count, got %d", providerName, tokens)
		}
	}
}
