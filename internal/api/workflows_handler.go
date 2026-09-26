package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

var defaultWorkflows = []types.WorkflowDefinition{
	{
		ID:          "general_ai_sdlc",
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
	},
	{
		ID:          "fast_hotfix_sdlc",
		Name:        "Fast Hotfix SDLC (3-Stage)",
		Description: "Abbreviated lifecycle for high-priority bug fixes directly entering implementation and E2E verification.",
		Version:     "1.0.0",
		DefaultRole: "senior_dev",
		Stages: []types.WorkflowStage{
			{ID: "hotfix_reproduction", Name: "Bug Reproduction & ATDD Spec", Type: types.StageTypeAutomated, AssignedRole: "qa_engineer", AllowedMethods: []string{"Supervisor"}, WriteLockWorkspace: true},
			{ID: "hotfix_implementation", Name: "Targeted Code Fix", Type: types.StageTypeAutomated, AssignedRole: "developer", AllowedMethods: []string{"ReAct", "Superpower"}, WriteLockWorkspace: false},
			{ID: "hotfix_validation", Name: "Regression Suite & Fast Merge", Type: types.StageTypeReview, AssignedRole: "release_manager", AllowedMethods: []string{"Supervisor"}, WriteLockWorkspace: false, RequiresGate: true},
		},
	},
	{
		ID:          "microservice_api",
		Name:        "Microservice API & Contract Pipeline (4-Stage)",
		Description: "Contract-first pipeline for distributed microservices with automated schema validation and consumer testing.",
		Version:     "1.0.0",
		DefaultRole: "architect",
		Stages: []types.WorkflowStage{
			{ID: "contract_spec", Name: "OpenAPI & Protobuf Schema Spec", Type: types.StageTypeAutomated, AssignedRole: "architect", AllowedMethods: []string{"BMAD"}, WriteLockWorkspace: true},
			{ID: "contract_tests", Name: "Consumer-Driven Contract Tests", Type: types.StageTypeAutomated, AssignedRole: "qa_engineer", AllowedMethods: []string{"Supervisor"}, WriteLockWorkspace: true},
			{ID: "mock_implementation", Name: "Service Mock Implementation", Type: types.StageTypeAutomated, AssignedRole: "developer", AllowedMethods: []string{"ReAct", "BMAD"}, WriteLockWorkspace: false},
			{ID: "consumer_verification", Name: "Cross-Repo Consumer Verification", Type: types.StageTypeReview, AssignedRole: "release_manager", AllowedMethods: []string{"Supervisor"}, WriteLockWorkspace: false, RequiresGate: true},
		},
	},
}

func (r *Router) handleWorkflows(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		r.writeJSON(w, http.StatusOK, defaultWorkflows)

	case http.MethodPost:
		var wf types.WorkflowDefinition
		if err := json.NewDecoder(req.Body).Decode(&wf); err != nil {
			r.writeError(w, http.StatusBadRequest, "Invalid workflow JSON")
			return
		}
		if wf.ID == "" || wf.Name == "" || len(wf.Stages) == 0 {
			r.writeError(w, http.StatusBadRequest, "Workflow requires ID, Name, and at least 1 Stage")
			return
		}

		defaultWorkflows = append(defaultWorkflows, wf)
		r.writeJSON(w, http.StatusCreated, map[string]interface{}{
			"status":      "created",
			"workflow_id": wf.ID,
			"timestamp":   time.Now(),
		})

	default:
		r.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}
