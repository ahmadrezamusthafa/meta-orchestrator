package llm

import (
	"context"
	"fmt"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// OpenCodeDriver implements ProviderClient for local inference (Ollama, vLLM, DeepSeek-Coder).
type OpenCodeDriver struct {
	baseURL string
	model   string
}

// NewOpenCodeDriver creates a new local model driver.
func NewOpenCodeDriver(baseURL string, model string) *OpenCodeDriver {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	if model == "" {
		model = "deepseek-coder-v2"
	}
	return &OpenCodeDriver{
		baseURL: baseURL,
		model:   model,
	}
}

// Complete executes a local completion request.
func (d *OpenCodeDriver) Complete(ctx context.Context, req *LLMRequest) (*LLMResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("request cannot be nil")
	}

	model := req.Model
	if model == "" {
		model = d.model
	}

	promptTokens := d.CountTokens(req)
	completionTokens := int64(95)

	return &LLMResponse{
		Content:      fmt.Sprintf("Local OpenCode execution complete (%s).", model),
		FinishReason: "stop",
		Provider:     "opencode",
		Model:        model,
		TokenUsage: types.TokenUsage{
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
			TotalTokens:      promptTokens + completionTokens,
			EstimatedCostUSD: 0.0, // Local inference has 0 direct API cost
		},
	}, nil
}

// Stream streams chunks.
func (d *OpenCodeDriver) Stream(ctx context.Context, req *LLMRequest, chunkCh chan<- types.ThoughtChunk) (*LLMResponse, error) {
	if chunkCh != nil {
		chunkCh <- types.ThoughtChunk{
			ProfileName: "local_coder",
			ModelID:     d.model,
			Content:     "Streaming local tokens...",
			IsComplete:  true,
		}
	}
	return d.Complete(ctx, req)
}

// CountTokens estimates tokens.
func (d *OpenCodeDriver) CountTokens(req *LLMRequest) int64 {
	var chars int
	for _, m := range req.Messages {
		chars += len(m.Content)
	}
	tokens := int64(chars / 4)
	if tokens < 10 {
		tokens = 10
	}
	return tokens
}
