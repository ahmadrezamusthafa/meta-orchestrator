package fsm

import (
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
	"gopkg.in/yaml.v3"
)

// DefaultWorkflowID is the workflow a task runs when it names none.
const DefaultWorkflowID = "general_ai_sdlc"

// legacyWorkflowIDs maps the IDs older builds (and the task profiler) used to the built-in workflows.
var legacyWorkflowIDs = map[string]string{
	"general-ai-sdlc":   "general_ai_sdlc",
	"hotfix-fast-track": "fast_hotfix_sdlc",
	"microservice-api":  "microservice_api",
}

// ErrWorkflowExists reports a workflow ID that is already taken or belongs to a built-in workflow.
var ErrWorkflowExists = errors.New("workflow already exists")

// WorkflowRegistry manages built-in and user-defined custom workflows. It is safe for concurrent use.
type WorkflowRegistry struct {
	mu        sync.RWMutex
	workflows map[string]*types.WorkflowDefinition
	order     []string
}

// NewWorkflowRegistry initializes a registry with standard built-in workflows.
func NewWorkflowRegistry() *WorkflowRegistry {
	reg := &WorkflowRegistry{
		workflows: make(map[string]*types.WorkflowDefinition),
	}
	for _, def := range []*types.WorkflowDefinition{GetGeneralAISDLCWorkflow(), GetHotfixFastTrackWorkflow(), GetMicroserviceAPIWorkflow()} {
		def.Builtin = true
		reg.Register(def)
	}
	return reg
}

// Register registers (or replaces) a workflow definition.
func (r *WorkflowRegistry) Register(def *types.WorkflowDefinition) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.workflows[def.ID]; !exists {
		r.order = append(r.order, def.ID)
	}
	r.workflows[def.ID] = def
}

// Add registers a custom workflow after validating its structure. With replace false an existing ID
// is a conflict; built-in workflows (and their legacy IDs) can never be replaced.
func (r *WorkflowRegistry) Add(def *types.WorkflowDefinition, replace bool) error {
	if err := ValidateWorkflow(def); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, legacy := legacyWorkflowIDs[def.ID]; legacy {
		return fmt.Errorf("%w: '%s' is a built-in workflow", ErrWorkflowExists, def.ID)
	}
	if existing, ok := r.workflows[def.ID]; ok {
		if existing.Builtin {
			return fmt.Errorf("%w: '%s' is a built-in workflow", ErrWorkflowExists, def.ID)
		}
		if !replace {
			return fmt.Errorf("%w: '%s'", ErrWorkflowExists, def.ID)
		}
	} else {
		r.order = append(r.order, def.ID)
	}
	cp := *def
	cp.Builtin = false
	cp.Stages = append([]types.WorkflowStage(nil), def.Stages...)
	r.workflows[def.ID] = &cp
	return nil
}

func (r *WorkflowRegistry) resolveLocked(id string) string {
	if _, ok := r.workflows[id]; ok {
		return id
	}
	if canonical, ok := legacyWorkflowIDs[id]; ok {
		return canonical
	}
	return id
}

// Get retrieves a workflow by ID; legacy built-in IDs resolve to their current workflow.
func (r *WorkflowRegistry) Get(id string) (*types.WorkflowDefinition, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	def, exists := r.workflows[r.resolveLocked(id)]
	if !exists {
		return nil, fmt.Errorf("workflow '%s' not registered", id)
	}
	return def, nil
}

// List returns all registered workflows: built-ins first, then custom ones in registration order.
func (r *WorkflowRegistry) List() []*types.WorkflowDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := make([]*types.WorkflowDefinition, 0, len(r.order))
	for _, id := range r.order {
		list = append(list, r.workflows[id])
	}
	return list
}

// Custom returns copies of the workflows that are not built in, for persistence.
func (r *WorkflowRegistry) Custom() []types.WorkflowDefinition {
	out := []types.WorkflowDefinition{}
	for _, def := range r.List() {
		if !def.Builtin {
			out = append(out, *def)
		}
	}
	return out
}

// StageIDs returns the workflow's stage IDs in order.
func StageIDs(def *types.WorkflowDefinition) []string {
	ids := make([]string, len(def.Stages))
	for i, s := range def.Stages {
		ids[i] = s.ID
	}
	return ids
}

// ValidateWorkflow checks the structural rules every workflow must meet: an ID, at least one stage
// and unique, non-empty stage IDs.
func ValidateWorkflow(def *types.WorkflowDefinition) error {
	if def == nil || def.ID == "" {
		return fmt.Errorf("workflow is missing its mandatory 'id' field")
	}
	if len(def.Stages) == 0 {
		return fmt.Errorf("workflow '%s' contains no stages", def.ID)
	}
	stageIDs := make(map[string]bool)
	for i, stage := range def.Stages {
		if stage.ID == "" {
			return fmt.Errorf("stage at index %d missing 'id'", i)
		}
		if stageIDs[stage.ID] {
			return fmt.Errorf("duplicate stage ID '%s' in workflow '%s'", stage.ID, def.ID)
		}
		stageIDs[stage.ID] = true
	}
	return nil
}

// ParseWorkflowYAML reads a workflow from a YAML file without registering it. The definition may
// sit at the top level or under a "workflow:" key (the Custom SDLC Builder's export format).
func ParseWorkflowYAML(filePath string) (*types.WorkflowDefinition, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read workflow file: %w", err)
	}
	var wrapped struct {
		Workflow *types.WorkflowDefinition `yaml:"workflow"`
	}
	var def *types.WorkflowDefinition
	if err := yaml.Unmarshal(data, &wrapped); err == nil && wrapped.Workflow != nil {
		def = wrapped.Workflow
	} else {
		def = &types.WorkflowDefinition{}
		if err := yaml.Unmarshal(data, def); err != nil {
			return nil, fmt.Errorf("failed to parse workflow YAML: %w", err)
		}
	}
	if err := ValidateWorkflow(def); err != nil {
		return nil, err
	}
	return def, nil
}

// LoadFromYAMLFile parses a custom workflow from a YAML file and registers it.
func (r *WorkflowRegistry) LoadFromYAMLFile(filePath string) (*types.WorkflowDefinition, error) {
	def, err := ParseWorkflowYAML(filePath)
	if err != nil {
		return nil, err
	}
	if err := r.Add(def, true); err != nil {
		return nil, err
	}
	return r.Get(def.ID)
}

// The built-in workflows use only stages the orchestrator can run (stageInstructions in
// internal/api). Every stage ends in operator review before the next one starts.

// GetGeneralAISDLCWorkflow returns the canonical 8-stage General AI SDLC pipeline.
func GetGeneralAISDLCWorkflow() *types.WorkflowDefinition {
	return &types.WorkflowDefinition{
		ID:          DefaultWorkflowID,
		Name:        "General AI SDLC (8-Stage)",
		Description: "Standard zero-trust software factory workflow with ATDD write-locking and rich evidence.",
		Version:     "1.0.0",
		DefaultRole: "orchestrator_agent",
		Stages: []types.WorkflowStage{
			{ID: "prd_discovery", Name: "PRD & Dynamic Repo Discovery", Type: types.StageTypeAutomated, AssignedRole: "product_manager", AllowedMethods: []string{"BMAD", "Supervisor"}, WriteLockWorkspace: true},
			{ID: "atdd_creation", Name: "ATDD Creation (Red Phase)", Type: types.StageTypeAutomated, AssignedRole: "qa_engineer", AllowedMethods: []string{"Supervisor", "BMAD"}, WriteLockWorkspace: true},
			{ID: "techdoc_rfc", Name: "Tech Doc / RFC Review (Gate)", Type: types.StageTypeReview, AssignedRole: "architect", AllowedMethods: []string{"BMAD"}, WriteLockWorkspace: true, RequiresGate: true},
			{ID: "task_breakdown", Name: "Task Breakdown & Planning", Type: types.StageTypeAutomated, AssignedRole: "planner", AllowedMethods: []string{"Supervisor"}, WriteLockWorkspace: true},
			{ID: "task_implementation", Name: "Implementation (Write-Unlocked)", Type: types.StageTypeAutomated, AssignedRole: "developer", AllowedMethods: []string{"BMAD", "ReAct", "Supervisor", "Superpower"}, WriteLockWorkspace: false},
			{ID: "e2e_validation", Name: "Automation & E2E Validation", Type: types.StageTypeAutomated, AssignedRole: "qa_engineer", AllowedMethods: []string{"Supervisor", "Superpower"}, WriteLockWorkspace: false},
			{ID: "uat_verification", Name: "Manual & UAT Verification", Type: types.StageTypeManual, AssignedRole: "product_manager", AllowedMethods: []string{"Supervisor"}, WriteLockWorkspace: false},
			{ID: "signoff_merge", Name: "Ready for Sign-Off & Merge", Type: types.StageTypeReview, AssignedRole: "release_manager", AllowedMethods: []string{"BMAD"}, WriteLockWorkspace: false, RequiresGate: true},
		},
	}
}

// GetHotfixFastTrackWorkflow returns a lightweight 3-stage hotfix pipeline.
func GetHotfixFastTrackWorkflow() *types.WorkflowDefinition {
	return &types.WorkflowDefinition{
		ID:          "fast_hotfix_sdlc",
		Name:        "Fast Hotfix SDLC (3-Stage)",
		Description: "Abbreviated lifecycle for high-priority bug fixes: reproduce with a failing test, fix, then run the regression suite.",
		Version:     "1.0.0",
		DefaultRole: "senior_dev",
		Stages: []types.WorkflowStage{
			{ID: "atdd_creation", Name: "Bug Reproduction & ATDD Spec", Type: types.StageTypeAutomated, AssignedRole: "qa_engineer", AllowedMethods: []string{"Supervisor"}, WriteLockWorkspace: true},
			{ID: "task_implementation", Name: "Targeted Code Fix", Type: types.StageTypeAutomated, AssignedRole: "developer", AllowedMethods: []string{"ReAct", "Superpower"}, WriteLockWorkspace: false},
			{ID: "e2e_validation", Name: "Regression Suite & Fast Merge", Type: types.StageTypeReview, AssignedRole: "release_manager", AllowedMethods: []string{"Supervisor"}, WriteLockWorkspace: false, RequiresGate: true},
		},
	}
}

// GetMicroserviceAPIWorkflow returns a contract-first 4-stage pipeline.
func GetMicroserviceAPIWorkflow() *types.WorkflowDefinition {
	return &types.WorkflowDefinition{
		ID:          "microservice_api",
		Name:        "Microservice API & Contract Pipeline (4-Stage)",
		Description: "Contract-first pipeline: schema design, consumer contract tests, implementation and cross-repo verification.",
		Version:     "1.0.0",
		DefaultRole: "architect",
		Stages: []types.WorkflowStage{
			{ID: "techdoc_rfc", Name: "OpenAPI & Protobuf Schema Spec", Type: types.StageTypeAutomated, AssignedRole: "architect", AllowedMethods: []string{"BMAD"}, WriteLockWorkspace: true},
			{ID: "atdd_creation", Name: "Consumer-Driven Contract Tests", Type: types.StageTypeAutomated, AssignedRole: "qa_engineer", AllowedMethods: []string{"Supervisor"}, WriteLockWorkspace: true},
			{ID: "task_implementation", Name: "Service Implementation", Type: types.StageTypeAutomated, AssignedRole: "developer", AllowedMethods: []string{"ReAct", "BMAD"}, WriteLockWorkspace: false},
			{ID: "e2e_validation", Name: "Cross-Repo Consumer Verification", Type: types.StageTypeReview, AssignedRole: "release_manager", AllowedMethods: []string{"Supervisor"}, WriteLockWorkspace: false, RequiresGate: true},
		},
	}
}
