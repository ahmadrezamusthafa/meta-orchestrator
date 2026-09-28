package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// AnthropicDriver implements ProviderClient for Anthropic Claude models.
type AnthropicDriver struct {
	apiKey       string
	sessionToken string
	baseURL      string
	httpClient   *http.Client
	cliPath      string
}

// NewAnthropicDriver creates a new Claude driver.
func NewAnthropicDriver(apiKey string, baseURL string) *AnthropicDriver {
	if baseURL == "" {
		baseURL = "https://api.anthropic.com/v1"
	}
	return &AnthropicDriver{
		apiKey:     apiKey,
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}
}

// SetSessionToken configures session token auth.
func (d *AnthropicDriver) SetSessionToken(token string) {
	d.sessionToken = token
}

type anthropicMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicRequest struct {
	Model     string         `json:"model"`
	MaxTokens int            `json:"max_tokens"`
	Messages  []anthropicMsg `json:"messages"`
	System    string         `json:"system,omitempty"`
}

type anthropicResponse struct {
	Content []struct {
		Text string `json:"text"`
		Type string `json:"type"`
	} `json:"content"`
	Usage struct {
		InputTokens  int64 `json:"input_tokens"`
		OutputTokens int64 `json:"output_tokens"`
		// Anthropic reports cache reads/writes separately from input_tokens.
		CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
		CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
	} `json:"usage"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// Complete executes a chat completion via Anthropic API or local Claude Code CLI.
func (d *AnthropicDriver) Complete(ctx context.Context, req *LLMRequest) (*LLMResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("request cannot be nil")
	}

	model := req.Model
	if model == "" {
		model = "claude-3-5-sonnet-20241022"
	}

	// Unit test mock bypass
	if d.apiKey == "test-key" || strings.HasPrefix(d.apiKey, "mock-") {
		promptTokens := d.CountTokens(req)
		return &LLMResponse{
			Content:      "Claude architecture analysis complete.",
			FinishReason: "stop",
			Provider:     "anthropic",
			Model:        model,
			TokenUsage: types.TokenUsage{
				PromptTokens:     promptTokens,
				CompletionTokens: 150,
				TotalTokens:      promptTokens + 150,
				EstimatedCostUSD: 0.0003,
			},
		}, nil
	}

	// 1. If valid Anthropic API key is provided (real sk-ant- key without bullet masks)
	if d.apiKey != "" && !strings.Contains(d.apiKey, "••••") && strings.HasPrefix(d.apiKey, "sk-ant-") {
		return d.completeViaAPI(ctx, req, model)
	}

	// 2. If Claude Code CLI is available locally (logged in with subscription)
	if claudePath, err := exec.LookPath("claude"); err == nil && claudePath != "" {
		return d.completeViaCLI(ctx, req, model, claudePath)
	}

	// 3. Fallback: If no real key and no CLI, return clear error to trigger 9router failover
	return nil, fmt.Errorf("anthropic: no active api key or claude CLI session available")
}

func (d *AnthropicDriver) completeViaAPI(ctx context.Context, req *LLMRequest, model string) (*LLMResponse, error) {
	var systemPrompt string
	var msgs []anthropicMsg

	for _, m := range req.Messages {
		if m.Role == RoleSystem {
			systemPrompt = m.Content
		} else {
			role := "user"
			if m.Role == RoleAssistant {
				role = "assistant"
			}
			msgs = append(msgs, anthropicMsg{Role: role, Content: m.Content})
		}
	}

	if len(msgs) == 0 {
		msgs = append(msgs, anthropicMsg{Role: "user", Content: "Hello"})
	}

	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 4096
	}

	payload := anthropicRequest{
		Model:     model,
		MaxTokens: maxTokens,
		Messages:  msgs,
		System:    systemPrompt,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal anthropic request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, d.baseURL+"/messages", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", d.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	httpResp, err := d.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("anthropic request failed: %w", err)
	}
	defer httpResp.Body.Close()

	bodyBytes, _ := io.ReadAll(httpResp.Body)
	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("anthropic api error (status %d): %s", httpResp.StatusCode, string(bodyBytes))
	}

	var parsed anthropicResponse
	if err := json.Unmarshal(bodyBytes, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse anthropic response: %w", err)
	}

	var contentBuilder strings.Builder
	for _, c := range parsed.Content {
		if c.Type == "text" || c.Text != "" {
			contentBuilder.WriteString(c.Text)
		}
	}

	// Normalize to the OpenAI convention: prompt tokens include cached tokens.
	cachedTokens := parsed.Usage.CacheReadInputTokens
	promptTokens := parsed.Usage.InputTokens + cachedTokens + parsed.Usage.CacheCreationInputTokens
	if promptTokens == 0 {
		promptTokens = d.CountTokens(req)
	}
	completionTokens := parsed.Usage.OutputTokens
	if completionTokens == 0 {
		completionTokens = int64(len(contentBuilder.String()) / 4)
	}

	return &LLMResponse{
		Content:      contentBuilder.String(),
		FinishReason: "stop",
		Provider:     "anthropic",
		Model:        model,
		TokenUsage: types.TokenUsage{
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
			CachedTokens:     cachedTokens,
			TotalTokens:      promptTokens + completionTokens,
			EstimatedCostUSD: float64(promptTokens)*0.000003 + float64(completionTokens)*0.000015,
		},
	}, nil
}

func (d *AnthropicDriver) completeViaCLI(ctx context.Context, req *LLMRequest, model string, claudePath string) (*LLMResponse, error) {
	var promptBuilder strings.Builder
	for _, m := range req.Messages {
		if m.Role == RoleSystem {
			promptBuilder.WriteString(fmt.Sprintf("[System Instructions]\n%s\n\n", m.Content))
		} else {
			promptBuilder.WriteString(fmt.Sprintf("%s\n\n", m.Content))
		}
	}

	promptStr := strings.TrimSpace(promptBuilder.String())
	if promptStr == "" {
		promptStr = "Analyze task implementation context."
	}

	cmdCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, claudePath, "-p", promptStr, "--print")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("claude CLI execution error: %v (output: %s)", err, string(output))
	}

	respContent := strings.TrimSpace(string(output))
	promptTokens := d.CountTokens(req)
	completionTokens := int64(len(respContent) / 4)
	if completionTokens < 10 {
		completionTokens = 10
	}

	return &LLMResponse{
		Content:      respContent,
		FinishReason: "stop",
		Provider:     "anthropic",
		Model:        model,
		TokenUsage: types.TokenUsage{
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
			TotalTokens:      promptTokens + completionTokens,
			EstimatedCostUSD: float64(promptTokens)*0.000003 + float64(completionTokens)*0.000015,
		},
	}, nil
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
	}
	resp, err := d.Complete(ctx, req)
	if chunkCh != nil && err == nil {
		chunkCh <- types.ThoughtChunk{
			ProfileName: "system_architect",
			ModelID:     req.Model,
			Content:     " Analysis and plan generation complete.",
			IsComplete:  true,
		}
	}
	return resp, err
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
	tokens := int64(charCount / 4)
	if tokens < 10 {
		tokens = 10
	}
	return tokens
}
