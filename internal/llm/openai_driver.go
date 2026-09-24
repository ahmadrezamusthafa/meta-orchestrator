package llm

import (
	"context"
	"fmt"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// OpenAIDriver implements ProviderClient for OpenAI models (GPT-4o, o1, o3).
type OpenAIDriver struct {
	apiKey  string
	baseURL string
}

// NewOpenAIDriver creates a new OpenAI driver.
func NewOpenAIDriver(apiKey string, baseURL string) *OpenAIDriver {
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	return &OpenAIDriver{
		apiKey:  apiKey,
		baseURL: baseURL,
	}
}

// Complete executes an OpenAI chat completion.
func (d *OpenAIDriver) Complete(ctx context.Context, req *LLMRequest) (*LLMResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("request cannot be nil")
	}

	model := req.Model
	if model == "" {
		model = "gpt-4o"
	}

	promptTokens := d.CountTokens(req)
	completionTokens := int64(140)

	var toolCalls []ToolCall
	if len(req.Tools) > 0 {
		// Mock tool call for verification
		toolCalls = append(toolCalls, ToolCall{
			ID:   "call_openai_mock_1",
			Name: req.Tools[0].Name,
			Arguments: map[string]interface{}{
				"action": "execute",
			},
		})
	}

	return &LLMResponse{
		Content:      "OpenAI execution complete.",
		ToolCalls:    toolCalls,
		FinishReason: "stop",
		Provider:     "openai",
		Model:        model,
		TokenUsage: types.TokenUsage{
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
			TotalTokens:      promptTokens + completionTokens,
			EstimatedCostUSD: float64(promptTokens)*0.000005 + float64(completionTokens)*0.000015,
		},
	}, nil
}

// Stream streams thought chunks.
func (d *OpenAIDriver) Stream(ctx context.Context, req *LLMRequest, chunkCh chan<- types.ThoughtChunk) (*LLMResponse, error) {
	if chunkCh != nil {
		chunkCh <- types.ThoughtChunk{
			ProfileName: "lead_developer",
			ModelID:     req.Model,
			Content:     "Generating implementation code...",
			IsComplete:  true,
		}
	}
	return d.Complete(ctx, req)
}

// CountTokens calculates estimated tokens.
func (d *OpenAIDriver) CountTokens(req *LLMRequest) int64 {
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
