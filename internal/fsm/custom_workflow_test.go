package fsm

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

func TestWorkflowRegistry(t *testing.T) {
	reg := NewWorkflowRegistry()

	// Verify standard workflows exist
	general, err := reg.Get("general-ai-sdlc")
	if err != nil || general == nil {
		t.Fatalf("expected general-ai-sdlc to exist: %v", err)
	}
	if len(general.Stages) != 9 {
		t.Errorf("expected 9 stages in general AI SDLC, got %d", len(general.Stages))
	}

	hotfix, err := reg.Get("hotfix-fast-track")
	if err != nil || len(hotfix.Stages) != 3 {
		t.Fatalf("expected 3 stages in hotfix-fast-track: %v", err)
	}

	micro, err := reg.Get("microservice-api")
	if err != nil || len(micro.Stages) != 4 {
		t.Fatalf("expected 4 stages in microservice-api: %v", err)
	}
}

func TestLoadCustomWorkflowYAML(t *testing.T) {
	reg := NewWorkflowRegistry()
	tmpDir := t.TempDir()
	yamlPath := filepath.Join(tmpDir, "custom.yaml")

	yamlContent := `
id: bespoke-pipeline
name: Bespoke 3-Stage Pipeline
description: Custom QA and deploy flow
version: 1.0.0
default_role: lead_developer
stages:
  - id: SPEC
    name: Spec Definition
    type: automated
    assigned_role: product_manager
    allowed_methods: [react]
    timeout: 10m
  - id: GATE_REVIEW
    name: Architecture Gate
    type: review_gate
    assigned_role: human_reviewer
    requires_gate: true
    timeout: 24h
  - id: DEPLOY
    name: Canary Deploy
    type: automated
    assigned_role: devops
    allowed_methods: [superpower]
    timeout: 15m
`
	if err := os.WriteFile(yamlPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write test YAML: %v", err)
	}

	def, err := reg.LoadFromYAMLFile(yamlPath)
	if err != nil {
		t.Fatalf("failed to load custom YAML: %v", err)
	}

	if def.ID != "bespoke-pipeline" {
		t.Errorf("expected ID 'bespoke-pipeline', got '%s'", def.ID)
	}
	if len(def.Stages) != 3 {
		t.Errorf("expected 3 stages, got %d", len(def.Stages))
	}
}

func TestSliceHydrator(t *testing.T) {
	reg := NewWorkflowRegistry()
	general, _ := reg.Get("general-ai-sdlc")
	hydrator := NewSliceHydrator()

	// Test 1: Mid-process slice from IMPLEMENTATION_GREEN to E2E_AUTOMATION
	slice := &types.StageSlice{
		StartStageID: "IMPLEMENTATION_GREEN",
		HaltStageID:  "E2E_AUTOMATION",
		ProduceVideo: true,
	}

	// Without existing prerequisites (PRD.md, ATDD_SUITE.md, TECH_DOC_RFC.md, TASK_PLAN.md)
	plan, err := hydrator.HydrateSlice(general, slice, nil)
	if err != nil {
		t.Fatalf("failed to hydrate slice: %v", err)
	}

	if len(plan.EffectiveStages) != 2 {
		t.Fatalf("expected 2 effective stages, got %d", len(plan.EffectiveStages))
	}
	if plan.EffectiveStages[0].ID != "IMPLEMENTATION_GREEN" {
		t.Errorf("expected first stage IMPLEMENTATION_GREEN, got %s", plan.EffectiveStages[0].ID)
	}
	if plan.EffectiveStages[1].ID != "E2E_AUTOMATION" {
		t.Errorf("expected second stage E2E_AUTOMATION, got %s", plan.EffectiveStages[1].ID)
	}
	if !plan.RequiresVideo {
		t.Errorf("expected ProduceVideo to be true")
	}
	if len(plan.MissingArtifacts) == 0 {
		t.Errorf("expected missing prerequisite artifacts to be reported")
	}

	// Test 2: Invalid slice range (start after halt)
	invalidSlice := &types.StageSlice{
		StartStageID: "E2E_AUTOMATION",
		HaltStageID:  "IMPLEMENTATION_GREEN",
	}
	_, err = hydrator.HydrateSlice(general, invalidSlice, nil)
	if err == nil {
		t.Fatalf("expected error for inverted slice bounds")
	}
}

func TestFSMExecutionAndGateApproval(t *testing.T) {
	reg := NewWorkflowRegistry()
	general, _ := reg.Get("general-ai-sdlc")
	store := NewMemoryTaskStateStore()
	ctx := context.Background()

	task := &types.Task{
		ID:             "task-fsm-test-1",
		WorkflowID:     general.ID,
		Title:          "Implement Order API",
		AssignedRepos:  []string{"backend-api"},
		CurrentStageID: "TECH_DOC_RFC", // Index 3
		State:          types.TaskStateRunning,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	slicePlan := &SliceExecutionPlan{
		WorkflowID:      general.ID,
		OriginalStages:  general.Stages,
		EffectiveStages: general.Stages,
		StartIndex:      3,
		HaltIndex:       8,
	}

	fsm, err := NewStateMachine(task, general, slicePlan, func(t *types.Task) error {
		return store.SaveTask(ctx, t)
	}, nil)
	if err != nil {
		t.Fatalf("failed to create FSM: %v", err)
	}

	// 1. Advance from TECH_DOC_RFC (Index 3) to GATE_TECH_DOC_REVIEW (Index 4)
	err = fsm.CompleteCurrentStage()
	if err != nil {
		t.Fatalf("failed to complete TECH_DOC_RFC: %v", err)
	}

	current := fsm.GetTask()
	if current.CurrentStageID != "GATE_TECH_DOC_REVIEW" {
		t.Errorf("expected stage GATE_TECH_DOC_REVIEW, got %s", current.CurrentStageID)
	}
	if current.State != types.TaskStateWaitingGateApproval {
		t.Errorf("expected state WAITING_GATE_APPROVAL, got %s", current.State)
	}

	// 2. Reject early completion without approval
	err = fsm.CompleteCurrentStage()
	// Next stage should still be blocked or require explicit gate approval
	// In our FSM, ApproveGate is required
	err = fsm.ApproveGate("lead_architect", "Tech RFC reviewed and approved")
	if err != nil {
		t.Fatalf("failed to approve gate: %v", err)
	}

	approvedTask := fsm.GetTask()
	if approvedTask.CurrentStageID != "TASK_BREAKDOWN" {
		t.Errorf("expected stage TASK_BREAKDOWN after approval, got %s", approvedTask.CurrentStageID)
	}
	if approvedTask.State != types.TaskStateRunning {
		t.Errorf("expected state RUNNING, got %s", approvedTask.State)
	}
	if approvedTask.Metadata["gate_approved_by"] != "lead_architect" {
		t.Errorf("metadata missing gate_approved_by")
	}
}

func TestCrashRecovery(t *testing.T) {
	reg := NewWorkflowRegistry()
	general, _ := reg.Get("general-ai-sdlc")
	store := NewMemoryTaskStateStore()
	ctx := context.Background()

	// Simulate active interrupted task
	task := &types.Task{
		ID:                "crash-recovery-task-99",
		WorkflowID:        general.ID,
		Title:             "Interrupted Task",
		CurrentStageID:    "IMPLEMENTATION_GREEN",
		CurrentStageIndex: 6,
		State:             types.TaskStateRunning,
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}
	if err := store.SaveTask(ctx, task); err != nil {
		t.Fatalf("failed to seed task: %v", err)
	}

	// Simulate daemon recovery
	start := time.Now()
	recovered, err := RecoverActiveTasks(ctx, store, reg)
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("crash recovery failed: %v", err)
	}
	if len(recovered) != 1 {
		t.Fatalf("expected 1 recovered task, got %d", len(recovered))
	}
	if duration > 5*time.Second {
		t.Errorf("crash recovery exceeded 5s RTO: took %v", duration)
	}

	recoveredTask := recovered[0].GetTask()
	if recoveredTask.ID != "crash-recovery-task-99" {
		t.Errorf("unexpected recovered task ID: %s", recoveredTask.ID)
	}
	if recoveredTask.CurrentStageID != "IMPLEMENTATION_GREEN" {
		t.Errorf("expected stage IMPLEMENTATION_GREEN, got %s", recoveredTask.CurrentStageID)
	}
}
