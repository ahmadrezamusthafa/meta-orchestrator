package fsm

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// StatePersistenceCallback is called whenever task state updates.
type StatePersistenceCallback func(task *types.Task) error

// StateTransitionHook is invoked before/after stage changes.
type StateTransitionHook func(task *types.Task, fromStage string, toStage string) error

// StateMachine manages runtime execution of an orchestrated task across SDLC stages.
type StateMachine struct {
	mu           sync.RWMutex
	workflow     *types.WorkflowDefinition
	slicePlan    *SliceExecutionPlan
	task         *types.Task
	onPersist    StatePersistenceCallback
	onTransition StateTransitionHook
}

// NewStateMachine instantiates an FSM for the given task and workflow definition.
func NewStateMachine(
	task *types.Task,
	workflow *types.WorkflowDefinition,
	slicePlan *SliceExecutionPlan,
	onPersist StatePersistenceCallback,
	onTransition StateTransitionHook,
) (*StateMachine, error) {
	if task == nil {
		return nil, fmt.Errorf("task cannot be nil")
	}
	if workflow == nil {
		return nil, fmt.Errorf("workflow cannot be nil")
	}
	if slicePlan == nil {
		hydrator := NewSliceHydrator()
		var err error
		slicePlan, err = hydrator.HydrateSlice(workflow, task.ActiveSlice, nil)
		if err != nil {
			return nil, err
		}
	}

	fsm := &StateMachine{
		workflow:     workflow,
		slicePlan:    slicePlan,
		task:         task,
		onPersist:    onPersist,
		onTransition: onTransition,
	}

	// Initialize task stage if not set
	if task.CurrentStageID == "" {
		task.CurrentStageIndex = slicePlan.StartIndex
		task.CurrentStageID = workflow.Stages[slicePlan.StartIndex].ID
		task.State = types.TaskStatePending
	} else {
		found := false
		for i, s := range workflow.Stages {
			if s.ID == task.CurrentStageID {
				task.CurrentStageIndex = i
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("task current stage '%s' not found in workflow '%s'", task.CurrentStageID, workflow.ID)
		}
	}

	return fsm, nil
}

// CurrentStage returns the active stage metadata.
func (f *StateMachine) CurrentStage() *types.WorkflowStage {
	f.mu.RLock()
	defer f.mu.RUnlock()

	for i := range f.workflow.Stages {
		if f.workflow.Stages[i].ID == f.task.CurrentStageID {
			return &f.workflow.Stages[i]
		}
	}
	return nil
}

// Start begins execution of the current stage.
func (f *StateMachine) Start(ctx context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	stage := f.CurrentStageUnlocked()
	if stage == nil {
		return fmt.Errorf("current stage '%s' not found in workflow", f.task.CurrentStageID)
	}

	if stage.Type == types.StageTypeReview && stage.RequiresGate {
		f.task.State = types.TaskStateWaitingGateApproval
	} else {
		f.task.State = types.TaskStateRunning
	}
	f.task.UpdatedAt = time.Now()

	return f.persist()
}

// ApproveGate releases an execution blocked on human approval.
func (f *StateMachine) ApproveGate(approver string, notes string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.task.State != types.TaskStateWaitingGateApproval {
		return fmt.Errorf("task %s is not waiting for gate approval (state: %s)", f.task.ID, f.task.State)
	}

	if f.task.Metadata == nil {
		f.task.Metadata = make(map[string]string)
	}
	f.task.Metadata["gate_approved_by"] = approver
	f.task.Metadata["gate_approval_notes"] = notes
	f.task.Metadata["gate_approved_at"] = time.Now().Format(time.RFC3339)

	// Gate approved, advance to next stage
	return f.advanceToNextStageUnlocked()
}

// CompleteCurrentStage marks current stage finished and advances or halts.
func (f *StateMachine) CompleteCurrentStage() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.task.State == types.TaskStateWaitingGateApproval {
		return fmt.Errorf("cannot complete stage %s: waiting for human gate approval", f.task.CurrentStageID)
	}

	return f.advanceToNextStageUnlocked()
}

// FailCurrentStage marks the task as failed.
func (f *StateMachine) FailCurrentStage(reason string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.task.State = types.TaskStateFailed
	if f.task.Metadata == nil {
		f.task.Metadata = make(map[string]string)
	}
	f.task.Metadata["failure_reason"] = reason
	f.task.UpdatedAt = time.Now()

	return f.persist()
}

// BlockFrustration transitions the task to blocked frustration state due to repetition.
func (f *StateMachine) BlockFrustration(signature string, consecutive int) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.task.State = types.TaskStateBlockedFrustration
	if f.task.Metadata == nil {
		f.task.Metadata = make(map[string]string)
	}
	f.task.Metadata["frustration_signature"] = signature
	f.task.Metadata["consecutive_failures"] = fmt.Sprintf("%d", consecutive)
	f.task.UpdatedAt = time.Now()

	return f.persist()
}

// ResumeFromFrustration clears the block and resumes execution.
func (f *StateMachine) ResumeFromFrustration() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.task.State != types.TaskStateBlockedFrustration {
		return fmt.Errorf("task %s is not blocked in frustration", f.task.ID)
	}

	f.task.State = types.TaskStateRunning
	f.task.UpdatedAt = time.Now()
	return f.persist()
}

// GetTask returns a copy of current task state.
func (f *StateMachine) GetTask() types.Task {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return *f.task
}

func (f *StateMachine) CurrentStageUnlocked() *types.WorkflowStage {
	for i := range f.workflow.Stages {
		if f.workflow.Stages[i].ID == f.task.CurrentStageID {
			return &f.workflow.Stages[i]
		}
	}
	return nil
}

func (f *StateMachine) advanceToNextStageUnlocked() error {
	prevStageID := f.task.CurrentStageID

	// Check if this was the halt stage for the active slice
	if f.task.CurrentStageIndex >= f.slicePlan.HaltIndex {
		f.task.State = types.TaskStateCompleted
		f.task.UpdatedAt = time.Now()
		if f.onTransition != nil {
			_ = f.onTransition(f.task, prevStageID, "COMPLETED")
		}
		return f.persist()
	}

	nextIdx := f.task.CurrentStageIndex + 1
	if nextIdx >= len(f.workflow.Stages) {
		f.task.State = types.TaskStateCompleted
		f.task.UpdatedAt = time.Now()
		if f.onTransition != nil {
			_ = f.onTransition(f.task, prevStageID, "COMPLETED")
		}
		return f.persist()
	}

	nextStage := f.workflow.Stages[nextIdx]
	f.task.CurrentStageIndex = nextIdx
	f.task.CurrentStageID = nextStage.ID

	if nextStage.Type == types.StageTypeReview && nextStage.RequiresGate {
		f.task.State = types.TaskStateWaitingGateApproval
	} else {
		f.task.State = types.TaskStateRunning
	}
	f.task.UpdatedAt = time.Now()

	if f.onTransition != nil {
		_ = f.onTransition(f.task, prevStageID, nextStage.ID)
	}

	return f.persist()
}

func (f *StateMachine) persist() error {
	if f.onPersist != nil {
		return f.onPersist(f.task)
	}
	return nil
}
