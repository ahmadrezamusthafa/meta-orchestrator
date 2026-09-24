package types

import "time"

// EventType represents pub/sub event topics in the orchestrator.
type EventType string

const (
	EventAgentThought   EventType = "agent.thought"
	EventAgentTerminal  EventType = "agent.terminal"
	EventTaskStatus     EventType = "task.status"
	EventStageTransition EventType = "stage.transition"
	EventFrustrationHalt EventType = "event.frustration_halt"
	EventHITLRequired    EventType = "hitl.required"
	EventToolProgress   EventType = "tool.install.progress"
)

// OrchestratorEvent encapsulates payload broadcast via Redis and WebSocket.
type OrchestratorEvent struct {
	Type      EventType   `json:"type"`
	TaskID    string      `json:"task_id"`
	StageID   string      `json:"stage_id,omitempty"`
	Timestamp time.Time   `json:"timestamp"`
	Payload   interface{} `json:"payload"`
}

// ThoughtChunk represents live LLM reasoning stream.
type ThoughtChunk struct {
	ProfileName string `json:"profile_name"`
	ModelID     string `json:"model_id"`
	Content     string `json:"content"`
	IsComplete  bool   `json:"is_complete"`
}

// TerminalChunk represents raw stdout/stderr output from container/process.
type TerminalChunk struct {
	Stream string `json:"stream"` // "stdout" or "stderr"
	Data   string `json:"data"`
}

// FrustrationHaltPayload details the loop circuit breaker trip.
type FrustrationHaltPayload struct {
	TaskID          string `json:"task_id"`
	StageID         string `json:"stage_id"`
	ConsecutiveFails int    `json:"consecutive_fails"`
	ErrorSignature  string `json:"error_signature"`
	LastStackTrace  string `json:"last_stack_trace"`
}
