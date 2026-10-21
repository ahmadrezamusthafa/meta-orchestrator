package api

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/fsm"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// runnableStages lists the stage IDs the live flow can run, sorted.
func runnableStages() []string {
	ids := make([]string, 0, len(stageInstructions))
	for id := range stageInstructions {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// validateRunnable rejects a workflow whose stages the orchestrator cannot run.
func validateRunnable(def *types.WorkflowDefinition) error {
	if err := fsm.ValidateWorkflow(def); err != nil {
		return err
	}
	if def.Name == "" {
		return fmt.Errorf("workflow '%s' needs a name", def.ID)
	}
	for _, s := range def.Stages {
		if _, ok := stageInstructions[s.ID]; !ok {
			return fmt.Errorf("stage %q cannot run: use one of %v", s.ID, runnableStages())
		}
	}
	return nil
}

// workflowStages returns the stage order of a workflow. An unknown ID (a task saved by an older
// build or naming a workflow that was removed) runs the default pipeline, as every task did before.
func (r *Router) workflowStages(workflowID string) []string {
	if workflowID == "" {
		return stagePipeline
	}
	def, err := r.cfg.WorkflowReg.Get(workflowID)
	if err != nil {
		return stagePipeline
	}
	return fsm.StageIDs(def)
}

func indexIn(stages []string, stage string) int {
	for i, s := range stages {
		if s == stage {
			return i
		}
	}
	return -1
}

// stageIndexFor is the position of stage in the task's workflow, or -1.
func (r *Router) stageIndexFor(t *types.Task, stage string) int {
	return indexIn(r.workflowStages(t.WorkflowID), stage)
}

// taskStages are the stages a task runs: its workflow's, ending at the halt stage of its slice
// when it has one (a halt stage outside the workflow is ignored).
func (r *Router) taskStages(t *types.Task) []string {
	stages := r.workflowStages(t.WorkflowID)
	if t.ActiveSlice != nil && t.ActiveSlice.HaltStageID != "" {
		if i := indexIn(stages, t.ActiveSlice.HaltStageID); i >= 0 {
			return stages[:i+1]
		}
	}
	return stages
}

// nextStageFor returns the stage after current among the stages the task runs, or "" when current
// is its last stage — the workflow's last, or the halt stage of its slice — or not part of it.
func (r *Router) nextStageFor(t *types.Task, current string) string {
	stages := r.taskStages(t)
	i := indexIn(stages, current)
	if i < 0 || i+1 >= len(stages) {
		return ""
	}
	return stages[i+1]
}

// workflowForNewTask resolves the workflow a new task asks for; empty means the default.
func (r *Router) workflowForNewTask(id string) (*types.WorkflowDefinition, error) {
	if id == "" {
		id = fsm.DefaultWorkflowID
	}
	def, err := r.cfg.WorkflowReg.Get(id)
	if err != nil {
		return nil, fmt.Errorf("unknown workflow %q", id)
	}
	return def, nil
}

// workflowSlice returns the stages a new task runs (start to halt inclusive) and its first stage.
// A start or halt stage outside the workflow is an error.
func workflowSlice(def *types.WorkflowDefinition, slice *types.StageSlice) (stages []string, err error) {
	all := fsm.StageIDs(def)
	from, to := 0, len(all)-1
	if slice != nil && slice.StartStageID != "" {
		if from = indexIn(all, slice.StartStageID); from < 0 {
			return nil, fmt.Errorf("start stage %q is not part of workflow %s", slice.StartStageID, def.ID)
		}
	}
	if slice != nil && slice.HaltStageID != "" {
		if to = indexIn(all, slice.HaltStageID); to < 0 {
			return nil, fmt.Errorf("halt stage %q is not part of workflow %s", slice.HaltStageID, def.ID)
		}
		if to < from {
			return nil, fmt.Errorf("halt stage %q comes before start stage %q", slice.HaltStageID, slice.StartStageID)
		}
	}
	return append([]string(nil), all[from:to+1]...), nil
}

func (r *Router) workflowStorePath() string {
	if r.cfg.RootDir == "" {
		return ""
	}
	return filepath.Join(r.cfg.RootDir, ".sdlc", "workflows.json")
}

// loadWorkflows registers the operator's custom workflows: those saved through the API
// (.sdlc/workflows.json) and a declarative .sdlc/workflow.yaml. Invalid ones are skipped with a log line.
func (r *Router) loadWorkflows() {
	if r.cfg.RootDir == "" {
		return
	}
	if data, err := os.ReadFile(r.workflowStorePath()); err == nil {
		var defs []types.WorkflowDefinition
		if err := json.Unmarshal(data, &defs); err != nil {
			fmt.Printf("[Workflows] Ignoring unreadable %s: %v\n", r.workflowStorePath(), err)
		}
		for i := range defs {
			r.registerCustomWorkflow(&defs[i], r.workflowStorePath())
		}
	} else if !os.IsNotExist(err) {
		fmt.Printf("[Workflows] Failed to read %s: %v\n", r.workflowStorePath(), err)
	}
	yamlPath := filepath.Join(r.cfg.RootDir, ".sdlc", "workflow.yaml")
	if _, err := os.Stat(yamlPath); err == nil {
		def, err := fsm.ParseWorkflowYAML(yamlPath)
		if err != nil {
			fmt.Printf("[Workflows] Ignoring %s: %v\n", yamlPath, err)
			return
		}
		r.registerCustomWorkflow(def, yamlPath)
	}
}

func (r *Router) registerCustomWorkflow(def *types.WorkflowDefinition, source string) {
	if err := validateRunnable(def); err != nil {
		fmt.Printf("[Workflows] Skipping workflow %q from %s: %v\n", def.ID, source, err)
		return
	}
	if err := r.cfg.WorkflowReg.Add(def, true); err != nil {
		fmt.Printf("[Workflows] Skipping workflow %q from %s: %v\n", def.ID, source, err)
	}
}

// saveWorkflows writes the custom workflows atomically. Caller holds workflowsMu.
func (r *Router) saveWorkflows() error {
	path := r.workflowStorePath()
	if path == "" {
		return nil
	}
	data, err := json.MarshalIndent(r.cfg.WorkflowReg.Custom(), "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// transitionLocked moves task to state when the lifecycle allows it. Caller holds r.mu.
func (r *Router) transitionLocked(task *types.Task, to types.TaskState) error {
	if !fsm.CanTransition(task.State, to) {
		return fmt.Errorf("%s is %s and cannot move to %s", task.ID, task.State, to)
	}
	r.setTaskState(task, to)
	return nil
}
