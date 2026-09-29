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
