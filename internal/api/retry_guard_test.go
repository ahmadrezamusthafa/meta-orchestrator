package api

import (
	"context"
	"strings"
	"testing"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/llm"
)

func TestRetryGuardRefusesARepeatedFailingCommand(t *testing.T) {
	g := newRetryGuard()
	asked := 0
	refused := ""
	approve := g.guard(func(context.Context, llm.ApprovalRequest) llm.ApprovalDecision {
		asked++
		return llm.ApprovalDecision{Allow: true}
	}, func(cmd string) { refused = cmd })
	run := map[string]interface{}{"command": "go  test ./..."}
	ask := func() llm.ApprovalDecision {
		return approve(context.Background(), llm.ApprovalRequest{ToolName: "Bash", Input: run})
	}

	for i, id := range []string{"t1", "t2"} {
		if d := ask(); !d.Allow {
			t.Fatalf("attempt %d refused: %s", i+1, d.Message)
		}
		g.toolUse(id, "Bash", run)
		g.toolResult(id, true)
	}
	d := ask()
	if d.Allow || !strings.Contains(d.Message, "failed 2 times") || refused != "go test ./..." || asked != 2 {
		t.Fatalf("third identical attempt: %+v, refused=%q, asked=%d", d, refused, asked)
	}
	if d := approve(context.Background(), llm.ApprovalRequest{ToolName: "Bash", Input: map[string]interface{}{"command": "go test ./internal/api"}}); !d.Allow {
		t.Fatal("a different command must still reach the operator")
	}

	// Editing a file makes the retry legitimate.
	g.toolUse("e1", "Edit", map[string]interface{}{"file_path": "main.go"})
	if d := ask(); !d.Allow {
		t.Fatalf("retry after an edit refused: %s", d.Message)
	}
}

func TestRetryGuardForgetsACommandThatSucceeds(t *testing.T) {
	g := newRetryGuard()
	run := map[string]interface{}{"command": "make build"}
	g.toolUse("t1", "Bash", run)
	g.toolResult("t1", true)
	g.toolUse("t2", "Bash", run)
	g.toolResult("t2", false)
	g.toolUse("t3", "Bash", run)
	g.toolResult("t3", true)
	if msg := g.refusal("Bash", run); msg != "" {
		t.Fatalf("failures must be consecutive: %s", msg)
	}
}
