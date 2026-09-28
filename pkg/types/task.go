package types

import "time"

// TaskState represents execution states for orchestrator tasks.
type TaskState string

const (
	TaskStatePending             TaskState = "PENDING"
	TaskStateRunning             TaskState = "RUNNING"
	TaskStateWaitingGateApproval TaskState = "WAITING_GATE_APPROVAL"
	TaskStateBlockedFrustration  TaskState = "BLOCKED_FRUSTRATION"
	TaskStateWaitingDependency   TaskState = "WAITING_DEPENDENCY"
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
	Dependencies      []string          `json:"dependencies,omitempty"`
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
	CachedTokens     int64   `json:"cached_tokens,omitempty"` // subset of PromptTokens served from provider prompt cache
	TotalTokens      int64   `json:"total_tokens"`
	EstimatedCostUSD float64 `json:"estimated_cost_usd"`
}

// TaskProfile summarizes dynamic classification of a request.
type TaskProfile struct {
	WorkflowID       string   `json:"workflow_id"`
	Complexity       string   `json:"complexity"` // "LOW", "MEDIUM", "HIGH", "CRITICAL"
	TaskType         string   `json:"task_type,omitempty"` // recurring category, e.g. "crud", "migration"
	IdentifiedRepos  []string `json:"identified_repos"`
	RecommendedModel string   `json:"recommended_model"`
	Method           string   `json:"method"`
	TokenBudget      int64    `json:"token_budget"`
	RequiresDocker   bool     `json:"requires_docker"`
}

// TaskSubProcess represents a child process spawned within the task container.
type TaskSubProcess struct {
	PID     int    `json:"pid"`
	Command string `json:"command"`
	Status  string `json:"status"` // "RUNNING", "COMPLETED", "FAILED"
}

// TaskProcessInfo describes the background runtime process, console terminal logs, and system telemetry of a task.
type TaskProcessInfo struct {
	TaskID          string           `json:"task_id"`
	ProcessID       int              `json:"process_id"`
	Command         string           `json:"command"`
	WorkingDir      string           `json:"working_dir"`
	ContainerID     string           `json:"container_id"`
	Status          string           `json:"status"` // "RUNNING", "IDLE", "COMPLETED", "PAUSED", "BLOCKED", "FAILED"
	StartedAt       time.Time        `json:"started_at"`
	DurationSeconds int64            `json:"duration_seconds"`
	CPUPercent      float64          `json:"cpu_percent"`
	MemoryMB        float64          `json:"memory_mb"`
	CurrentStep     string           `json:"current_step"`
	ActiveAgent     string           `json:"active_agent"`
	ExitCode        int              `json:"exit_code,omitempty"`
	SubProcesses    []TaskSubProcess `json:"subprocesses,omitempty"`
	Logs            []string         `json:"logs"`
}
