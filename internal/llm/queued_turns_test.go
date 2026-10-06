package llm

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	turnEndGrace = 200 * time.Millisecond // keep fake CLIs from idling after their last result
	os.Exit(m.Run())
}

// A resumed session first runs a turn about background work that was cut off, then the turn for
// the prompt, which still needs to ask for permission.
func TestPermissionChannelStaysOpenForAQueuedTurn(t *testing.T) {
	old := turnEndGrace
	turnEndGrace = 2 * time.Second
	defer func() { turnEndGrace = old }()

	cli := filepath.Join(t.TempDir(), "claude")
	script := `#!/bin/sh
read init; read user
echo '{"type":"system","subtype":"init","session_id":"s1"}'
echo '{"type":"result","subtype":"success","result":"background agents were stopped","session_id":"s1"}'
sleep 0.3
echo '{"type":"system","subtype":"init","session_id":"s1"}'
echo '{"type":"control_request","request_id":"r1","request":{"subtype":"can_use_tool","tool_name":"Bash","tool_use_id":"t1","input":{"command":"git -C repo log"}}}'
if read answer; then
  case "$answer" in
    *'"behavior":"allow"'*) echo '{"type":"result","subtype":"success","result":"ALLOWED","session_id":"s1"}';;
    *) echo '{"type":"result","subtype":"success","result":"DENIED","session_id":"s1"}';;
  esac
else
  echo '{"type":"result","subtype":"success","result":"Tool permission request failed: AbortError: Stream closed","session_id":"s1"}'
fi
`
	if err := os.WriteFile(cli, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	d := NewAnthropicDriver("", "")
	d.SetCLIPath(cli)
	asked := false
	resp, err := d.StreamActivity(context.Background(), &LLMRequest{Messages: []Message{{Role: RoleUser, Content: "continue"}},
		PermissionMode: "acceptEdits", Approver: func(context.Context, ApprovalRequest) ApprovalDecision {
			asked = true
			return ApprovalDecision{Allow: true}
		}}, func(StreamEvent) {})
	if err != nil {
		t.Fatal(err)
	}
	if !asked || resp.Content != "ALLOWED" {
		t.Fatalf("queued turn lost its permission channel: asked=%v content=%q", asked, resp.Content)
	}
}
