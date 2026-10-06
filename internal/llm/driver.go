package llm

import (
	"context"
	"errors"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// Role denotes message sender type.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Message encapsulates a prompt or conversational turn.
type Message struct {
	Role       Role                   `json:"role"`
	Content    string                 `json:"content"`
	ToolCalls  []ToolCall             `json:"tool_calls,omitempty"`
	ToolCallID string                 `json:"tool_call_id,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// ToolCall represents an LLM's invocation of a tool/skill.
type ToolCall struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

// LLMRequest encapsulates the unified payload sent to any provider.
type LLMRequest struct {
	Model       string                          `json:"model"`
	Messages    []Message                       `json:"messages"`
	Temperature float32                         `json:"temperature"`
	MaxTokens   int                             `json:"max_tokens"`
	Tools       []*types.UniversalSkillContract `json:"tools,omitempty"`
	// SessionID resumes a provider-side conversation (Claude Code CLI --resume).
	SessionID string `json:"session_id,omitempty"`
	// WorkDir is the working directory for agentic providers (Claude Code CLI).
	WorkDir string `json:"work_dir,omitempty"`
	// AddDirs are extra directories an agentic provider may read (Claude Code CLI --add-dir).
	AddDirs []string `json:"add_dirs,omitempty"`
	// PermissionMode for agentic providers; empty means read-only ("plan").
	PermissionMode string `json:"permission_mode,omitempty"`
	// Timeout bounds agentic turns; zero uses the driver default.
	Timeout time.Duration `json:"-"`
	// IdleTimeout stops an agentic turn that produces no output for this long (time spent waiting
	// for the operator to answer a permission prompt does not count); zero uses the driver default.
	IdleTimeout time.Duration `json:"-"`
	// AllowedTools are permission rules the agent may use without asking (Claude Code CLI
	// --allowedTools, e.g. "Bash(git status:*)").
	AllowedTools []string `json:"allowed_tools,omitempty"`
	// Approver answers the agent's permission prompts (Claude Code CLI). Nil means nobody can
	// answer, so anything that needs permission is denied.
	Approver Approver `json:"-"`
}

// Errors an agentic turn stops with when it runs out of time. The provider session is kept, so
// the turn can be continued.
var (
	ErrAgentIdle     = errors.New("the agent produced no output")
	ErrTurnTimeLimit = errors.New("the turn hit its time limit")
)

// ApprovalRequest is an agent's request to use a tool that needs operator permission.
type ApprovalRequest struct {
	ToolName    string
	ToolUseID   string
	Description string
	Input       map[string]interface{}
	BlockedPath string
}

// Tools whose permission prompt is an operator interaction rather than a gated action.
const (
	// ToolExitPlanMode asks the operator to approve the agent's plan and leave read-only plan mode.
	ToolExitPlanMode = "ExitPlanMode"
	// ToolAskUserQuestion asks the operator multiple-choice questions; the answers go back in its input.
	ToolAskUserQuestion = "AskUserQuestion"
)

// ApprovalDecision answers an ApprovalRequest. Message is shown to the agent when denied.
type ApprovalDecision struct {
	Allow   bool
	Message string
	// Mode is the permission mode the agent switches to when an allowed ExitPlanMode leaves plan
	// mode; empty means "default" (every gated action still asks).
	Mode string
	// UpdatedInput replaces the tool input when allowed (AskUserQuestion answers); nil keeps it.
	UpdatedInput map[string]interface{}
}

// PermissionDenial is a tool call the agent could not make because permission was refused.
type PermissionDenial struct {
	ToolName string                 `json:"tool_name"`
	Input    map[string]interface{} `json:"tool_input,omitempty"`
}

// Approver blocks until the request is decided or ctx ends.
type Approver func(ctx context.Context, req ApprovalRequest) ApprovalDecision

// LLMResponse normalizes responses from all providers.
type LLMResponse struct {
	Content      string           `json:"content"`
	ToolCalls    []ToolCall       `json:"tool_calls,omitempty"`
	FinishReason string           `json:"finish_reason"`
	TokenUsage   types.TokenUsage `json:"token_usage"`
	Provider     string           `json:"provider"`
	Model        string           `json:"model"`
	SessionID    string           `json:"session_id,omitempty"`
	DurationMS   int64            `json:"duration_ms,omitempty"`
	// PermissionDenials lists the tool calls of the turn that were refused permission.
	PermissionDenials []PermissionDenial `json:"permission_denials,omitempty"`
}

// ProviderClient specifies the standard driver contract for all AI providers.
type ProviderClient interface {
	Complete(ctx context.Context, req *LLMRequest) (*LLMResponse, error)
	Stream(ctx context.Context, req *LLMRequest, chunkCh chan<- types.ThoughtChunk) (*LLMResponse, error)
	CountTokens(req *LLMRequest) int64
}
