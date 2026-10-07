package llm

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/config"
)

func collect(evs *[]StreamEvent) func(StreamEvent) {
	return func(ev StreamEvent) { *evs = append(*evs, ev) }
}

// fakeClaudeCLI writes a shell script that records its argv and working dir, then replays a
// stream-json transcript shaped like Claude Code's --output-format stream-json output.
func fakeClaudeCLI(t *testing.T, exitCode int) (cli string, argsFile string) {
	dir := t.TempDir()
	argsFile = filepath.Join(dir, "args.txt")
	lines := []string{
		`{"type":"system","subtype":"init","session_id":"11111111-2222-3333-4444-555555555555","model":"claude-sonnet-x"}`,
		`{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"checking the handler"}}}`,
		`{"type":"stream_event","event":{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"Reading "}}}`,
		`{"type":"stream_event","event":{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"the file."}}}`,
		`{"type":"assistant","message":{"model":"claude-sonnet-x","content":[{"type":"text","text":"Reading the file."},{"type":"tool_use","id":"toolu_1","name":"Read","input":{"file_path":"main.go"}}]}}`,
		`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_1","content":[{"type":"text","text":"package main"}],"is_error":false}]}}`,
		`{"type":"assistant","message":{"content":[{"type":"text","text":" Done."}]}}`,
		`{"type":"result","subtype":"success","result":"Reading the file. Done.","session_id":"11111111-2222-3333-4444-555555555555","total_cost_usd":0.0123,"usage":{"input_tokens":100,"output_tokens":40,"cache_read_input_tokens":900,"cache_creation_input_tokens":0}}`,
	}
	script := "#!/bin/sh\nprintf '%s\\n' \"$PWD\" > " + argsFile + "\nfor a in \"$@\"; do printf '%s\\n' \"$a\" >> " + argsFile + "; done\ncat <<'EOF'\n" +
		strings.Join(lines, "\n") + "\nEOF\n" + fmt.Sprintf("exit %d\n", exitCode)
	cli = filepath.Join(dir, "claude")
	if err := os.WriteFile(cli, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return cli, argsFile
}

func TestClaudeCLIStreamActivity(t *testing.T) {
	cli, argsFile := fakeClaudeCLI(t, 0)
	d := NewAnthropicDriver("", "")
	d.SetCLIPath(cli)
	work := t.TempDir()

	var evs []StreamEvent
	resp, err := d.StreamActivity(context.Background(), &LLMRequest{
		Messages: []Message{{Role: RoleSystem, Content: "You are the task agent."}, {Role: RoleUser, Content: "old turn"},
			{Role: RoleAssistant, Content: "old answer"}, {Role: RoleUser, Content: "why does it fail?"}},
		SessionID: "11111111-2222-3333-4444-555555555555", WorkDir: work,
	}, collect(&evs))
	if err != nil {
		t.Fatal(err)
	}

	kinds := []string{}
	for _, e := range evs {
		kinds = append(kinds, e.Type)
	}
	want := []string{StreamSession, StreamThinkingDelta, StreamTextDelta, StreamTextDelta, StreamToolUse, StreamToolResult, StreamTextDelta}
	if strings.Join(kinds, ",") != strings.Join(want, ",") {
		t.Fatalf("events = %v, want %v", kinds, want)
	}
	if evs[4].ToolName != "Read" || evs[4].ToolInput["file_path"] != "main.go" || evs[5].ToolID != "toolu_1" || evs[5].Text != "package main" {
		t.Fatalf("tool events = %+v / %+v", evs[4], evs[5])
	}
	// The second assistant message had no partial deltas, so its text is emitted once from the full message.
	if evs[6].Text != " Done." {
		t.Fatalf("non-partial text = %q", evs[6].Text)
	}
	if resp.Content != "Reading the file. Done." || resp.SessionID == "" || resp.Model != "claude-sonnet-x" {
		t.Fatalf("resp = %+v", resp)
	}
	u := resp.TokenUsage
	if u.PromptTokens != 1000 || u.CachedTokens != 900 || u.CompletionTokens != 40 || u.EstimatedCostUSD != 0.0123 {
		t.Fatalf("usage = %+v", u)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("tool calls = %+v", resp.ToolCalls)
	}

	raw, _ := os.ReadFile(argsFile)
	argv := strings.Split(strings.TrimSpace(string(raw)), "\n")
	realWork, _ := filepath.EvalSymlinks(work)
	if gotWD, _ := filepath.EvalSymlinks(argv[0]); gotWD != realWork {
		t.Fatalf("cwd = %s, want %s", argv[0], work)
	}
	joined := strings.Join(argv[1:], " ")
	for _, want := range []string{"-p why does it fail?", "--output-format stream-json", "--verbose", "--include-partial-messages",
		"--resume 11111111-2222-3333-4444-555555555555", "--append-system-prompt You are the task agent.", "--permission-mode plan"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("argv missing %q: %v", want, argv[1:])
		}
	}
	if strings.Contains(joined, "old turn") {
		t.Fatal("resumed session must only send the latest user message")
	}
}

func TestClaudeCLIErrorResultFails(t *testing.T) {
	dir := t.TempDir()
	cli := filepath.Join(dir, "claude")
	_ = os.WriteFile(cli, []byte("#!/bin/sh\necho '{\"type\":\"result\",\"subtype\":\"error_max_turns\",\"is_error\":true,\"result\":\"Reached max turns\"}'\n"), 0o755)
	d := NewAnthropicDriver("", "")
	d.SetCLIPath(cli)
	if _, err := d.StreamActivity(context.Background(), &LLMRequest{Messages: []Message{{Role: RoleUser, Content: "x"}}}, func(StreamEvent) {}); err == nil ||
		!strings.Contains(err.Error(), "Reached max turns") {
		t.Fatalf("err = %v", err)
	}
}

// A UserPromptSubmit hook that refuses the prompt comes back as a "successful" result whose text is
// the refusal; the turn never reached the model, so it must fail — without echoing the prompt.
func TestClaudeCLIHookBlockedPromptFails(t *testing.T) {
	dir := t.TempDir()
	cli := filepath.Join(dir, "claude")
	lines := []string{
		`{"type":"system","subtype":"init","session_id":"s1","model":"claude-sonnet-x"}`,
		`{"type":"system","subtype":"informational","content":"UserPromptSubmit operation blocked by hook:\nPII Shield blocked this prompt\n\nOriginal prompt: mail svc@example.iam","level":"warning","prevent_continuation":true}`,
		`{"type":"result","subtype":"success","is_error":false,"num_turns":0,"result":"UserPromptSubmit operation blocked by hook:\nPII Shield blocked this prompt\n\nOriginal prompt: mail svc@example.iam","session_id":"s1","total_cost_usd":0,"usage":{"input_tokens":0,"output_tokens":0}}`,
	}
	_ = os.WriteFile(cli, []byte("#!/bin/sh\ncat <<'EOF'\n"+strings.Join(lines, "\n")+"\nEOF\n"), 0o755)
	d := NewAnthropicDriver("", "")
	d.SetCLIPath(cli)
	_, err := d.StreamActivity(context.Background(), &LLMRequest{Messages: []Message{{Role: RoleUser, Content: "x"}}}, func(StreamEvent) {})
	if !errors.Is(err, ErrPromptBlocked) || !strings.Contains(err.Error(), "PII Shield") || strings.Contains(err.Error(), "svc@example.iam") {
		t.Fatalf("err = %v", err)
	}
}

func TestAnthropicAPIStreamSSE(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for _, ev := range []string{
			`{"type":"message_start","message":{"model":"claude-x","usage":{"input_tokens":50,"cache_read_input_tokens":10,"output_tokens":1}}}`,
			`{"type":"content_block_start","index":0,"content_block":{"type":"text"}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hel"}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"lo"}}`,
			`{"type":"content_block_stop","index":0}`,
			`{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"tu_9","name":"get_weather"}}`,
			`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"city\":"}}`,
			`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"\"Jakarta\"}"}}`,
			`{"type":"content_block_stop","index":1}`,
			`{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":22}}`,
			`{"type":"message_stop"}`,
		} {
			fmt.Fprintf(w, "event: x\ndata: %s\n\n", ev)
		}
	}))
	defer srv.Close()
	// Placeholder credential for a local httptest server; not a real key.
	d := NewAnthropicDriver("sk-ant-placeholder-for-httptest", srv.URL)
	var evs []StreamEvent
	resp, err := d.StreamActivity(context.Background(), &LLMRequest{Messages: []Message{{Role: RoleUser, Content: "hi"}}}, collect(&evs))
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 3 || evs[0].Text != "Hel" || evs[2].Type != StreamToolUse || evs[2].ToolInput["city"] != "Jakarta" {
		t.Fatalf("events = %+v", evs)
	}
	if resp.Content != "Hello" || resp.FinishReason != "tool_use" || resp.TokenUsage.PromptTokens != 60 ||
		resp.TokenUsage.CachedTokens != 10 || resp.TokenUsage.CompletionTokens != 22 || resp.Model != "claude-x" {
		t.Fatalf("resp = %+v", resp)
	}
}

func TestOpenAIStreamSSE(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"model\":\"gpt-x\",\"choices\":[{\"delta\":{\"content\":\"A\"}}]}\n\n")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"B\"},\"finish_reason\":\"stop\"}]}\n\n")
		fmt.Fprint(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":7,\"completion_tokens\":2,\"prompt_tokens_details\":{\"cached_tokens\":3}}}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()
	// Placeholder credential for a local httptest server; not a real key.
	d := NewOpenAIDriver("sk-placeholder-for-httptest", srv.URL)
	var evs []StreamEvent
	resp, err := d.StreamActivity(context.Background(), &LLMRequest{Messages: []Message{{Role: RoleUser, Content: "hi"}}}, collect(&evs))
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 2 || resp.Content != "AB" || resp.TokenUsage.CachedTokens != 3 || resp.TokenUsage.TotalTokens != 9 || resp.Model != "gpt-x" {
		t.Fatalf("evs=%+v resp=%+v", evs, resp)
	}
}

type failingClient struct{ fixedClient }

func (failingClient) Complete(ctx context.Context, req *LLMRequest) (*LLMResponse, error) {
	return nil, fmt.Errorf("provider down")
}

func TestStreamWithFallbackChainFailsOverAndFallsBackToComplete(t *testing.T) {
	f := NewClientFactory(config.GetDefaultConfig())
	f.OverrideProvider("claude", failingClient{})
	f.OverrideProvider("openai", fixedClient{})
	var failovers []string
	var evs []StreamEvent
	resp, used, err := f.StreamWithFallbackChain(context.Background(), []string{"claude/x", "openai/y"},
		&LLMRequest{Messages: []Message{{Role: RoleUser, Content: "hi"}}}, collect(&evs),
		func(failed, next string, err error) { failovers = append(failovers, failed+"->"+next) })
	if err != nil {
		t.Fatal(err)
	}
	if used != "openai/y" || len(failovers) != 1 || resp.Content != "ok" {
		t.Fatalf("used=%s failovers=%v resp=%+v", used, failovers, resp)
	}
	// Non-streaming client: exactly one text event with the full content, nothing invented.
	if len(evs) != 1 || evs[0].Type != StreamTextDelta || evs[0].Text != "ok" {
		t.Fatalf("events = %+v", evs)
	}
}
