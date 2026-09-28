package llm

import (
	"bufio"
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

// Stream event types emitted by StreamActivity.
const (
	StreamTextDelta     = "text_delta"
	StreamThinkingDelta = "thinking_delta"
	StreamToolUse       = "tool_use"
	StreamToolResult    = "tool_result"
	StreamSession       = "session" // session/model identity reported by the provider
)

// StreamEvent is one real-time increment of a model turn.
type StreamEvent struct {
	Type      string                 `json:"type"`
	Text      string                 `json:"text,omitempty"`
	ToolID    string                 `json:"tool_id,omitempty"`
	ToolName  string                 `json:"tool_name,omitempty"`
	ToolInput map[string]interface{} `json:"tool_input,omitempty"`
	IsError   bool                   `json:"is_error,omitempty"`
	SessionID string                 `json:"session_id,omitempty"`
	Model     string                 `json:"model,omitempty"`
}

// ActivityStreamer is implemented by drivers that can stream a turn incrementally.
type ActivityStreamer interface {
	StreamActivity(ctx context.Context, req *LLMRequest, emit func(StreamEvent)) (*LLMResponse, error)
}

// StreamActivity streams a turn when the client supports it; otherwise it completes the request
// and reports the whole response as a single text event (no simulated increments).
func StreamActivity(ctx context.Context, client ProviderClient, req *LLMRequest, emit func(StreamEvent)) (*LLMResponse, error) {
	if emit == nil {
		emit = func(StreamEvent) {}
	}
	if s, ok := client.(ActivityStreamer); ok {
		return s.StreamActivity(ctx, req, emit)
	}
	resp, err := client.Complete(ctx, req)
	if err != nil {
		return nil, err
	}
	for _, tc := range resp.ToolCalls {
		emit(StreamEvent{Type: StreamToolUse, ToolID: tc.ID, ToolName: tc.Name, ToolInput: tc.Arguments})
	}
	if resp.Content != "" {
		emit(StreamEvent{Type: StreamTextDelta, Text: resp.Content})
	}
	return resp, nil
}

// StreamWithFallbackChain is ExecuteWithFallbackChain with streaming. Failover only happens
// before the first event of an attempt is emitted, so the caller never sees two mixed answers.
func (f *ClientFactory) StreamWithFallbackChain(
	ctx context.Context,
	models []string,
	req *LLMRequest,
	emit func(StreamEvent),
	onFailover func(failedModel string, nextModel string, err error),
) (*LLMResponse, string, error) {
	if len(models) == 0 {
		return nil, "", fmt.Errorf("no models provided in priority chain")
	}
	var errList []string
	for i, modelStr := range models {
		client, modelID, err := f.GetClient(modelStr)
		if err == nil {
			attempt := *req
			attempt.Model = modelID
			emitted := false
			resp, streamErr := StreamActivity(ctx, client, &attempt, func(ev StreamEvent) {
				emitted = true
				if emit != nil {
					emit(ev)
				}
			})
			if streamErr == nil {
				f.mu.RLock()
				obs := f.usageObs
				f.mu.RUnlock()
				if obs != nil && resp != nil {
					obs(modelStr, resp)
				}
				return resp, modelStr, nil
			}
			if emitted || ctx.Err() != nil {
				return nil, modelStr, streamErr
			}
			err = streamErr
		}
		errList = append(errList, fmt.Sprintf("%s (%v)", modelStr, err))
		if i < len(models)-1 && onFailover != nil {
			onFailover(modelStr, models[i+1], err)
		}
	}
	return nil, "", fmt.Errorf("all models in 9router priority chain failed: %s", strings.Join(errList, "; "))
}

// ---------------------------------------------------------------------------------------------
// Anthropic: Claude Code CLI (stream-json) and Messages API (SSE)

// SetCLIPath pins the Claude Code CLI binary (tests, custom installs). Empty uses $PATH lookup.
func (d *AnthropicDriver) SetCLIPath(path string) { d.cliPath = path }

func (d *AnthropicDriver) resolveCLI() string {
	if d.cliPath != "" {
		return d.cliPath
	}
	if p, err := exec.LookPath("claude"); err == nil {
		return p
	}
	return ""
}

// StreamActivity streams via the Messages API when an API key is configured, otherwise via the
// local Claude Code CLI.
func (d *AnthropicDriver) StreamActivity(ctx context.Context, req *LLMRequest, emit func(StreamEvent)) (*LLMResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("request cannot be nil")
	}
	if d.apiKey == "test-key" || strings.HasPrefix(d.apiKey, "mock-") {
		resp, err := d.Complete(ctx, req)
		if err == nil {
			emit(StreamEvent{Type: StreamTextDelta, Text: resp.Content})
		}
		return resp, err
	}
	model := req.Model
	if model == "" {
		model = "claude-3-5-sonnet-20241022"
	}
	if d.apiKey != "" && !strings.Contains(d.apiKey, "••••") && strings.HasPrefix(d.apiKey, "sk-ant-") {
		return d.streamViaAPI(ctx, req, model, emit)
	}
	if cli := d.resolveCLI(); cli != "" {
		return d.streamViaCLI(ctx, req, cli, emit)
	}
	return nil, fmt.Errorf("anthropic: no active api key or claude CLI session available")
}

// cliPrompt splits messages into the CLI's system prompt and the new user turn. When resuming a
// session the CLI already holds prior turns, so only the latest user message is sent.
func cliPrompt(req *LLMRequest) (system string, prompt string) {
	var sys []string
	var turns []string
	lastUser := ""
	for _, m := range req.Messages {
		switch m.Role {
		case RoleSystem:
			sys = append(sys, m.Content)
		case RoleAssistant:
			turns = append(turns, "[Assistant]\n"+m.Content)
		default:
			turns = append(turns, m.Content)
			lastUser = m.Content
		}
	}
	system = strings.Join(sys, "\n\n")
	if req.SessionID != "" && lastUser != "" {
		return system, lastUser
	}
	return system, strings.TrimSpace(strings.Join(turns, "\n\n"))
}

type cliStreamLine struct {
	Type      string          `json:"type"`
	Subtype   string          `json:"subtype"`
	SessionID string          `json:"session_id"`
	Model     string          `json:"model"`
	Message   json.RawMessage `json:"message"`
	Event     json.RawMessage `json:"event"`
	Result    string          `json:"result"`
	IsError   bool            `json:"is_error"`
	TotalCost float64         `json:"total_cost_usd"`
	Usage     *anthropicUsage `json:"usage"`
}

type anthropicUsage struct {
	InputTokens              int64 `json:"input_tokens"`
	OutputTokens             int64 `json:"output_tokens"`
	CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
}

func (u anthropicUsage) tokenUsage() types.TokenUsage {
	prompt := u.InputTokens + u.CacheReadInputTokens + u.CacheCreationInputTokens
	return types.TokenUsage{PromptTokens: prompt, CompletionTokens: u.OutputTokens, CachedTokens: u.CacheReadInputTokens,
		TotalTokens: prompt + u.OutputTokens}
}

type contentBlock struct {
	Type      string                 `json:"type"`
	Text      string                 `json:"text"`
	Thinking  string                 `json:"thinking"`
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Input     map[string]interface{} `json:"input"`
	ToolUseID string                 `json:"tool_use_id"`
	Content   json.RawMessage        `json:"content"`
	IsError   bool                   `json:"is_error"`
}

type messageEnvelope struct {
	Model   string         `json:"model"`
	Content []contentBlock `json:"content"`
}

// toolResultText flattens a tool_result content field (string or array of text blocks).
func toolResultText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var blocks []contentBlock
	if json.Unmarshal(raw, &blocks) == nil {
		var parts []string
		for _, b := range blocks {
			if b.Text != "" {
				parts = append(parts, b.Text)
			}
		}
		return strings.Join(parts, "\n")
	}
	return string(raw)
}

type sseDelta struct {
	Type        string `json:"type"`
	Text        string `json:"text"`
	Thinking    string `json:"thinking"`
	PartialJSON string `json:"partial_json"`
	StopReason  string `json:"stop_reason"`
}

type sseEvent struct {
	Type         string       `json:"type"`
	Index        int          `json:"index"`
	Delta        sseDelta     `json:"delta"`
	ContentBlock contentBlock `json:"content_block"`
	Message      *struct {
		Model string         `json:"model"`
		Usage anthropicUsage `json:"usage"`
	} `json:"message"`
	Usage *anthropicUsage `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (d *AnthropicDriver) streamViaCLI(ctx context.Context, req *LLMRequest, cli string, emit func(StreamEvent)) (*LLMResponse, error) {
	system, prompt := cliPrompt(req)
	if prompt == "" {
		prompt = "Continue."
	}
	args := []string{"-p", prompt, "--output-format", "stream-json", "--verbose", "--include-partial-messages"}
	if req.SessionID != "" {
		args = append(args, "--resume", req.SessionID)
	}
	if system != "" {
		args = append(args, "--append-system-prompt", system)
	}
	mode := req.PermissionMode
	if mode == "" {
		mode = "plan" // read-only unless the caller explicitly grants more
	}
	args = append(args, "--permission-mode", mode)

	timeout := req.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(cmdCtx, cli, args...)
	if req.WorkDir != "" {
		cmd.Dir = req.WorkDir
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("claude CLI start: %w", err)
	}

	start := time.Now()
	resp := &LLMResponse{Provider: "anthropic", Model: "claude-code", FinishReason: "stop"}
	var text strings.Builder
	partialText := false
	gotResult := false

	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for sc.Scan() {
		var line cliStreamLine
		if json.Unmarshal(sc.Bytes(), &line) != nil {
			continue
		}
		switch line.Type {
		case "system":
			if line.SessionID != "" {
				resp.SessionID = line.SessionID
			}
			if line.Model != "" {
				resp.Model = line.Model
			}
			if line.Subtype == "init" {
				emit(StreamEvent{Type: StreamSession, SessionID: line.SessionID, Model: line.Model})
			}
		case "stream_event":
			var ev sseEvent
			if json.Unmarshal(line.Event, &ev) == nil && ev.Type == "content_block_delta" {
				switch ev.Delta.Type {
				case "text_delta":
					partialText = true
					text.WriteString(ev.Delta.Text)
					emit(StreamEvent{Type: StreamTextDelta, Text: ev.Delta.Text})
				case "thinking_delta":
					emit(StreamEvent{Type: StreamThinkingDelta, Text: ev.Delta.Thinking})
				}
			}
		case "assistant":
			var msg messageEnvelope
			if json.Unmarshal(line.Message, &msg) != nil {
				continue
			}
			if msg.Model != "" {
				resp.Model = msg.Model
			}
			for _, b := range msg.Content {
				switch b.Type {
				case "text":
					if !partialText && b.Text != "" {
						text.WriteString(b.Text)
						emit(StreamEvent{Type: StreamTextDelta, Text: b.Text})
					}
				case "tool_use":
					resp.ToolCalls = append(resp.ToolCalls, ToolCall{ID: b.ID, Name: b.Name, Arguments: b.Input})
					emit(StreamEvent{Type: StreamToolUse, ToolID: b.ID, ToolName: b.Name, ToolInput: b.Input})
				}
			}
			partialText = false
		case "user":
			var msg messageEnvelope
			if json.Unmarshal(line.Message, &msg) != nil {
				continue
			}
			for _, b := range msg.Content {
				if b.Type == "tool_result" {
					emit(StreamEvent{Type: StreamToolResult, ToolID: b.ToolUseID, Text: toolResultText(b.Content), IsError: b.IsError})
				}
			}
		case "result":
			gotResult = true
			if line.SessionID != "" {
				resp.SessionID = line.SessionID
			}
			if line.Usage != nil {
				resp.TokenUsage = line.Usage.tokenUsage()
			}
			resp.TokenUsage.EstimatedCostUSD = line.TotalCost
			if line.IsError || (line.Subtype != "" && line.Subtype != "success") {
				resp.FinishReason = "error"
				if line.Result != "" {
					resp.Content = line.Result
				}
			} else if line.Result != "" {
				resp.Content = line.Result
			}
		}
	}
	waitErr := cmd.Wait()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if resp.Content == "" {
		resp.Content = text.String()
	}
	if waitErr != nil && !gotResult {
		return nil, fmt.Errorf("claude CLI: %v: %s", waitErr, strings.TrimSpace(stderr.String()))
	}
	if resp.FinishReason == "error" {
		return resp, fmt.Errorf("claude CLI turn failed: %s", firstLine(resp.Content))
	}
	if resp.TokenUsage.TotalTokens == 0 {
		resp.TokenUsage.PromptTokens = d.CountTokens(req)
		resp.TokenUsage.CompletionTokens = int64(len(resp.Content) / 4)
		resp.TokenUsage.TotalTokens = resp.TokenUsage.PromptTokens + resp.TokenUsage.CompletionTokens
	}
	resp.DurationMS = time.Since(start).Milliseconds()
	return resp, nil
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func (d *AnthropicDriver) streamViaAPI(ctx context.Context, req *LLMRequest, model string, emit func(StreamEvent)) (*LLMResponse, error) {
	var system string
	var msgs []anthropicMsg
	for _, m := range req.Messages {
		if m.Role == RoleSystem {
			system = m.Content
			continue
		}
		role := "user"
		if m.Role == RoleAssistant {
			role = "assistant"
		}
		msgs = append(msgs, anthropicMsg{Role: role, Content: m.Content})
	}
	if len(msgs) == 0 {
		msgs = append(msgs, anthropicMsg{Role: "user", Content: "Hello"})
	}
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 4096
	}
	payload := map[string]interface{}{"model": model, "max_tokens": maxTokens, "messages": msgs, "stream": true}
	if system != "" {
		payload["system"] = system
	}
	data, _ := json.Marshal(payload)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, d.baseURL+"/messages", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("x-api-key", d.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	start := time.Now()
	httpResp, err := (&http.Client{}).Do(httpReq) // no client timeout: the context bounds streaming
	if err != nil {
		return nil, fmt.Errorf("anthropic request failed: %w", err)
	}
	defer httpResp.Body.Close()
	if httpResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(httpResp.Body, 4096))
		return nil, fmt.Errorf("anthropic api error (status %d): %s", httpResp.StatusCode, string(body))
	}

	resp := &LLMResponse{Provider: "anthropic", Model: model, FinishReason: "stop"}
	var text strings.Builder
	var usage anthropicUsage
	type toolAcc struct {
		id, name string
		json     strings.Builder
	}
	tools := map[int]*toolAcc{}
	err = readSSE(httpResp.Body, func(data []byte) bool {
		var ev sseEvent
		if json.Unmarshal(data, &ev) != nil {
			return true
		}
		switch ev.Type {
		case "message_start":
			if ev.Message != nil {
				usage = ev.Message.Usage
				if ev.Message.Model != "" {
					resp.Model = ev.Message.Model
				}
			}
		case "content_block_start":
			if ev.ContentBlock.Type == "tool_use" {
				tools[ev.Index] = &toolAcc{id: ev.ContentBlock.ID, name: ev.ContentBlock.Name}
			}
		case "content_block_delta":
			switch ev.Delta.Type {
			case "text_delta":
				text.WriteString(ev.Delta.Text)
				emit(StreamEvent{Type: StreamTextDelta, Text: ev.Delta.Text})
			case "thinking_delta":
				emit(StreamEvent{Type: StreamThinkingDelta, Text: ev.Delta.Thinking})
			case "input_json_delta":
				if t := tools[ev.Index]; t != nil {
					t.json.WriteString(ev.Delta.PartialJSON)
				}
			}
		case "content_block_stop":
			if t := tools[ev.Index]; t != nil {
				input := map[string]interface{}{}
				_ = json.Unmarshal([]byte(t.json.String()), &input)
				resp.ToolCalls = append(resp.ToolCalls, ToolCall{ID: t.id, Name: t.name, Arguments: input})
				emit(StreamEvent{Type: StreamToolUse, ToolID: t.id, ToolName: t.name, ToolInput: input})
				delete(tools, ev.Index)
			}
		case "message_delta":
			if ev.Delta.StopReason != "" {
				resp.FinishReason = ev.Delta.StopReason
			}
			if ev.Usage != nil {
				usage.OutputTokens = ev.Usage.OutputTokens
			}
		case "error":
			if ev.Error != nil {
				resp.FinishReason = "error"
				resp.Content = ev.Error.Message
				return false
			}
		}
		return true
	})
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, err
	}
	if resp.FinishReason == "error" {
		return nil, fmt.Errorf("anthropic stream error: %s", resp.Content)
	}
	resp.Content = text.String()
	resp.TokenUsage = usage.tokenUsage()
	resp.DurationMS = time.Since(start).Milliseconds()
	return resp, nil
}

// readSSE calls fn with each `data:` payload until the stream ends, fn returns false, or [DONE].
func readSSE(r io.Reader, fn func(data []byte) bool) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		data := bytes.TrimSpace(line[len("data:"):])
		if bytes.Equal(data, []byte("[DONE]")) {
			return nil
		}
		if !fn(data) {
			return nil
		}
	}
	return sc.Err()
}

// ---------------------------------------------------------------------------------------------
// OpenAI: Chat Completions (SSE)

type openAIStreamChunk struct {
	Model   string `json:"model"`
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int64 `json:"prompt_tokens"`
		CompletionTokens int64 `json:"completion_tokens"`
		PromptDetails    struct {
			CachedTokens int64 `json:"cached_tokens"`
		} `json:"prompt_tokens_details"`
	} `json:"usage"`
}

// StreamActivity streams an OpenAI chat completion.
func (d *OpenAIDriver) StreamActivity(ctx context.Context, req *LLMRequest, emit func(StreamEvent)) (*LLMResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("request cannot be nil")
	}
	if d.apiKey == "test-key" || strings.HasPrefix(d.apiKey, "mock-") || d.apiKey == "" ||
		strings.Contains(d.apiKey, "••••") || !strings.HasPrefix(d.apiKey, "sk-") {
		resp, err := d.Complete(ctx, req)
		if err == nil {
			emit(StreamEvent{Type: StreamTextDelta, Text: resp.Content})
		}
		return resp, err
	}
	model := req.Model
	if model == "" {
		model = "gpt-4o"
	}
	var msgs []openAIMsg
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
	payload := map[string]interface{}{"model": model, "messages": msgs, "stream": true,
		"stream_options": map[string]bool{"include_usage": true}}
	data, _ := json.Marshal(payload)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, d.baseURL+"/chat/completions", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("Authorization", "Bearer "+d.apiKey)

	start := time.Now()
	httpResp, err := (&http.Client{}).Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openai request failed: %w", err)
	}
	defer httpResp.Body.Close()
	if httpResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(httpResp.Body, 4096))
		return nil, fmt.Errorf("openai api error (status %d): %s", httpResp.StatusCode, string(body))
	}
	resp := &LLMResponse{Provider: "openai", Model: model, FinishReason: "stop"}
	var text strings.Builder
	err = readSSE(httpResp.Body, func(data []byte) bool {
		var c openAIStreamChunk
		if json.Unmarshal(data, &c) != nil {
			return true
		}
		if c.Model != "" {
			resp.Model = c.Model
		}
		for _, ch := range c.Choices {
			if ch.Delta.Content != "" {
				text.WriteString(ch.Delta.Content)
				emit(StreamEvent{Type: StreamTextDelta, Text: ch.Delta.Content})
			}
			if ch.FinishReason != "" {
				resp.FinishReason = ch.FinishReason
			}
		}
		if c.Usage != nil {
			resp.TokenUsage = types.TokenUsage{PromptTokens: c.Usage.PromptTokens, CompletionTokens: c.Usage.CompletionTokens,
				CachedTokens: c.Usage.PromptDetails.CachedTokens, TotalTokens: c.Usage.PromptTokens + c.Usage.CompletionTokens}
		}
		return true
	})
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, err
	}
	resp.Content = text.String()
	if resp.TokenUsage.TotalTokens == 0 {
		resp.TokenUsage.PromptTokens = d.CountTokens(req)
		resp.TokenUsage.CompletionTokens = int64(len(resp.Content) / 4)
		resp.TokenUsage.TotalTokens = resp.TokenUsage.PromptTokens + resp.TokenUsage.CompletionTokens
	}
	resp.DurationMS = time.Since(start).Milliseconds()
	return resp, nil
}
