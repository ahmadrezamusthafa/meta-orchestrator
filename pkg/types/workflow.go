package types

import "time"

// StageType defines standard execution modes for a workflow stage.
type StageType string

const (
	StageTypeAutomated StageType = "automated"
	StageTypeReview    StageType = "review_gate"
	StageTypeManual    StageType = "manual_verification"
)

// WorkflowStage defines a single step in a custom or standard SDLC workflow.
type WorkflowStage struct {
	ID                 string            `json:"id" yaml:"id"`
	Name               string            `json:"name" yaml:"name"`
	Type               StageType         `json:"type" yaml:"type"`
	AssignedRole       string            `json:"assigned_role" yaml:"assigned_role"`
	AllowedMethods     []string          `json:"allowed_methods" yaml:"allowed_methods"`
	WriteLockWorkspace bool              `json:"write_lock_workspace" yaml:"write_lock_workspace"`
	RequiresGate       bool              `json:"requires_gate" yaml:"requires_gate"`
	GateCriteria       string            `json:"gate_criteria,omitempty" yaml:"gate_criteria,omitempty"`
	Timeout            time.Duration     `json:"timeout" yaml:"timeout"`
	RequiredArtifacts  []string          `json:"required_artifacts,omitempty" yaml:"required_artifacts,omitempty"`
	Metadata           map[string]string `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

// WorkflowDefinition specifies the full declarative definition of an SDLC pipeline.
type WorkflowDefinition struct {
	ID          string          `json:"id" yaml:"id"`
	Name        string          `json:"name" yaml:"name"`
	Description string          `json:"description" yaml:"description"`
	Version     string          `json:"version" yaml:"version"`
	Stages      []WorkflowStage `json:"stages" yaml:"stages"`
	DefaultRole string          `json:"default_role" yaml:"default_role"`
}

// StageSlice defines a bounded execution window across stages.
type StageSlice struct {
	StartStageID string `json:"start_stage_id" yaml:"start_stage_id"`
	HaltStageID  string `json:"halt_stage_id" yaml:"halt_stage_id"`
	ProduceVideo bool   `json:"produce_video" yaml:"produce_video"`
}
