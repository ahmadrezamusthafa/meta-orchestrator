package types

import "time"

// Console event topics (agent activity transcript).
const (
	EventAgentActivity      EventType = "agent.activity"
	EventAgentActivityDelta EventType = "agent.activity.delta"
	EventAgentActivityClear EventType = "agent.activity.clear"
)

// Console entry kinds.
const (
	ConsoleKindUser       = "user"
	ConsoleKindAssistant  = "assistant"
	ConsoleKindThinking   = "thinking"
	ConsoleKindToolUse    = "tool_use"
	ConsoleKindToolResult = "tool_result"
	ConsoleKindRequest    = "request"
	ConsoleKindResponse   = "response"
	ConsoleKindSystem     = "system"
	ConsoleKindError      = "error"
	ConsoleKindState      = "state"
)

// Console entry statuses.
const (
	ConsoleStreaming = "streaming"
	ConsoleDone      = "done"
	ConsoleFailed    = "error"
	ConsoleCancelled = "cancelled"
)

// ConsoleTool describes a tool invocation or its result.
type ConsoleTool struct {
	ID      string                 `json:"id"`
	Name    string                 `json:"name,omitempty"`
	Input   map[string]interface{} `json:"input,omitempty"`
	IsError bool                   `json:"is_error,omitempty"`
}

// ConsoleMessage is one message of a recorded model request.
type ConsoleMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ConsoleRequest records exactly what was sent to the model router.
type ConsoleRequest struct {
	Model         string           `json:"model"`
	Method        string           `json:"method,omitempty"`
	Strategy      string           `json:"strategy,omitempty"`
	FallbackChain []string         `json:"fallback_chain,omitempty"`
	Messages      []ConsoleMessage `json:"messages"`
	SessionID     string           `json:"session_id,omitempty"`
	Source        string           `json:"source"` // "chat" or "execute"
}

// ConsoleUsage records what came back.
type ConsoleUsage struct {
	Model            string  `json:"model"`
	Provider         string  `json:"provider"`
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	CachedTokens     int64   `json:"cached_tokens"`
	CostUSD          float64 `json:"cost_usd"`
	DurationMS       int64   `json:"duration_ms"`
	FinishReason     string  `json:"finish_reason"`
	SessionID        string  `json:"session_id,omitempty"`
}

// ConsoleState records a task state transition.
type ConsoleState struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Stage string `json:"stage,omitempty"`
}

// ConsoleEntry is one item of a task's real activity transcript.
type ConsoleEntry struct {
	ID        string          `json:"id"`
	TaskID    string          `json:"task_id"`
	TurnID    string          `json:"turn_id,omitempty"`
	Kind      string          `json:"kind"`
	Content   string          `json:"content,omitempty"`
	Status    string          `json:"status,omitempty"`
	Tool      *ConsoleTool    `json:"tool,omitempty"`
	Request   *ConsoleRequest `json:"request,omitempty"`
	Usage     *ConsoleUsage   `json:"usage,omitempty"`
	State     *ConsoleState   `json:"state,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}
