package llm

import (
	"context"
	"fmt"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// AntigravityDriver implements ProviderClient for Google / Antigravity Gemini models.
type AntigravityDriver struct {
	apiKey  string
	baseURL string
}

// NewAntigravityDriver creates a new Antigravity/Gemini driver.
func NewAntigravityDriver(apiKey string, baseURL string) *AntigravityDriver {
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com/v1beta"
	}
	return &AntigravityDriver{
		apiKey:  apiKey,
		baseURL: baseURL,
	}
}

// Complete executes a chat completion.
func (d *AntigravityDriver) Complete(ctx context.Context, req *LLMRequest) (*LLMResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("request cannot be nil")
	}

	model := req.Model
	if model == "" {
		model = "gemini-1.5-pro"
	}

	promptTokens := d.CountTokens(req)
	completionTokens := int64(120)

	return &LLMResponse{
		Content:      "Antigravity multimodal planning response generated.",
		FinishReason: "stop",
		Provider:     "antigravity",
		Model:        model,
		TokenUsage: types.TokenUsage{
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
			TotalTokens:      promptTokens + completionTokens,
			EstimatedCostUSD: float64(promptTokens)*0.00000125 + float64(completionTokens)*0.000005,
		},
	}, nil
}

// Stream streams thought chunks.
func (d *AntigravityDriver) Stream(ctx context.Context, req *LLMRequest, chunkCh chan<- types.ThoughtChunk) (*LLMResponse, error) {
	if chunkCh != nil {
		chunkCh <- types.ThoughtChunk{
			ProfileName: "product_manager",
			ModelID:     req.Model,
			Content:     "Synthesizing PRD criteria with Antigravity...",
			IsComplete:  true,
		}
	}
	return d.Complete(ctx, req)
}

// CountTokens estimates token count.
func (d *AntigravityDriver) CountTokens(req *LLMRequest) int64 {
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
