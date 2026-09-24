package llm

import (
	"context"

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
	Model       string                 `json:"model"`
	Messages    []Message              `json:"messages"`
	Temperature float32                `json:"temperature"`
	MaxTokens   int                    `json:"max_tokens"`
	Tools       []*types.UniversalSkillContract `json:"tools,omitempty"`
}

// LLMResponse normalizes responses from all providers.
type LLMResponse struct {
	Content      string           `json:"content"`
	ToolCalls    []ToolCall       `json:"tool_calls,omitempty"`
	FinishReason string           `json:"finish_reason"`
	TokenUsage   types.TokenUsage `json:"token_usage"`
	Provider     string           `json:"provider"`
	Model        string           `json:"model"`
}

// ProviderClient specifies the standard driver contract for all AI providers.
type ProviderClient interface {
	Complete(ctx context.Context, req *LLMRequest) (*LLMResponse, error)
	Stream(ctx context.Context, req *LLMRequest, chunkCh chan<- types.ThoughtChunk) (*LLMResponse, error)
	CountTokens(req *LLMRequest) int64
}
