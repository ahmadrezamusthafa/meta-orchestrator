package workspace

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/artifacts"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/fsm"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// ExternalSliceInput provides input for mid-process execution.
type ExternalSliceInput struct {
	SourceBranch      string `json:"source_branch,omitempty"`
	ExternalTaskPlan  string `json:"external_task_plan,omitempty"`
	ExternalPRD       string `json:"external_prd,omitempty"`
	ProduceVideo      bool   `json:"produce_video"`
}

// SliceIngestionEngine hydrates workspaces for mid-process execution slices.
type SliceIngestionEngine struct {
	artMgr      *artifacts.ArtifactManager
	synthesizer *SliceContextSynthesizer
	hydrator    *fsm.SliceHydrator
}

// NewSliceIngestionEngine creates an ingestion engine.
func NewSliceIngestionEngine(artMgr *artifacts.ArtifactManager) *SliceIngestionEngine {
	return &SliceIngestionEngine{
		artMgr:      artMgr,
		synthesizer: NewSliceContextSynthesizer(artMgr),
		hydrator:    fsm.NewSliceHydrator(),
	}
}

// IngestAndHydrate prepares the workspace and artifacts for sliced execution.
func (e *SliceIngestionEngine) IngestAndHydrate(
	ctx context.Context,
	ws *Workspace,
	task *types.Task,
	workflowDef *types.WorkflowDefinition,
	slice *types.StageSlice,
	input *ExternalSliceInput,
) (*fsm.SliceExecutionPlan, error) {
	// 1. Calculate slice bounds
	plan, err := e.hydrator.HydrateSlice(workflowDef, slice, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate slice plan: %w", err)
	}

	// 2. Checkout existing branch if specified
	if input != nil && input.SourceBranch != "" {
		for _, repoPath := range ws.RepoPaths {
			cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "checkout", input.SourceBranch)
			_ = cmd.Run()
		}
	}

	// 3. Ingest external TASK_PLAN.md if provided
	if input != nil && input.ExternalTaskPlan != "" {
		taskPlanPath := e.artMgr.TaskArtifactDir(task.ID)
		_, err := e.artMgr.WriteArtifact(task.ID, artifacts.ArtifactTaskPlan, input.ExternalTaskPlan, "operator_upload")
		if err != nil {
			return nil, fmt.Errorf("failed to write external task plan: %w", err)
		}
		_ = taskPlanPath
	}

	// 4. Synthesize upstream artifacts if starting mid-process (e.g. Stage 7 Implementation)
	if plan.StartIndex > 0 {
		err := e.synthesizer.SynthesizePrerequisites(task.ID, task.Title, task.Description)
		if err != nil {
			return nil, fmt.Errorf("failed to synthesize prerequisites: %w", err)
		}
	}

	// 5. Update task state and stage
	task.CurrentStageIndex = plan.StartIndex
	task.CurrentStageID = workflowDef.Stages[plan.StartIndex].ID
	task.ActiveSlice = slice

	return plan, nil
}
