package llm

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeInteractiveCLI speaks the host control protocol: it asks for permission to run one Bash
// command and reports whether the host allowed it.
func fakeInteractiveCLI(t *testing.T) (cli, argsFile, stdinFile string) {
	t.Helper()
	dir := t.TempDir()
	argsFile = filepath.Join(dir, "args")
	stdinFile = filepath.Join(dir, "stdin")
	cli = filepath.Join(dir, "claude")
	script := `#!/bin/sh
printf '%s\n' "$@" > '` + argsFile + `'
read init; read user
printf '%s\n%s\n' "$init" "$user" > '` + stdinFile + `'
echo '{"type":"system","subtype":"init","session_id":"s1","model":"claude-x"}'
echo '{"type":"control_request","request_id":"r1","request":{"subtype":"can_use_tool","tool_name":"Bash","tool_use_id":"t1","description":"Run tests","input":{"command":"go test ./..."}}}'
read answer
case "$answer" in
  *'"behavior":"allow"'*) echo '{"type":"result","subtype":"success","result":"ALLOWED","session_id":"s1"}';;
  *) echo '{"type":"result","subtype":"success","result":"DENIED","session_id":"s1"}';;
esac
`
	if err := os.WriteFile(cli, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return cli, argsFile, stdinFile
}

func TestCLIPermissionPromptsReachTheApprover(t *testing.T) {
	for _, allow := range []bool{true, false} {
		cli, argsFile, stdinFile := fakeInteractiveCLI(t)
		d := NewAnthropicDriver("", "")
		d.SetCLIPath(cli)
		var asked ApprovalRequest
		resp, err := d.StreamActivity(context.Background(), &LLMRequest{
			Messages:       []Message{{Role: RoleUser, Content: "fix the bug"}},
			PermissionMode: "acceptEdits",
			Approver: func(ctx context.Context, req ApprovalRequest) ApprovalDecision {
				asked = req
				return ApprovalDecision{Allow: allow, Message: "not now"}
			},
		}, func(StreamEvent) {})
		if err != nil {
			t.Fatal(err)
		}
		want := map[bool]string{true: "ALLOWED", false: "DENIED"}[allow]
		if resp.Content != want {
			t.Fatalf("allow=%v: CLI saw %q", allow, resp.Content)
		}
		if asked.ToolName != "Bash" || asked.Input["command"] != "go test ./..." || asked.Description != "Run tests" {
			t.Fatalf("approval request = %+v", asked)
		}
		args, _ := os.ReadFile(argsFile)
		if !strings.Contains(string(args), "--permission-prompt-tool\nstdio") || !strings.Contains(string(args), "--input-format\nstream-json") {
			t.Fatalf("interactive flags missing:\n%s", args)
		}
		stdin, _ := os.ReadFile(stdinFile)
		if !strings.Contains(string(stdin), `"subtype":"initialize"`) || !strings.Contains(string(stdin), "fix the bug") {
			t.Fatalf("prompt must be sent over stdin after initialize:\n%s", stdin)
		}
	}
}

// fakeCLIRecordingAnswer asks for one tool and writes the host's answer line to a file; it reports
// the call as denied in the result when the host refused it.
func fakeCLIRecordingAnswer(t *testing.T, tool, input string) (cli, answerFile string) {
	t.Helper()
	dir := t.TempDir()
	answerFile = filepath.Join(dir, "answer")
	cli = filepath.Join(dir, "claude")
	script := `#!/bin/sh
read init; read user
echo '{"type":"control_request","request_id":"r1","request":{"subtype":"can_use_tool","tool_name":"` + tool + `","tool_use_id":"t1","input":` + input + `}}'
read answer
printf '%s' "$answer" > '` + answerFile + `'
case "$answer" in
  *'"behavior":"allow"'*) echo '{"type":"result","subtype":"success","result":"ok","session_id":"s1"}';;
  *) echo '{"type":"result","subtype":"success","result":"ok","session_id":"s1","permission_denials":[{"tool_name":"` + tool + `","tool_use_id":"t1","tool_input":` + input + `}]}';;
esac
`
	if err := os.WriteFile(cli, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return cli, answerFile
}

func runFakeCLI(t *testing.T, cli string, d ApprovalDecision) *LLMResponse {
	t.Helper()
	drv := NewAnthropicDriver("", "")
	drv.SetCLIPath(cli)
	resp, err := drv.StreamActivity(context.Background(), &LLMRequest{Messages: []Message{{Role: RoleUser, Content: "go"}}, PermissionMode: "plan",
		Approver: func(context.Context, ApprovalRequest) ApprovalDecision { return d }}, func(StreamEvent) {})
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestApprovedPlanSwitchesTheCLIOutOfPlanMode(t *testing.T) {
	cli, answerFile := fakeCLIRecordingAnswer(t, ToolExitPlanMode, `{"plan":"do it"}`)
	runFakeCLI(t, cli, ApprovalDecision{Allow: true, Mode: "acceptEdits"})
	answer, _ := os.ReadFile(answerFile)
	if !strings.Contains(string(answer), `"updatedPermissions":[{"destination":"session","mode":"acceptEdits","type":"setMode"}]`) {
		t.Fatalf("approving the plan must set the session mode, or the CLI keeps refusing edits:\n%s", answer)
	}

	cli, answerFile = fakeCLIRecordingAnswer(t, "Bash", `{"command":"ls"}`)
	runFakeCLI(t, cli, ApprovalDecision{Allow: true, Mode: "acceptEdits"})
	if answer, _ := os.ReadFile(answerFile); strings.Contains(string(answer), "updatedPermissions") {
		t.Fatalf("only a plan approval changes the mode:\n%s", answer)
	}
}

func TestQuestionAnswersAndDenialsCrossTheWire(t *testing.T) {
	cli, answerFile := fakeCLIRecordingAnswer(t, ToolAskUserQuestion, `{"questions":[{"question":"Tea?"}]}`)
	runFakeCLI(t, cli, ApprovalDecision{Allow: true, UpdatedInput: map[string]interface{}{"questions": []interface{}{}, "answers": map[string]string{"Tea?": "Yes"}}})
	if answer, _ := os.ReadFile(answerFile); !strings.Contains(string(answer), `"updatedInput":{"answers":{"Tea?":"Yes"}`) {
		t.Fatalf("answers must be sent in updatedInput:\n%s", answer)
	}

	cli, _ = fakeCLIRecordingAnswer(t, "Bash", `{"command":"git push"}`)
	resp := runFakeCLI(t, cli, ApprovalDecision{Allow: false})
	if len(resp.PermissionDenials) != 1 || resp.PermissionDenials[0].ToolName != "Bash" || resp.PermissionDenials[0].Input["command"] != "git push" {
		t.Fatalf("denials from the result must be reported: %+v", resp.PermissionDenials)
	}
}

// fakeCLIWithBackgroundAgent ends its main turn while a background agent is running; the agent
// then asks for permission, and the CLI runs a follow-up turn once it is done.
func fakeCLIWithBackgroundAgent(t *testing.T) string {
	t.Helper()
	cli := filepath.Join(t.TempDir(), "claude")
	script := `#!/bin/sh
read init; read user
echo '{"type":"system","subtype":"init","session_id":"s1"}'
echo '{"type":"system","subtype":"background_tasks_changed","tasks":[{"task_id":"a1"}]}'
echo '{"type":"result","subtype":"success","result":"STARTED","session_id":"s1","total_cost_usd":0.1,"usage":{"input_tokens":10,"output_tokens":5}}'
echo '{"type":"control_request","request_id":"r1","request":{"subtype":"can_use_tool","tool_name":"Bash","tool_use_id":"t1","input":{"command":"git commit"}}}'
read answer || { echo '{"type":"result","subtype":"error_during_execution","is_error":true,"result":"Stream closed"}'; exit 1; }
case "$answer" in *'"behavior":"allow"'*) ok=yes;; *) ok=no;; esac
echo '{"type":"system","subtype":"background_tasks_changed","tasks":[]}'
echo '{"type":"system","subtype":"init","session_id":"s1"}'
echo '{"type":"result","subtype":"success","result":"background done, allowed='$ok'","session_id":"s1","total_cost_usd":0.2,"usage":{"input_tokens":20,"output_tokens":7}}'
read eof
`
	if err := os.WriteFile(cli, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return cli
}

func TestBackgroundAgentsKeepThePermissionChannelOpen(t *testing.T) {
	d := NewAnthropicDriver("", "")
	d.SetCLIPath(fakeCLIWithBackgroundAgent(t))
	var background []int
	resp, err := d.StreamActivity(context.Background(), &LLMRequest{Messages: []Message{{Role: RoleUser, Content: "go"}}, PermissionMode: "acceptEdits",
		Approver: func(context.Context, ApprovalRequest) ApprovalDecision { return ApprovalDecision{Allow: true} }},
		func(ev StreamEvent) {
			if ev.Type == StreamBackground {
				background = append(background, ev.Count)
			}
		})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Content != "background done, allowed=yes" {
		t.Fatalf("the background agent's permission request must still be answered after the main result: %q", resp.Content)
	}
	if len(background) != 1 || background[0] != 1 {
		t.Fatalf("the console should hear that background work continues: %v", background)
	}
	if resp.TokenUsage.PromptTokens != 30 || resp.TokenUsage.CompletionTokens != 12 || resp.TokenUsage.EstimatedCostUSD != 0.2 {
		t.Fatalf("tokens should cover both turns and cost be the CLI's running total: %+v", resp.TokenUsage)
	}
}
