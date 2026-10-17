package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// OpenCodeDriver implements ProviderClient for local inference (Ollama, vLLM, DeepSeek-Coder)
// through the OpenAI-compatible chat completions endpoint those servers expose.
type OpenCodeDriver struct {
	baseURL    string
	model      string
	httpClient *http.Client
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
		baseURL:    baseURL,
		model:      model,
		httpClient: &http.Client{Timeout: 10 * time.Minute}, // local models can be slow; ctx bounds it further
	}
}

// chatURL is the OpenAI-compatible endpoint under baseURL, which may or may not already end in /v1.
func (d *OpenCodeDriver) chatURL() string {
	base := strings.TrimRight(d.baseURL, "/")
	if !strings.HasSuffix(base, "/v1") {
		base += "/v1"
	}
	return base + "/chat/completions"
}

// Complete executes a completion on the local inference server. An unreachable server is an
// error, so the failover chain moves on instead of accepting an invented answer.
func (d *OpenCodeDriver) Complete(ctx context.Context, req *LLMRequest) (*LLMResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("request cannot be nil")
	}

	model := req.Model
	if model == "" {
		model = d.model
	}

	msgs := make([]openAIMsg, 0, len(req.Messages))
	for _, m := range req.Messages {
		role := string(m.Role)
		if role == "" {
			role = "user"
		}
		msgs = append(msgs, openAIMsg{Role: role, Content: m.Content})
	}
	if len(msgs) == 0 {
		msgs = append(msgs, openAIMsg{Role: "user", Content: "Hello"})
	}
	data, err := json.Marshal(openAIRequest{Model: model, Messages: msgs})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal opencode request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, d.chatURL(), bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	start := time.Now()
	httpResp, err := d.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("opencode request failed: %w", err)
	}
	defer httpResp.Body.Close()
	if httpResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(httpResp.Body, 4096))
		return nil, fmt.Errorf("opencode api error (status %d): %s", httpResp.StatusCode, string(body))
	}
	var parsed openAIResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("failed to parse opencode response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return nil, fmt.Errorf("opencode: the server returned no choices")
	}

	content := parsed.Choices[0].Message.Content
	finishReason := parsed.Choices[0].FinishReason
	if finishReason == "" {
		finishReason = "stop"
	}
	promptTokens := parsed.Usage.PromptTokens
	if promptTokens == 0 {
		promptTokens = d.CountTokens(req)
	}
	completionTokens := parsed.Usage.CompletionTokens
	if completionTokens == 0 {
		completionTokens = int64(len(content) / 4)
	}

	return &LLMResponse{
		Content:      content,
		FinishReason: finishReason,
		Provider:     "opencode",
		Model:        model,
		DurationMS:   time.Since(start).Milliseconds(),
		TokenUsage: types.TokenUsage{
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
			TotalTokens:      promptTokens + completionTokens,
			EstimatedCostUSD: 0.0, // Local inference has 0 direct API cost
		},
	}, nil
}

// Stream completes the request and reports the answer as one chunk.
func (d *OpenCodeDriver) Stream(ctx context.Context, req *LLMRequest, chunkCh chan<- types.ThoughtChunk) (*LLMResponse, error) {
	resp, err := d.Complete(ctx, req)
	if chunkCh != nil && err == nil {
		chunkCh <- types.ThoughtChunk{
			ProfileName: "local_coder",
			ModelID:     resp.Model,
			Content:     resp.Content,
			IsComplete:  true,
		}
	}
	return resp, err
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
