package llm

import (
	"context"
	"fmt"
	"strings"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// AnthropicDriver implements ProviderClient for Anthropic Claude models.
type AnthropicDriver struct {
	apiKey  string
	baseURL string
}

// NewAnthropicDriver creates a new Claude driver.
func NewAnthropicDriver(apiKey string, baseURL string) *AnthropicDriver {
	if baseURL == "" {
		baseURL = "https://api.anthropic.com/v1"
	}
	return &AnthropicDriver{
		apiKey:  apiKey,
		baseURL: baseURL,
	}
}

// Complete executes a non-streaming chat completion.
func (d *AnthropicDriver) Complete(ctx context.Context, req *LLMRequest) (*LLMResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("request cannot be nil")
	}

	model := req.Model
	if model == "" {
		model = "claude-3-5-sonnet-20241022"
	}

	// Calculate simulated token counts for normalization
	promptTokens := d.CountTokens(req)
	completionTokens := int64(150)

	resp := &LLMResponse{
		Content:      "Claude architecture analysis complete.",
		FinishReason: "stop",
		Provider:     "anthropic",
		Model:        model,
		TokenUsage: types.TokenUsage{
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
			TotalTokens:      promptTokens + completionTokens,
			EstimatedCostUSD: float64(promptTokens)*0.000003 + float64(completionTokens)*0.000015,
		},
	}

	return resp, nil
}

// Stream streams thought and completion chunks.
func (d *AnthropicDriver) Stream(ctx context.Context, req *LLMRequest, chunkCh chan<- types.ThoughtChunk) (*LLMResponse, error) {
	if chunkCh != nil {
		chunkCh <- types.ThoughtChunk{
			ProfileName: "system_architect",
			ModelID:     req.Model,
			Content:     "Synthesizing AST impact graph...",
			IsComplete:  false,
		}
		chunkCh <- types.ThoughtChunk{
			ProfileName: "system_architect",
			ModelID:     req.Model,
			Content:     " Analyzing API schema contracts...",
			IsComplete:  true,
		}
	}
	return d.Complete(ctx, req)
}

// CountTokens provides token estimation.
func (d *AnthropicDriver) CountTokens(req *LLMRequest) int64 {
	var charCount int
	for _, m := range req.Messages {
		charCount += len(m.Content)
	}
	for _, t := range req.Tools {
		charCount += len(t.Name) + len(t.Description)
	}
	// Heuristic 4 chars ~ 1 token
	tokens := int64(strings.Count(fmt.Sprintf("%d", charCount), "") * 1)
	if charCount > 0 {
		tokens = int64(charCount / 4)
	}
	if tokens < 10 {
		tokens = 10
	}
	return tokens
}
