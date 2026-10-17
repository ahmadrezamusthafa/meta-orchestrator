package fsm

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

func TestWorkflowRegistry(t *testing.T) {
	reg := NewWorkflowRegistry()

	general, err := reg.Get(DefaultWorkflowID)
	if err != nil || len(general.Stages) != 8 || !general.Builtin {
		t.Fatalf("expected the 8-stage built-in %s: %v %+v", DefaultWorkflowID, err, general)
	}
	for legacy, want := range map[string]int{"general-ai-sdlc": 8, "hotfix-fast-track": 3, "microservice-api": 4} {
		def, err := reg.Get(legacy)
		if err != nil || len(def.Stages) != want {
			t.Fatalf("legacy ID %s should resolve to a %d-stage workflow: %v", legacy, want, err)
		}
	}
	if err := reg.Add(&types.WorkflowDefinition{ID: "general-ai-sdlc", Stages: []types.WorkflowStage{{ID: "x"}}}, true); !errors.Is(err, ErrWorkflowExists) {
		t.Fatalf("a legacy built-in ID must not be replaceable, got %v", err)
	}
	if err := reg.Add(&types.WorkflowDefinition{ID: DefaultWorkflowID, Stages: []types.WorkflowStage{{ID: "x"}}}, true); !errors.Is(err, ErrWorkflowExists) {
		t.Fatalf("a built-in workflow must not be replaceable, got %v", err)
	}
	custom := &types.WorkflowDefinition{ID: "lean", Stages: []types.WorkflowStage{{ID: "a"}, {ID: "b"}}}
	if err := reg.Add(custom, false); err != nil {
		t.Fatal(err)
	}
	if err := reg.Add(custom, false); !errors.Is(err, ErrWorkflowExists) {
		t.Fatalf("duplicate custom ID without replace must conflict, got %v", err)
	}
	if err := reg.Add(&types.WorkflowDefinition{ID: "dup", Stages: []types.WorkflowStage{{ID: "a"}, {ID: "a"}}}, false); err == nil {
		t.Fatal("duplicate stage IDs must be rejected")
	}
	list := reg.List()
	if list[0].ID != DefaultWorkflowID || list[len(list)-1].ID != "lean" {
		t.Fatalf("List should keep built-ins first in a stable order, got %v", list)
	}
	if c := reg.Custom(); len(c) != 1 || c[0].ID != "lean" {
		t.Fatalf("Custom() = %+v", c)
	}
}

func TestCanTransitionMatchesLiveLifecycle(t *testing.T) {
	allowed := [][2]types.TaskState{
		{types.TaskStatePending, types.TaskStateRunning},
		{types.TaskStateRunning, types.TaskStateWaitingGateApproval},
		{types.TaskStateWaitingGateApproval, types.TaskStatePending},
		{types.TaskStateWaitingGateApproval, types.TaskStateRunning},
		{types.TaskStateWaitingGateApproval, types.TaskStateCompleted},
		{types.TaskStateFailed, types.TaskStateRunning},
		{types.TaskStateSuspended, types.TaskStateRunning},
		{types.TaskStateRunning, types.TaskStateSuspended},
		{types.TaskStateCompleted, types.TaskStatePending},
	}
	for _, tr := range allowed {
		if !CanTransition(tr[0], tr[1]) {
			t.Errorf("%s → %s should be allowed", tr[0], tr[1])
		}
	}
	denied := [][2]types.TaskState{
		{types.TaskStatePending, types.TaskStateWaitingGateApproval}, // no output to review
		{types.TaskStateRunning, types.TaskStateCompleted},           // must be reviewed first
		{types.TaskStateCompleted, types.TaskStateRunning},
	}
	for _, tr := range denied {
		if CanTransition(tr[0], tr[1]) {
			t.Errorf("%s → %s should be denied", tr[0], tr[1])
		}
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
	general, _ := reg.Get(DefaultWorkflowID)
	hydrator := NewSliceHydrator()

	slice := &types.StageSlice{
		StartStageID: "task_implementation",
		HaltStageID:  "e2e_validation",
		ProduceVideo: true,
	}

	plan, err := hydrator.HydrateSlice(general, slice, nil)
	if err != nil {
		t.Fatalf("failed to hydrate slice: %v", err)
	}

	if len(plan.EffectiveStages) != 2 {
		t.Fatalf("expected 2 effective stages, got %d", len(plan.EffectiveStages))
	}
	if plan.EffectiveStages[0].ID != "task_implementation" || plan.EffectiveStages[1].ID != "e2e_validation" {
		t.Errorf("unexpected effective stages %+v", plan.EffectiveStages)
	}
	if !plan.RequiresVideo {
		t.Errorf("expected ProduceVideo to be true")
	}

	// Test 2: Invalid slice range (start after halt)
	invalidSlice := &types.StageSlice{
		StartStageID: "e2e_validation",
		HaltStageID:  "task_implementation",
	}
	_, err = hydrator.HydrateSlice(general, invalidSlice, nil)
	if err == nil {
		t.Fatalf("expected error for inverted slice bounds")
	}
}

func TestFSMExecutionAndGateApproval(t *testing.T) {
	reg := NewWorkflowRegistry()
	general, _ := reg.Get(DefaultWorkflowID)
	store := NewMemoryTaskStateStore()
	ctx := context.Background()

	task := &types.Task{
		ID:             "task-fsm-test-1",
		WorkflowID:     general.ID,
		Title:          "Implement Order API",
		CurrentStageID: "techdoc_rfc",
		State:          types.TaskStatePending,
	}
	fsm, err := NewStateMachine(task, general, nil, func(t *types.Task) error {
		return store.SaveTask(ctx, t)
	}, nil)
	if err != nil {
		t.Fatalf("failed to create FSM: %v", err)
	}
	if err := fsm.CompleteCurrentStage(); err == nil {
		t.Fatal("a stage that is not running cannot complete")
	}
	if err := fsm.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if err := fsm.CompleteCurrentStage(); err != nil {
		t.Fatal(err)
	}
	if cur := fsm.GetTask(); cur.State != types.TaskStateWaitingGateApproval || cur.CurrentStageID != "techdoc_rfc" {
		t.Fatalf("a finished stage awaits review of its own output, got %s on %s", cur.State, cur.CurrentStageID)
	}
	if err := fsm.ApproveGate("lead_architect", "Tech RFC reviewed and approved"); err != nil {
		t.Fatalf("failed to approve gate: %v", err)
	}
	approved := fsm.GetTask()
	if approved.CurrentStageID != "task_breakdown" || approved.State != types.TaskStatePending {
		t.Errorf("approval should move to task_breakdown PENDING, got %s %s", approved.CurrentStageID, approved.State)
	}
	if approved.Metadata["gate_approved_by"] != "lead_architect" {
		t.Errorf("metadata missing gate_approved_by")
	}
	if saved, _ := store.GetTask(ctx, task.ID); saved == nil || saved.CurrentStageID != "task_breakdown" {
		t.Errorf("every transition is persisted")
	}
}

func TestCrashRecovery(t *testing.T) {
	reg := NewWorkflowRegistry()
	store := NewMemoryTaskStateStore()
	ctx := context.Background()

	seed := []*types.Task{
		{ID: "crash-recovery-task-99", WorkflowID: "general-ai-sdlc", Title: "Interrupted Task", CurrentStageID: "task_implementation", State: types.TaskStateRunning},
		{ID: "orphan", WorkflowID: "no-such-workflow", CurrentStageID: "task_implementation", State: types.TaskStatePending},
		{ID: "done", WorkflowID: DefaultWorkflowID, CurrentStageID: "signoff_merge", State: types.TaskStateCompleted},
	}
	for _, task := range seed {
		if err := store.SaveTask(ctx, task); err != nil {
			t.Fatalf("failed to seed task: %v", err)
		}
	}

	start := time.Now()
	recovered, skipped, err := RecoverActiveTasks(ctx, store, reg)
	if err != nil {
		t.Fatalf("crash recovery failed: %v", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Errorf("crash recovery exceeded 5s RTO")
	}
	if len(recovered) != 1 || recovered[0].GetTask().ID != "crash-recovery-task-99" || recovered[0].GetTask().CurrentStageIndex != 4 {
		t.Fatalf("expected the interrupted task restored at stage index 4, got %d", len(recovered))
	}
	if skipped["orphan"] == nil || len(skipped) != 1 {
		t.Fatalf("a task with an unknown workflow must be reported, got %v", skipped)
	}
	if err := store.DeleteTask(ctx, "crash-recovery-task-99"); err != nil {
		t.Fatal(err)
	}
	if active, _ := store.ListActiveTasks(ctx); len(active) != 1 {
		t.Fatalf("deleted tasks are no longer active, got %d", len(active))
	}
}
