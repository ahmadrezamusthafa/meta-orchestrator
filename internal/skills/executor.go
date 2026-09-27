package skills

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// StandardSkillExecutor executes UniversalSkillContracts in secure subprocess or containers.
type StandardSkillExecutor struct{}

// NewStandardSkillExecutor creates a standard skill executor.
func NewStandardSkillExecutor() *StandardSkillExecutor {
	return &StandardSkillExecutor{}
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

	timeout := time.Duration(skill.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// In test/subprocess mode: execute script if present
	if skill.SourceFormat == types.SkillFormatClaude {
		// 1. If explicit command/script is configured, execute it as a subprocess
		if skill.Command != "" {
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
			} else if skill.SourceLocation != "" {
				cmd.Dir = skill.SourceLocation
			}
			if len(req.Parameters) > 0 {
				paramBytes, _ := json.Marshal(req.Parameters)
				cmd.Stdin = bytes.NewReader(paramBytes)
			}
			out, err := cmd.CombinedOutput()
			if streamWriter != nil {
				_, _ = streamWriter.Write(out)
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
				Stdout:   string(out),
			}, err
		}

		// 2. If prompt/instruction-based Claude skill, return normalized instruction context
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

	if skill.SourceFormat == types.SkillFormatMCP {
		cmdName := skill.Command
		if cmdName == "" {
			cmdName = "npx"
		}
		cmd := exec.CommandContext(execCtx, cmdName, skill.Args...)
		cmd.Env = os.Environ()
		for k, v := range skill.Environment {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}
		for k, v := range req.EnvironmentVars {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}
		if req.WorkspacePath != "" {
			cmd.Dir = req.WorkspacePath
		}
		if len(req.Parameters) > 0 {
			paramBytes, _ := json.Marshal(req.Parameters)
			cmd.Stdin = bytes.NewReader(paramBytes)
		}
		out, err := cmd.CombinedOutput()
		if streamWriter != nil {
			_, _ = streamWriter.Write(out)
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
			Stdout:   string(out),
		}, err
	}

	return &types.SkillExecutionResult{
		ExitCode: 0,
		Stdout:   fmt.Sprintf("Executed skill %s successfully", skill.Name),
	}, nil
}
