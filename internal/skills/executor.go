package skills

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// StandardSkillExecutor runs skills that have a command (Claude skills with a command or
// entrypoint script, MCP servers, superpower tools) as a subprocess, and resolves the instructions
// of instruction-only Claude skills. Anything else is an error: it never reports a run that did
// not happen.
type StandardSkillExecutor struct {
	// MaxOutputBytes caps the captured output; zero uses defaultMaxOutput.
	MaxOutputBytes int
}

const defaultMaxOutput = 1 << 20

// NewStandardSkillExecutor creates a standard skill executor.
func NewStandardSkillExecutor() *StandardSkillExecutor {
	return &StandardSkillExecutor{}
}

// ErrNotRunnable means the skill has nothing the executor can run.
var ErrNotRunnable = errors.New("skill has no command to run")

// Runnable reports whether Execute would start a subprocess for the skill.
func Runnable(skill *types.UniversalSkillContract) bool {
	return skill != nil && skill.Command != ""
}

// Execute runs the skill according to isolation constraints and permission boundaries.
func (e *StandardSkillExecutor) Execute(
	ctx context.Context,
	skill *types.UniversalSkillContract,
	req *types.SkillExecutionRequest,
	streamWriter io.Writer,
) (*types.SkillExecutionResult, error) {
	if skill == nil {
		return nil, fmt.Errorf("skill cannot be nil")
	}
	if req == nil {
		return nil, fmt.Errorf("request cannot be nil")
	}

	// 1. RBAC check
	if len(skill.RequiredRoles) > 0 {
		roleAuthorized := false
		for _, r := range skill.RequiredRoles {
			if r == req.CallerRole {
				roleAuthorized = true
				break
			}
		}
		if !roleAuthorized {
			return nil, fmt.Errorf("role '%s' is unauthorized to execute skill '%s' (requires one of %v)",
				req.CallerRole, skill.Name, skill.RequiredRoles)
		}
	}

	if Runnable(skill) {
		return e.run(ctx, skill, req, streamWriter)
	}

	// Instruction-only Claude skill: return its normalized instructions.
	if skill.SourceFormat == types.SkillFormatClaude {
		instructions := ""
		if skill.Metadata != nil {
			if inst, ok := skill.Metadata["instructions"].(string); ok {
				instructions = inst
			}
		}
		if instructions == "" {
			instructions = skill.Description
		}
		stdout := fmt.Sprintf("Claude skill [%s] instructions resolved:\n%s", skill.Name, instructions)
		if streamWriter != nil {
			_, _ = streamWriter.Write([]byte(stdout))
		}
		return &types.SkillExecutionResult{
			ExitCode: 0,
			Stdout:   stdout,
			Output: map[string]interface{}{
				"instructions": instructions,
				"skill_name":   skill.Name,
			},
		}, nil
	}

	return nil, fmt.Errorf("%w: %s (%s)", ErrNotRunnable, skill.Name, skill.SourceFormat)
}

func (e *StandardSkillExecutor) run(ctx context.Context, skill *types.UniversalSkillContract, req *types.SkillExecutionRequest, streamWriter io.Writer) (*types.SkillExecutionResult, error) {
	timeout := time.Duration(skill.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(execCtx, skill.Command, skill.Args...)
	cmd.Env = os.Environ()
	for k, v := range skill.Environment {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}
	for k, v := range req.EnvironmentVars {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}
	if req.WorkspacePath != "" {
		cmd.Dir = req.WorkspacePath
	} else if skill.SourceFormat == types.SkillFormatClaude && skill.SourceLocation != "" {
		cmd.Dir = skill.SourceLocation
	}
	if len(req.Parameters) > 0 {
		paramBytes, _ := json.Marshal(req.Parameters)
		cmd.Stdin = bytes.NewReader(paramBytes)
	}
	limit := e.MaxOutputBytes
	if limit <= 0 {
		limit = defaultMaxOutput
	}
	out := &cappedBuffer{limit: limit}
	cmd.Stdout, cmd.Stderr = out, out
	err := cmd.Run()
	if execCtx.Err() == context.DeadlineExceeded {
		err = fmt.Errorf("skill %s timed out after %s", skill.Name, timeout)
	}
	stdout := out.String()
	if streamWriter != nil {
		_, _ = streamWriter.Write([]byte(stdout))
	}
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}
	return &types.SkillExecutionResult{
		ExitCode: exitCode,
		Stdout:   stdout,
	}, err
}

// cappedBuffer keeps the first limit bytes written and counts the rest.
type cappedBuffer struct {
	mu      sync.Mutex
	buf     bytes.Buffer
	limit   int
	dropped int
}

func (c *cappedBuffer) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	room := c.limit - c.buf.Len()
	if room > len(p) {
		room = len(p)
	}
	if room > 0 {
		c.buf.Write(p[:room])
	}
	c.dropped += len(p) - max(room, 0)
	return len(p), nil
}

func (c *cappedBuffer) String() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.dropped > 0 {
		return c.buf.String() + fmt.Sprintf("\n[… %d more bytes of output not shown]", c.dropped)
	}
	return c.buf.String()
}
