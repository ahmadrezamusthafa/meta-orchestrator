package types

import (
	"context"
	"io"
)

// SkillFormat denotes the original ecosystem source of a tool.
type SkillFormat string

const (
	SkillFormatClaude     SkillFormat = "claude"     // Anthropic SKILL.md bundle
	SkillFormatBMAD       SkillFormat = "bmad"       // BMAD skill pack
	SkillFormatSuperpower SkillFormat = "superpower" // Superpower shell execution tool
	SkillFormatMCP        SkillFormat = "mcp"        // Model Context Protocol stdio/SSE tool
	SkillFormatOpenAI     SkillFormat = "openai"     // OpenAI function definition
	SkillFormatNative     SkillFormat = "native"     // Native Go internal tool
)

// IsolationLevel defines execution boundary restrictions.
type IsolationLevel string

const (
	IsolationSubprocess IsolationLevel = "subprocess"
	IsolationDocker     IsolationLevel = "docker"
	IsolationHost       IsolationLevel = "host"
)

// UniversalSkillContract provides the normalized specification for any tool.
type UniversalSkillContract struct {
	Name            string                 `json:"name" yaml:"name"`
	Description     string                 `json:"description" yaml:"description"`
	SourceFormat    SkillFormat            `json:"source_format" yaml:"source_format"`
	SourceLocation  string                 `json:"source_location" yaml:"source_location"`
	Isolation       IsolationLevel         `json:"isolation" yaml:"isolation"`
	InputSchema     map[string]interface{} `json:"input_schema" yaml:"input_schema"`
	RequiredRoles   []string               `json:"required_roles,omitempty" yaml:"required_roles,omitempty"`
	TimeoutSeconds  int                    `json:"timeout_seconds" yaml:"timeout_seconds"`
	RequiresNetwork bool                   `json:"requires_network" yaml:"requires_network"`
}

// SkillExecutionRequest parameters passed when executing a skill.
type SkillExecutionRequest struct {
	TaskID          string                 `json:"task_id"`
	CallerRole      string                 `json:"caller_role"`
	Parameters      map[string]interface{} `json:"parameters"`
	WorkspacePath   string                 `json:"workspace_path"`
	ContainerID     string                 `json:"container_id,omitempty"`
	EnvironmentVars map[string]string      `json:"environment_vars,omitempty"`
}

// SkillExecutionResult captures output from executing a skill.
type SkillExecutionResult struct {
	ExitCode int                    `json:"exit_code"`
	Stdout   string                 `json:"stdout"`
	Stderr   string                 `json:"stderr"`
	Output   map[string]interface{} `json:"output,omitempty"`
	Error    string                 `json:"error,omitempty"`
}

// SkillExecutor executes normalized skills across execution targets.
type SkillExecutor interface {
	Execute(ctx context.Context, skill *UniversalSkillContract, req *SkillExecutionRequest, streamWriter io.Writer) (*SkillExecutionResult, error)
}
