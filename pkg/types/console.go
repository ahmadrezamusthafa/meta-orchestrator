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
	ConsoleKindApproval   = "approval" // the agent asks the operator for permission
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
	// Model is the model that actually answered; RoutedModel is what the router asked for, set
	// only when the two differ.
	Model            string  `json:"model"`
	RoutedModel      string  `json:"routed_model,omitempty"`
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
	ID        string           `json:"id"`
	TaskID    string           `json:"task_id"`
	TurnID    string           `json:"turn_id,omitempty"`
	Kind      string           `json:"kind"`
	Content   string           `json:"content,omitempty"`
	Status    string           `json:"status,omitempty"`
	Tool      *ConsoleTool     `json:"tool,omitempty"`
	Request   *ConsoleRequest  `json:"request,omitempty"`
	Usage     *ConsoleUsage    `json:"usage,omitempty"`
	State     *ConsoleState    `json:"state,omitempty"`
	Approval  *ConsoleApproval `json:"approval,omitempty"`
	CreatedAt time.Time        `json:"created_at"`
	UpdatedAt time.Time        `json:"updated_at"`
}

// Approval decisions.
const (
	ApprovalPending   = "pending"
	ApprovalAllowed   = "allowed"
	ApprovalAlways    = "always"
	ApprovalDenied    = "denied"
	ApprovalExpired   = "expired"
	ApprovalCancelled = "cancelled"
	ApprovalAutomatic = "auto" // matched a rule the operator saved earlier
	ApprovalAll       = "all"  // allowed, and every later request in the task is allowed too
)

// ConsoleApproval is a permission request from the agent and its outcome.
type ConsoleApproval struct {
	ID          string    `json:"id"`
	ToolName    string    `json:"tool_name"`
	Summary     string    `json:"summary"`              // the command, file or URL in one line
	RuleLabel   string    `json:"rule_label,omitempty"` // what "always allow" would cover
	BlockedPath string    `json:"blocked_path,omitempty"`
	Decision    string    `json:"decision"`
	Message     string    `json:"message,omitempty"` // operator note sent to the agent on deny
	ExpiresAt   time.Time `json:"expires_at"`
	DecidedAt   time.Time `json:"decided_at,omitempty"`
}
