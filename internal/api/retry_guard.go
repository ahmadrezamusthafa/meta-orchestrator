package api

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/llm"
)

// maxIdenticalFailures is how often one shell command may fail in a turn, with no file edit in
// between, before the orchestrator refuses to run it unchanged again.
const maxIdenticalFailures = 2

// readOnlyCommands run without an approval round-trip: they only inspect the repository. Waiting
// on (or being denied) these made agents retry variations of the same command.
var readOnlyCommands = []string{
	"Bash(git status:*)", "Bash(git diff:*)", "Bash(git log:*)", "Bash(git show:*)",
	"Bash(git rev-parse:*)", "Bash(git ls-files:*)", "Bash(pwd)", "Bash(ls:*)",
}

// fileEditTools change files, which makes re-running a failed command a legitimate retry.
var fileEditTools = map[string]bool{"Edit": true, "Write": true, "MultiEdit": true, "NotebookEdit": true}

// retryGuard tracks shell commands that fail repeatedly within one turn, so the agent is stopped
// from burning tokens re-running a command that will fail the same way.
type retryGuard struct {
	mu       sync.Mutex
	commands map[string]string // tool use id → normalized command
	failures map[string]int    // normalized command → consecutive failures since the last file edit
}

func newRetryGuard() *retryGuard {
	return &retryGuard{commands: map[string]string{}, failures: map[string]int{}}
}

func shellCommand(tool string, input map[string]interface{}) string {
	if tool != "Bash" {
		return ""
	}
	cmd, _ := input["command"].(string)
	return strings.Join(strings.Fields(cmd), " ")
}

// toolUse records a tool call as the agent makes it.
func (g *retryGuard) toolUse(id, tool string, input map[string]interface{}) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if fileEditTools[tool] {
		clear(g.failures)
		return
	}
	if cmd := shellCommand(tool, input); cmd != "" && id != "" {
		g.commands[id] = cmd
	}
}

// toolResult records how a tool call ended.
func (g *retryGuard) toolResult(id string, isError bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	cmd, ok := g.commands[id]
	if !ok {
		return
	}
	delete(g.commands, id)
	if isError {
		g.failures[cmd]++
	} else {
		delete(g.failures, cmd)
	}
}

// refusal explains why a command must not run again unchanged, or returns "" when it may.
func (g *retryGuard) refusal(tool string, input map[string]interface{}) string {
	cmd := shellCommand(tool, input)
	if cmd == "" {
		return ""
	}
	g.mu.Lock()
	n := g.failures[cmd]
	g.mu.Unlock()
	if n < maxIdenticalFailures {
		return ""
	}
	return fmt.Sprintf("Not run: this exact command already failed %d times in this turn with no file changes in between, "+
		"so it would fail the same way. Read its earlier error output, then fix the cause, run a different command, "+
		"or stop and explain to the operator what is blocking you.", n)
}

// guard wraps an Approver so a repeated failing command is refused before it reaches the operator.
// onRefuse reports each refusal.
func (g *retryGuard) guard(next llm.Approver, onRefuse func(cmd string)) llm.Approver {
	if next == nil {
		return nil
	}
	return func(ctx context.Context, req llm.ApprovalRequest) llm.ApprovalDecision {
		if msg := g.refusal(req.ToolName, req.Input); msg != "" {
			onRefuse(shellCommand(req.ToolName, req.Input))
			return llm.ApprovalDecision{Message: msg}
		}
		return next(ctx, req)
	}
}
