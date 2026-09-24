package types

import "time"

// TaskState represents execution states for orchestrator tasks.
type TaskState string

const (
	TaskStatePending             TaskState = "PENDING"
	TaskStateRunning             TaskState = "RUNNING"
	TaskStateWaitingGateApproval TaskState = "WAITING_GATE_APPROVAL"
	TaskStateBlockedFrustration  TaskState = "BLOCKED_FRUSTRATION"
	TaskStateCompleted           TaskState = "COMPLETED"
	TaskStateFailed              TaskState = "FAILED"
	TaskStateSuspended           TaskState = "SUSPENDED"
)

// Task represents an atomic unit of execution in the Meta-Orchestrator.
type Task struct {
	ID                string            `json:"id"`
	WorkflowID        string            `json:"workflow_id"`
	Title             string            `json:"title"`
	Description       string            `json:"description"`
	CurrentStageID    string            `json:"current_stage_id"`
	CurrentStageIndex int               `json:"current_stage_index"`
	State             TaskState         `json:"state"`
	ActiveSlice       *StageSlice       `json:"active_slice,omitempty"`
	AssignedRepos     []string          `json:"assigned_repos"`
	ProfileName       string            `json:"profile_name"`
	SelectedMethod    string            `json:"selected_method"`
	TokenUsage        TokenUsage        `json:"token_usage"`
	MaxTokenBudget    int64             `json:"max_token_budget"`
	ArtifactDir       string            `json:"artifact_dir"`
	Metadata          map[string]string `json:"metadata,omitempty"`
	CreatedAt         time.Time         `json:"created_at"`
	UpdatedAt         time.Time         `json:"updated_at"`
}

// TokenUsage tracks token expenditures across providers and models.
type TokenUsage struct {
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	TotalTokens      int64   `json:"total_tokens"`
	EstimatedCostUSD float64 `json:"estimated_cost_usd"`
}

// TaskProfile summarizes dynamic classification of a request.
type TaskProfile struct {
	WorkflowID       string   `json:"workflow_id"`
	Complexity       string   `json:"complexity"` // "LOW", "MEDIUM", "HIGH", "CRITICAL"
	IdentifiedRepos  []string `json:"identified_repos"`
	RecommendedModel string   `json:"recommended_model"`
	Method           string   `json:"method"`
	TokenBudget      int64    `json:"token_budget"`
	RequiresDocker   bool     `json:"requires_docker"`
}
