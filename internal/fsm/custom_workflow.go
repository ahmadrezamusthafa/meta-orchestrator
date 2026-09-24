package fsm

import (
	"fmt"
	"os"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
	"gopkg.in/yaml.v3"
)

// WorkflowRegistry manages built-in and user-defined custom workflows.
type WorkflowRegistry struct {
	workflows map[string]*types.WorkflowDefinition
}

// NewWorkflowRegistry initializes a registry with standard built-in workflows.
func NewWorkflowRegistry() *WorkflowRegistry {
	reg := &WorkflowRegistry{
		workflows: make(map[string]*types.WorkflowDefinition),
	}
	reg.Register(GetGeneralAISDLCWorkflow())
	reg.Register(GetHotfixFastTrackWorkflow())
	reg.Register(GetMicroserviceAPIWorkflow())
	return reg
}

// Register registers a workflow definition.
func (r *WorkflowRegistry) Register(def *types.WorkflowDefinition) {
	r.workflows[def.ID] = def
}

// Get retrieves a workflow by ID.
func (r *WorkflowRegistry) Get(id string) (*types.WorkflowDefinition, error) {
	def, exists := r.workflows[id]
	if !exists {
		return nil, fmt.Errorf("workflow '%s' not registered", id)
	}
	return def, nil
}

// List returns all registered workflows.
func (r *WorkflowRegistry) List() []*types.WorkflowDefinition {
	list := make([]*types.WorkflowDefinition, 0, len(r.workflows))
	for _, w := range r.workflows {
		list = append(list, w)
	}
	return list
}

// LoadFromYAMLFile parses a custom workflow from a YAML file.
func (r *WorkflowRegistry) LoadFromYAMLFile(filePath string) (*types.WorkflowDefinition, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read workflow file: %w", err)
	}

	var def types.WorkflowDefinition
	if err := yaml.Unmarshal(data, &def); err != nil {
		return nil, fmt.Errorf("failed to parse workflow YAML: %w", err)
	}

	if def.ID == "" {
		return nil, fmt.Errorf("workflow YAML missing mandatory 'id' field")
	}
	if len(def.Stages) == 0 {
		return nil, fmt.Errorf("workflow '%s' contains no stages", def.ID)
	}

	// Validate stage IDs are unique
	stageIDs := make(map[string]bool)
	for i, stage := range def.Stages {
		if stage.ID == "" {
			return nil, fmt.Errorf("stage at index %d missing 'id'", i)
		}
		if stageIDs[stage.ID] {
			return nil, fmt.Errorf("duplicate stage ID '%s' in workflow '%s'", stage.ID, def.ID)
		}
		stageIDs[stage.ID] = true
	}

	r.Register(&def)
	return &def, nil
}

// GetGeneralAISDLCWorkflow returns the canonical 9-stage General AI SDLC pipeline.
func GetGeneralAISDLCWorkflow() *types.WorkflowDefinition {
	return &types.WorkflowDefinition{
		ID:          "general-ai-sdlc",
		Name:        "General AI SDLC",
		Description: "Comprehensive 9-stage lifecycle with AST-informed ATDD, HITL approval gate, and Playwright evidence",
		Version:     "1.0.0",
		DefaultRole: "product_manager",
		Stages: []types.WorkflowStage{
			{
				ID:                 "INTAKE_PRD",
				Name:               "Product Requirements Synthesis",
				Type:               types.StageTypeAutomated,
				AssignedRole:       "product_manager",
				AllowedMethods:     []string{"bmad", "react"},
				WriteLockWorkspace: true,
				RequiresGate:       false,
				Timeout:            15 * time.Minute,
				RequiredArtifacts:  []string{"PRD.md"},
			},
			{
				ID:                 "REPO_DISCOVERY",
				Name:               "Dynamic Repository Discovery",
				Type:               types.StageTypeAutomated,
				AssignedRole:       "system_architect",
				AllowedMethods:     []string{"supervisor", "react"},
				WriteLockWorkspace: true,
				RequiresGate:       false,
				Timeout:            10 * time.Minute,
			},
			{
				ID:                 "ATDD_RED_PHASE",
				Name:               "AST-Informed ATDD Generation",
				Type:               types.StageTypeAutomated,
				AssignedRole:       "atdd_qa_engineer",
				AllowedMethods:     []string{"bmad"},
				WriteLockWorkspace: false, // Can write test files only
				RequiresGate:       false,
				Timeout:            25 * time.Minute,
				RequiredArtifacts:  []string{"ATDD_SUITE.md"},
			},
			{
				ID:                 "TECH_DOC_RFC",
				Name:               "Tech Doc & RFC Architecture",
				Type:               types.StageTypeAutomated,
				AssignedRole:       "system_architect",
				AllowedMethods:     []string{"bmad", "supervisor"},
				WriteLockWorkspace: true,
				RequiresGate:       false,
				Timeout:            20 * time.Minute,
				RequiredArtifacts:  []string{"TECH_DOC_RFC.md"},
			},
			{
				ID:                 "GATE_TECH_DOC_REVIEW",
				Name:               "HITL Tech Doc Review Gate",
				Type:               types.StageTypeReview,
				AssignedRole:       "human_reviewer",
				AllowedMethods:     []string{"hitl"},
				WriteLockWorkspace: true,
				RequiresGate:       true,
				GateCriteria:       "Review and approve TECH_DOC_RFC.md architecture and schema decisions",
				Timeout:            72 * time.Hour,
			},
			{
				ID:                 "TASK_BREAKDOWN",
				Name:               "Atomic Task Breakdown",
				Type:               types.StageTypeAutomated,
				AssignedRole:       "system_architect",
				AllowedMethods:     []string{"bmad", "supervisor"},
				WriteLockWorkspace: true,
				RequiresGate:       false,
				Timeout:            15 * time.Minute,
				RequiredArtifacts:  []string{"TASK_PLAN.md"},
			},
			{
				ID:                 "IMPLEMENTATION_GREEN",
				Name:               "Implementation (Green Phase)",
				Type:               types.StageTypeAutomated,
				AssignedRole:       "lead_developer",
				AllowedMethods:     []string{"bmad", "supervisor", "react", "superpower"},
				WriteLockWorkspace: false, // Write unlocked after ATDD Red verified
				RequiresGate:       false,
				Timeout:            60 * time.Minute,
			},
			{
				ID:                 "E2E_AUTOMATION",
				Name:               "E2E Automation & Regression",
				Type:               types.StageTypeAutomated,
				AssignedRole:       "atdd_qa_engineer",
				AllowedMethods:     []string{"bmad", "supervisor"},
				WriteLockWorkspace: true,
				RequiresGate:       false,
				Timeout:            30 * time.Minute,
			},
			{
				ID:                 "UAT_EVIDENCE",
				Name:               "UAT Manual & Video Evidence",
				Type:               types.StageTypeManual,
				AssignedRole:       "atdd_qa_engineer",
				AllowedMethods:     []string{"bmad", "superpower"},
				WriteLockWorkspace: true,
				RequiresGate:       false,
				Timeout:            30 * time.Minute,
				RequiredArtifacts:  []string{"UAT_PREPARATION.md", "EVIDENCE.md"},
			},
		},
	}
}

// GetHotfixFastTrackWorkflow returns a lightweight 3-stage hotfix pipeline.
func GetHotfixFastTrackWorkflow() *types.WorkflowDefinition {
	return &types.WorkflowDefinition{
		ID:          "hotfix-fast-track",
		Name:        "Hotfix Fast-Track",
		Description: "Rapid 3-stage pipeline for emergency fixes with instant regression verification",
		Version:     "1.0.0",
		DefaultRole: "lead_developer",
		Stages: []types.WorkflowStage{
			{
				ID:                 "DIAGNOSE_BUG",
				Name:               "Diagnose & Reproduce",
				Type:               types.StageTypeAutomated,
				AssignedRole:       "lead_developer",
				AllowedMethods:     []string{"react", "superpower"},
				WriteLockWorkspace: true,
				RequiresGate:       false,
				Timeout:            15 * time.Minute,
				RequiredArtifacts:  []string{"BUG_DIAGNOSIS.md"},
			},
			{
				ID:                 "PATCH_IMPLEMENTATION",
				Name:               "Apply Hotfix Patch",
				Type:               types.StageTypeAutomated,
				AssignedRole:       "lead_developer",
				AllowedMethods:     []string{"react", "superpower"},
				WriteLockWorkspace: false,
				RequiresGate:       false,
				Timeout:            20 * time.Minute,
			},
			{
				ID:                 "VERIFY_REGRESSION",
				Name:               "Verify Hotfix & Regression Tests",
				Type:               types.StageTypeAutomated,
				AssignedRole:       "atdd_qa_engineer",
				AllowedMethods:     []string{"react"},
				WriteLockWorkspace: true,
				RequiresGate:       false,
				Timeout:            15 * time.Minute,
				RequiredArtifacts:  []string{"EVIDENCE.md"},
			},
		},
	}
}

// GetMicroserviceAPIWorkflow returns a contract-first 4-stage pipeline.
func GetMicroserviceAPIWorkflow() *types.WorkflowDefinition {
	return &types.WorkflowDefinition{
		ID:          "microservice-api",
		Name:        "Microservice API Contract Workflow",
		Description: "OpenAPI/gRPC contract-first pipeline with mock testing and code generation",
		Version:     "1.0.0",
		DefaultRole: "system_architect",
		Stages: []types.WorkflowStage{
			{
				ID:                 "CONTRACT_SPEC",
				Name:               "API Contract Specification",
				Type:               types.StageTypeAutomated,
				AssignedRole:       "system_architect",
				AllowedMethods:     []string{"react", "supervisor"},
				WriteLockWorkspace: true,
				RequiresGate:       false,
				Timeout:            20 * time.Minute,
				RequiredArtifacts:  []string{"API_SPEC.yaml"},
			},
			{
				ID:                 "MOCK_ATDD",
				Name:               "Mock ATDD Test Suite",
				Type:               types.StageTypeAutomated,
				AssignedRole:       "atdd_qa_engineer",
				AllowedMethods:     []string{"bmad"},
				WriteLockWorkspace: false,
				RequiresGate:       false,
				Timeout:            25 * time.Minute,
				RequiredArtifacts:  []string{"ATDD_SUITE.md"},
			},
			{
				ID:                 "CODEGEN_IMPLEMENT",
				Name:               "API Implementation",
				Type:               types.StageTypeAutomated,
				AssignedRole:       "lead_developer",
				AllowedMethods:     []string{"bmad", "supervisor"},
				WriteLockWorkspace: false,
				RequiresGate:       false,
				Timeout:            45 * time.Minute,
			},
			{
				ID:                 "CONTRACT_VERIFY",
				Name:               "Contract Compliance Verification",
				Type:               types.StageTypeAutomated,
				AssignedRole:       "atdd_qa_engineer",
				AllowedMethods:     []string{"react"},
				WriteLockWorkspace: true,
				RequiresGate:       false,
				Timeout:            20 * time.Minute,
				RequiredArtifacts:  []string{"EVIDENCE.md"},
			},
		},
	}
}
