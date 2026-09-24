package llm

import (
	"context"
	"testing"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

func TestAllLLMDriversNormalizedContract(t *testing.T) {
	drivers := map[string]ProviderClient{
		"anthropic":   NewAnthropicDriver("test-key", ""),
		"antigravity": NewAntigravityDriver("test-key", ""),
		"openai":      NewOpenAIDriver("test-key", ""),
		"opencode":    NewOpenCodeDriver("", ""),
	}

	testTool := &types.UniversalSkillContract{
		Name:        "run_playwright_e2e",
		Description: "Run end-to-end tests",
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
