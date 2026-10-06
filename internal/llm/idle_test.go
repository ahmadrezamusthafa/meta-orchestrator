package llm

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSilentCLITurnStopsWithIdleError(t *testing.T) {
	cli := filepath.Join(t.TempDir(), "claude")
	script := "#!/bin/sh\necho '{\"type\":\"system\",\"subtype\":\"init\",\"session_id\":\"s1\"}'\nexec sleep 30\n"
	if err := os.WriteFile(cli, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	d := NewAnthropicDriver("", "")
	d.SetCLIPath(cli)
	start := time.Now()
	_, err := d.StreamActivity(context.Background(), &LLMRequest{Messages: []Message{{Role: RoleUser, Content: "go"}},
		IdleTimeout: 200 * time.Millisecond}, func(StreamEvent) {})
	if !errors.Is(err, ErrAgentIdle) {
		t.Fatalf("err = %v, want ErrAgentIdle", err)
	}
	if time.Since(start) > 10*time.Second {
		t.Fatal("idle turn was not stopped promptly")
	}
}

func TestCLITurnTimeLimitIsReported(t *testing.T) {
	cli := filepath.Join(t.TempDir(), "claude")
	script := "#!/bin/sh\nwhile true; do echo '{\"type\":\"system\",\"subtype\":\"status\"}'; sleep 0.05; done\n"
	if err := os.WriteFile(cli, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	d := NewAnthropicDriver("", "")
	d.SetCLIPath(cli)
	_, err := d.StreamActivity(context.Background(), &LLMRequest{Messages: []Message{{Role: RoleUser, Content: "go"}},
		Timeout: 300 * time.Millisecond, IdleTimeout: time.Minute}, func(StreamEvent) {})
	if !errors.Is(err, ErrTurnTimeLimit) {
		t.Fatalf("err = %v, want ErrTurnTimeLimit", err)
	}
}

func TestWaitingOnTheOperatorIsNotIdle(t *testing.T) {
	cli, argsFile, _ := fakeInteractiveCLI(t)
	d := NewAnthropicDriver("", "")
	d.SetCLIPath(cli)
	resp, err := d.StreamActivity(context.Background(), &LLMRequest{
		Messages: []Message{{Role: RoleUser, Content: "fix the bug"}}, PermissionMode: "acceptEdits",
		IdleTimeout: 1500 * time.Millisecond, AllowedTools: []string{"Bash(git status:*)", "Bash(ls:*)"},
		Approver: func(ctx context.Context, req ApprovalRequest) ApprovalDecision {
			time.Sleep(4 * time.Second) // the operator takes longer than the idle timeout
			return ApprovalDecision{Allow: true}
		},
	}, func(StreamEvent) {})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Content != "ALLOWED" {
		t.Fatalf("CLI saw %q", resp.Content)
	}
	args, _ := os.ReadFile(argsFile)
	if !strings.Contains(string(args), "--allowedTools\nBash(git status:*),Bash(ls:*)") {
		t.Fatalf("allowed tools not passed:\n%s", args)
	}
}
