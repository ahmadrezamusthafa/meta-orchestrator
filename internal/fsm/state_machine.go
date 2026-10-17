package fsm

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// transitions is the task lifecycle: the states a task may move to from each state. It is the single
// source of truth for both the live stage controller (internal/api) and StateMachine.
//
//	PENDING / SUSPENDED / FAILED / WAITING_GATE_APPROVAL(reject) --run--> RUNNING
//	RUNNING --success--> WAITING_GATE_APPROVAL --approve--> next stage PENDING … last stage → COMPLETED
//	RUNNING --error--> FAILED        any active state --pause--> SUSPENDED
//	any active state --unmet deps--> WAITING_DEPENDENCY
//
// Operators may also close an idle task (→ COMPLETED) or reopen a completed one (→ PENDING).
var transitions = map[types.TaskState][]types.TaskState{
	types.TaskStatePending: {types.TaskStateRunning, types.TaskStateWaitingDependency, types.TaskStateSuspended,
		types.TaskStateCompleted, types.TaskStateFailed},
	types.TaskStateRunning: {types.TaskStateWaitingGateApproval, types.TaskStateFailed, types.TaskStateSuspended,
		types.TaskStateBlockedFrustration},
	types.TaskStateWaitingGateApproval: {types.TaskStateRunning, types.TaskStatePending, types.TaskStateCompleted,
		types.TaskStateSuspended, types.TaskStateFailed, types.TaskStateWaitingDependency},
	types.TaskStateFailed: {types.TaskStateRunning, types.TaskStatePending, types.TaskStateSuspended,
		types.TaskStateWaitingDependency, types.TaskStateCompleted},
	types.TaskStateSuspended: {types.TaskStateRunning, types.TaskStatePending, types.TaskStateWaitingDependency,
		types.TaskStateCompleted, types.TaskStateFailed},
	types.TaskStateWaitingDependency: {types.TaskStateRunning, types.TaskStatePending, types.TaskStateSuspended,
		types.TaskStateCompleted},
	types.TaskStateBlockedFrustration: {types.TaskStateRunning, types.TaskStatePending, types.TaskStateSuspended,
		types.TaskStateFailed, types.TaskStateCompleted},
	types.TaskStateCompleted: {types.TaskStatePending},
}

// CanTransition reports whether a task may move from one state to another. Staying in the same
// state is always allowed.
func CanTransition(from, to types.TaskState) bool {
	if from == to {
		return true
	}
	for _, s := range transitions[from] {
		if s == to {
			return true
		}
	}
	return false
}

// StatePersistenceCallback is called whenever task state updates.
type StatePersistenceCallback func(task *types.Task) error

// StateTransitionHook is invoked before/after stage changes.
type StateTransitionHook func(task *types.Task, fromStage string, toStage string) error

// StateMachine manages runtime execution of an orchestrated task across SDLC stages, following the
// same lifecycle as the live stage controller: every stage ends in operator review.
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
	return f.CurrentStageUnlocked()
}

// Start begins execution of the current stage.
func (f *StateMachine) Start(ctx context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.CurrentStageUnlocked() == nil {
		return fmt.Errorf("current stage '%s' not found in workflow", f.task.CurrentStageID)
	}
	if err := f.setStateUnlocked(types.TaskStateRunning); err != nil {
		return err
	}
	return f.persist()
}

// CompleteCurrentStage records that the current stage's turn finished; its output now awaits review.
func (f *StateMachine) CompleteCurrentStage() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.task.State != types.TaskStateRunning {
		return fmt.Errorf("cannot complete stage %s: task is %s, not running", f.task.CurrentStageID, f.task.State)
	}
	if err := f.setStateUnlocked(types.TaskStateWaitingGateApproval); err != nil {
		return err
	}
	return f.persist()
}

// ApproveGate approves the reviewed stage: the task moves to the next stage (pending until it runs)
// or completes after the slice's last stage.
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

	return f.advanceToNextStageUnlocked()
}

// FailCurrentStage marks the task as failed.
func (f *StateMachine) FailCurrentStage(reason string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if err := f.setStateUnlocked(types.TaskStateFailed); err != nil {
		return err
	}
	if f.task.Metadata == nil {
		f.task.Metadata = make(map[string]string)
	}
	f.task.Metadata["failure_reason"] = reason
	return f.persist()
}

// BlockFrustration transitions the task to blocked frustration state due to repetition.
func (f *StateMachine) BlockFrustration(signature string, consecutive int) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if err := f.setStateUnlocked(types.TaskStateBlockedFrustration); err != nil {
		return err
	}
	if f.task.Metadata == nil {
		f.task.Metadata = make(map[string]string)
	}
	f.task.Metadata["frustration_signature"] = signature
	f.task.Metadata["consecutive_failures"] = fmt.Sprintf("%d", consecutive)
	return f.persist()
}

// ResumeFromFrustration clears the block and resumes execution.
func (f *StateMachine) ResumeFromFrustration() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.task.State != types.TaskStateBlockedFrustration {
		return fmt.Errorf("task %s is not blocked in frustration", f.task.ID)
	}
	if err := f.setStateUnlocked(types.TaskStateRunning); err != nil {
		return err
	}
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

func (f *StateMachine) setStateUnlocked(to types.TaskState) error {
	if !CanTransition(f.task.State, to) {
		return fmt.Errorf("task %s cannot move from %s to %s", f.task.ID, f.task.State, to)
	}
	f.task.State = to
	f.task.UpdatedAt = time.Now()
	return nil
}

func (f *StateMachine) advanceToNextStageUnlocked() error {
	prevStageID := f.task.CurrentStageID
	nextIdx := f.task.CurrentStageIndex + 1

	if f.task.CurrentStageIndex >= f.slicePlan.HaltIndex || nextIdx >= len(f.workflow.Stages) {
		if err := f.setStateUnlocked(types.TaskStateCompleted); err != nil {
			return err
		}
		if f.onTransition != nil {
			_ = f.onTransition(f.task, prevStageID, "COMPLETED")
		}
		return f.persist()
	}

	if err := f.setStateUnlocked(types.TaskStatePending); err != nil {
		return err
	}
	f.task.CurrentStageIndex = nextIdx
	f.task.CurrentStageID = f.workflow.Stages[nextIdx].ID
	if f.onTransition != nil {
		_ = f.onTransition(f.task, prevStageID, f.task.CurrentStageID)
	}
	return f.persist()
}

func (f *StateMachine) persist() error {
	if f.onPersist != nil {
		return f.onPersist(f.task)
	}
	return nil
}
